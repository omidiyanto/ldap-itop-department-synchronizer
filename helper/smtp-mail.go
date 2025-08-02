package helper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/smtp"
	"os"
	"strings"
)

// encodeBase64 encodes data to base64 with line breaks every 76 chars
func EncodeBase64(data []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	enc := make([]byte, 0, len(data)*2)
	for i := 0; i < len(data); i += 3 {
		var b [3]byte
		n := copy(b[:], data[i:])
		enc = append(enc, table[b[0]>>2])
		enc = append(enc, table[((b[0]&0x03)<<4)|(b[1]>>4)])
		if n > 1 {
			enc = append(enc, table[((b[1]&0x0f)<<2)|(b[2]>>6)])
		} else {
			enc = append(enc, '=')
		}
		if n > 2 {
			enc = append(enc, table[b[2]&0x3f])
		} else {
			enc = append(enc, '=')
		}
	}

	// insert CRLF every 76 chars
	var out strings.Builder
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		out.Write(enc[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

// SendErrorMail sends an email with optional attachments
func SendErrorMail(subject, body string, attachments map[string][]byte) error {
	from := os.Getenv("EMAIL_FROM_ADDR")
	fromName := os.Getenv("EMAIL_FROM_NAME")
	toList := strings.Split(os.Getenv("EMAIL_TO"), ",")
	ccList := []string{}
	if cc := os.Getenv("EMAIL_CC"); cc != "" {
		ccList = strings.Split(cc, ",")
	}
	smtpHost := os.Getenv("EMAIL_SMTP_HOST")
	smtpPort := os.Getenv("EMAIL_SMTP_PORT")
	skipTLS := strings.ToLower(os.Getenv("EMAIL_SKIP_TLS_VERIFY")) == "true"

	// prepare headers
	boundary := "BOUNDARY-1234567890"
	headers := map[string]string{
		"From":         fmt.Sprintf("%s <%s>", fromName, from),
		"To":           strings.Join(toList, ", "),
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": fmt.Sprintf("multipart/mixed; boundary=%s", boundary),
	}
	if len(ccList) > 0 {
		headers["Cc"] = strings.Join(ccList, ", ")
	}

	// build message body
	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n" + body + "\r\n")

	// attach files
	for fname, data := range attachments {
		msg.WriteString("--" + boundary + "\r\n")
		msg.WriteString(fmt.Sprintf("Content-Type: application/octet-stream; name=\"%s\"\r\n", fname))
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", fname))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		msg.WriteString(EncodeBase64(data))
	}
	msg.WriteString("--" + boundary + "--\r\n")

	addr := smtpHost + ":" + smtpPort
	allRecipients := append(toList, ccList...)

	if skipTLS {
		return sendPlain(addr, from, allRecipients, strings.NewReader(msg.String()))
	}
	return sendTLS(addr, smtpHost, from, allRecipients, strings.NewReader(msg.String()))
}

func sendPlain(addr, from string, to []string, r io.Reader) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err = c.Rcpt(strings.TrimSpace(rcpt)); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

func sendTLS(addr, host, from string, to []string, r io.Reader) error {
	tlsConfig := &tls.Config{InsecureSkipVerify: true, ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err = c.Rcpt(strings.TrimSpace(rcpt)); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}
