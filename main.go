package main

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"

	"ldap-itop/itopclient"
	"ldap-itop/ldapclient"
	"ldap-itop/parser"
	"ldap-itop/synchronizer"
)

// Helper: base64 encode for attachments
func encodeBase64(data []byte) string {
	const base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	enc := make([]byte, 0, len(data)*2)
	for i := 0; i < len(data); i += 3 {
		var b [3]byte
		n := copy(b[:], data[i:])
		enc = append(enc, base64Table[b[0]>>2])
		enc = append(enc, base64Table[((b[0]&0x03)<<4)|(b[1]>>4)])
		if n > 1 {
			enc = append(enc, base64Table[((b[1]&0x0f)<<2)|(b[2]>>6)])
		} else {
			enc = append(enc, '=')
		}
		if n > 2 {
			enc = append(enc, base64Table[b[2]&0x3f])
		} else {
			enc = append(enc, '=')
		}
	}
	// Add line breaks every 76 chars
	out := ""
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		out += string(enc[i:end]) + "\r\n"
	}
	return out
}

func main() {
	// Helper: send email with error file contents and attachments
	sendErrorMail := func(subject, body string, attachments map[string][]byte) error {
		from := os.Getenv("EMAIL_FROM_ADDR")
		to := os.Getenv("EMAIL_TO")
		smtpHost := os.Getenv("EMAIL_SMTP_HOST")
		smtpPort := os.Getenv("EMAIL_SMTP_PORT")
		skipTLS := os.Getenv("EMAIL_SKIP_TLS_VERIFY")
		fromName := os.Getenv("EMAIL_FROM_NAME")

		recipients := strings.Split(to, ",")

		boundary := "BOUNDARY-1234567890"
		headers := make(map[string]string)
		headers["From"] = fromName + " <" + from + ">"
		headers["To"] = to
		headers["Subject"] = subject
		headers["MIME-Version"] = "1.0"
		headers["Content-Type"] = "multipart/mixed; boundary=" + boundary

		msg := ""
		for k, v := range headers {
			msg += k + ": " + v + "\r\n"
		}
		msg += "\r\n"
		msg += "--" + boundary + "\r\n"
		msg += "Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n"
		msg += body + "\r\n"

		for fname, content := range attachments {
			msg += "--" + boundary + "\r\n"
			msg += "Content-Type: text/csv; name=\"" + fname + "\"\r\n"
			msg += "Content-Disposition: attachment; filename=\"" + fname + "\"\r\n"
			msg += "Content-Transfer-Encoding: base64\r\n\r\n"
			msg += encodeBase64(content) + "\r\n"
		}
		msg += "--" + boundary + "--\r\n"

		addr := smtpHost + ":" + smtpPort

		if strings.ToLower(skipTLS) == "true" {
			c, err := smtp.Dial(addr)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Mail(from); err != nil {
				return err
			}
			for _, rcpt := range recipients {
				if err := c.Rcpt(rcpt); err != nil {
					return err
				}
			}
			w, err := c.Data()
			if err != nil {
				return err
			}
			_, err = w.Write([]byte(msg))
			if err != nil {
				return err
			}
			err = w.Close()
			if err != nil {
				return err
			}
			return c.Quit()
		} else {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: strings.ToLower(skipTLS) == "true",
				ServerName:         smtpHost,
			}
			conn, err := tls.Dial("tcp", addr, tlsConfig)
			if err != nil {
				return err
			}
			c, err := smtp.NewClient(conn, smtpHost)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Mail(from); err != nil {
				return err
			}
			for _, rcpt := range recipients {
				if err := c.Rcpt(rcpt); err != nil {
					return err
				}
			}
			w, err := c.Data()
			if err != nil {
				return err
			}
			_, err = w.Write([]byte(msg))
			if err != nil {
				return err
			}
			err = w.Close()
			if err != nil {
				return err
			}
			return c.Quit()
		}
	}

	_ = godotenv.Load()
	baseDN := os.Getenv("LDAP_BASE_DN")

	client, err := ldapclient.NewLDAPClient()
	if err != nil {
		log.Fatalf("LDAP auth failed: %v", err)
	}
	defer client.Close()
	log.Println("LDAP authentication successful.")

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=user)(objectCategory=person))",
		[]string{"cn", "mail", "sAMAccountName", "department"},
		nil,
	)

	sr, err := client.Conn.Search(searchRequest)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	users := parser.ParseUsers(sr.Entries)
	// fmt.Println("User list:")
	// for _, u := range users {
	// 	fmt.Printf("CN: %s, Email: %s, Username: %s, Department: %s\n", u.CN, u.Email, u.SAMAccountName, u.Department)
	// }

	// Validate and assign department, output users.csv and dept-validation-errors-report.csv
	yamlPath := "data/valid-department-list.yaml"
	usersOut := "output/users.csv"
	reportOut := "output/dept-validation-errors-report.csv"
	// Ensure output directory exists
	if err := os.MkdirAll("output", os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	threshold := 1.00 // Jaro-Winkler similarity threshold
	err = parser.ValidateAndAssignDepartment(users, yamlPath, usersOut, reportOut, threshold)
	if err != nil {
		log.Fatalf("Gagal proses validasi department: %v", err)
	}
	log.Println("Proses validasi department selesai. Lihat users.csv dan dept-validation-errors-report.csv.")

	// Read error files for later email
	reportBytes, _ := ioutil.ReadFile(reportOut)

	// Sync DepartmentName as Team in iTop
	itopURL := os.Getenv("ITOP_API_URL")
	itopUser := os.Getenv("ITOP_API_USER")
	itopPwd := os.Getenv("ITOP_API_PWD")
	itopVersion := os.Getenv("ITOP_VERSION")
	orgID := os.Getenv("ITOP_ORG_ID")
	if itopURL == "" || itopUser == "" || itopPwd == "" || itopVersion == "" || orgID == "" {
		log.Fatalf("ITOP_URL, ITOP_USER, ITOP_PASSWORD, ITOP_VERSION, ITOP_ORG_ID env vars must be set")
	}
	itopClient := &itopclient.ITopClient{
		BaseURL:  itopURL,
		Username: itopUser,
		Password: itopPwd,
		Version:  itopVersion,
	}
	err = synchronizer.SyncTeamsToItop(yamlPath, itopClient, orgID)
	if err != nil {
		log.Fatalf("Gagal sync department as team ke iTop: %v", err)
	}
	log.Println("Sync department as team ke iTop selesai. Lihat valid-department-list.yaml untuk TeamID.")

	// Sync users to teams in iTop
	notSyncedCSV := "output/user-not-synchronized.csv"
	err = synchronizer.SyncUsersToTeams(usersOut, yamlPath, notSyncedCSV, itopClient)
	if err != nil {
		log.Fatalf("Gagal sync user ke team iTop: %v", err)
	}
	log.Println("Sync user ke team iTop selesai. Lihat user-not-synchronized.csv untuk hasilnya.")

	notSyncedBytes, _ := ioutil.ReadFile(notSyncedCSV)

	// Send single email at end if any errors
	if len(reportBytes) > 0 || len(notSyncedBytes) > 0 {
		emailBody := "Berikut adalah hasil error sinkronisasi iTop x LDAP:\n\n"
		if len(reportBytes) > 0 {
			emailBody += "Dept Validation Errors:\n\n" + string(reportBytes) + "\n\n"
		}
		if len(notSyncedBytes) > 0 {
			emailBody += "User Not Synchronized Errors:\n\n" + string(notSyncedBytes) + "\n\n"
		}
		attachments := map[string][]byte{}
		if len(reportBytes) > 0 {
			attachments["dept-validation-errors-report.csv"] = reportBytes
		}
		if len(notSyncedBytes) > 0 {
			attachments["user-not-synchronized.csv"] = notSyncedBytes
		}
		err := sendErrorMail(os.Getenv("EMAIL_SUBJECT"), emailBody, attachments)
		if err != nil {
			log.Printf("Gagal kirim email error sinkronisasi: %v", err)
		} else {
			log.Println("Email error sinkronisasi sent.")
		}
	}
}
