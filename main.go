package main

import (
	"fmt"
	"log"
	"os"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"

	"ldap-itop/ldapclient"
	"ldap-itop/parser"
)

func main() {
	_ = godotenv.Load()
	baseDN := os.Getenv("LDAP_BASE_DN")

	client, err := ldapclient.NewLDAPClient()
	if err != nil {
		log.Fatalf("LDAP auth failed: %v", err)
	}
	defer client.Close()
	fmt.Println("LDAP authentication successful.")

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

	// Validate and assign department, output users.csv and validation-errors-report.csv
	yamlPath := "data/valid-department-list.yaml"
	usersOut := "users.csv"
	reportOut := "validation-errors-report.csv"
	threshold := 1.00 // Jaro-Winkler similarity threshold
	err = parser.ValidateAndAssignDepartment(users, yamlPath, usersOut, reportOut, threshold)
	if err != nil {
		log.Fatalf("Gagal proses validasi department: %v", err)
	}
	fmt.Println("Proses validasi department selesai. Lihat users.csv dan validation-errors-report.csv.")
}
