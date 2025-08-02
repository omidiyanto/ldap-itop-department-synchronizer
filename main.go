package main

import (
	"bytes"
	"encoding/csv"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/joho/godotenv"
	"github.com/tealeg/xlsx"

	"ldap-itop/helper"
	"ldap-itop/itopclient"
	"ldap-itop/ldapclient"
	"ldap-itop/parser"
	"ldap-itop/synchronizer"
)

func toXLSX(csvData []byte) []byte {
	records, _ := csv.NewReader(strings.NewReader(string(csvData))).ReadAll()
	file := xlsx.NewFile()
	sheet, _ := file.AddSheet("Sheet1")
	for _, row := range records {
		xrow := sheet.AddRow()
		for _, cell := range row {
			xcell := xrow.AddCell()
			xcell.Value = cell
		}
	}
	buf := new(bytes.Buffer)
	file.Write(buf)
	return buf.Bytes()
}

func initItopClient() (*itopclient.ITopClient, string) {
	itopURL := os.Getenv("ITOP_API_URL")
	itopUser := os.Getenv("ITOP_API_USER")
	itopPwd := os.Getenv("ITOP_API_PWD")
	itopVersion := os.Getenv("ITOP_VERSION")
	orgID := os.Getenv("ITOP_ORG_ID")
	client := &itopclient.ITopClient{BaseURL: itopURL, Username: itopUser, Password: itopPwd, Version: itopVersion}
	return client, orgID
}

func buildEmailBody(hasDeptErr, hasUserErr bool) string {
	body := "Dear Team,\n\nBerikut adalah hasil error sinkronisasi user dan departmentnya dari AD ke iTop:\n"
	if hasDeptErr {
		body += "- Terdapat Department Validation Errors (Adanya department pada user yang tidak valid)\n"
	}
	if hasUserErr {
		body += "- User Not Synchronized Errors (Adanya user yang gagal dalam proses syncronization dari AD ke iTop)\n"
	}
	body += "\nSilakan periksa lampiran untuk detail lebih lanjut.\n\nBest regards,\nDevOps Team"
	return body
}

func main() {
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

	// Validate and assign department, write CSV reports
	yamlPath := "data/valid-department-list.yaml"
	usersOut := "output/users.csv"
	reportOut := "output/dept-validation-errors-report.csv"
	if err := os.MkdirAll("output", os.ModePerm); err != nil {
		log.Fatalf("Failed create output dir: %v", err)
	}
	threshold := 1.00 // Jaro-Winkler similarity threshold
	err = parser.ValidateAndAssignDepartment(users, yamlPath, usersOut, reportOut, threshold)
	if err != nil {
		log.Fatalf("Department validation failed: %v", err)
	}
	log.Println("Department validation complete.")

	reportBytes, _ := ioutil.ReadFile(reportOut)

	// Convert CSV to XLSX if error
	var deptXlsx []byte
	if len(reportBytes) > 0 {
		deptXlsx = toXLSX(reportBytes)
	}

	// Sync teams/department and users to iTop
	itopClient, orgID := initItopClient()
	err = synchronizer.SyncTeamsToItop(yamlPath, itopClient, orgID)
	if err != nil {
		log.Fatalf("Team/Department sync failed: %v", err)
	}
	log.Println("Teams/Departments synced successfully.")

	notSyncedCSV := "output/user-not-synchronized.csv"
	err = synchronizer.SyncUsersToTeams(usersOut, yamlPath, notSyncedCSV, itopClient)
	if err != nil {
		log.Fatalf("User sync failed: %v", err)
	}
	log.Println("Users synced successfully.")

	notSyncedBytes, _ := ioutil.ReadFile(notSyncedCSV)
	var userXlsx []byte
	if len(notSyncedBytes) > 0 {
		userXlsx = toXLSX(notSyncedBytes)
	}

	// Send email if any errors
	if len(deptXlsx) > 0 || len(userXlsx) > 0 {
		subject := os.Getenv("EMAIL_SUBJECT")
		body := buildEmailBody(len(deptXlsx) > 0, len(userXlsx) > 0)
		attachments := map[string][]byte{}
		if len(deptXlsx) > 0 {
			attachments["dept-validation-errors-report.xlsx"] = deptXlsx
		}
		if len(userXlsx) > 0 {
			attachments["user-not-synchronized.xlsx"] = userXlsx
		}
		err := helper.SendErrorMail(subject, body, attachments)
		if err != nil {
			log.Printf("Failed to send email: %v", err)
		} else {
			log.Println("Email sent.")
		}
	}
}
