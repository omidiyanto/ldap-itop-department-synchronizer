package ldapclient

import (
	"os"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"
)

// LDAPClient wraps the ldap.Conn
type LDAPClient struct {
	Conn *ldap.Conn
}

// NewLDAPClient creates and authenticates a new LDAP client
func NewLDAPClient() (*LDAPClient, error) {
	// Load .env file
	_ = godotenv.Load()

	ldapURL := os.Getenv("LDAP_URL")
	bindUser := os.Getenv("LDAP_BIND_USER") // username only
	bindPassword := os.Getenv("LDAP_BIND_PASSWORD")

	l, err := ldap.DialURL(ldapURL)
	if err != nil {
		return nil, err
	}

	err = l.Bind(bindUser, bindPassword)
	if err != nil {
		l.Close()
		return nil, err
	}

	return &LDAPClient{Conn: l}, nil
}

// Close closes the LDAP connection
func (c *LDAPClient) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
}
