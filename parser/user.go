package parser

import (
	"encoding/csv"
	"os"

	"github.com/go-ldap/ldap/v3"
)

type User struct {
	CN             string
	Email          string
	SAMAccountName string
	Department     string
}

// SaveUsersToCSV saves the list of users to a CSV file with CN, Email, SAMAccountName, Department fields
func SaveUsersToCSV(users []User, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"CN", "Email", "SAMAccountName", "Department"}); err != nil {
		return err
	}

	for _, u := range users {
		if err := writer.Write([]string{u.CN, u.Email, u.SAMAccountName, u.Department}); err != nil {
			return err
		}
	}
	return nil
}

func ParseUsers(entries []*ldap.Entry) []User {
	users := make([]User, 0, len(entries))
	for _, entry := range entries {
		users = append(users, User{
			CN:             entry.GetAttributeValue("cn"),
			Email:          entry.GetAttributeValue("mail"),
			SAMAccountName: entry.GetAttributeValue("sAMAccountName"),
			Department:     entry.GetAttributeValue("department"),
		})
	}
	return users
}
