package main

import (
	"fmt"
	"log"
	"os"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"

	"ldap-itop/itopclient"
	"ldap-itop/ldapclient"
	"ldap-itop/parser"
	"ldap-itop/synchronizer"
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
	usersOut := "output/users.csv"
	reportOut := "output/validation-errors-report.csv"
	// Ensure output directory exists
	if err := os.MkdirAll("output", os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	threshold := 1.00 // Jaro-Winkler similarity threshold
	err = parser.ValidateAndAssignDepartment(users, yamlPath, usersOut, reportOut, threshold)
	if err != nil {
		log.Fatalf("Gagal proses validasi department: %v", err)
	}
	fmt.Println("Proses validasi department selesai. Lihat users.csv dan validation-errors-report.csv.")

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
	fmt.Println("Sync department as team ke iTop selesai. Lihat valid-department-list.yaml untuk TeamID.")

	// Sync users to teams in iTop
	notSyncedCSV := "output/user-not-synchronized.csv"
	err = synchronizer.SyncUsersToTeams(usersOut, yamlPath, notSyncedCSV, itopClient)
	if err != nil {
		log.Fatalf("Gagal sync user ke team iTop: %v", err)
	}
	fmt.Println("Sync user ke team iTop selesai. Lihat user-not-synchronized.csv untuk hasilnya.")
}
