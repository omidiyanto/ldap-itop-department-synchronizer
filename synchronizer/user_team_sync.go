package synchronizer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	itopclient "ldap-itop/itopclient"

	"gopkg.in/yaml.v2"
)

type UserCSV struct {
	CN              string
	Email           string
	SAMAccountName  string
	Department      string
	ValidDepartment string
}

type TeamYAML struct {
	DepartmentName string `yaml:"DepartmentName"`
	TeamID         string `yaml:"TeamID,omitempty"`
}

type TeamYAMLList []TeamYAML

func SyncUsersToTeams(usersCSV, yamlPath, notSyncedCSV string, client *itopclient.ITopClient) error {
	// Load users.csv
	f, err := os.Open(usersCSV)
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return err
	}
	colIdx := map[string]int{}
	for i, h := range head {
		colIdx[h] = i
	}
	var users []UserCSV
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		users = append(users, UserCSV{
			CN:              rec[colIdx["CN"]],
			Email:           rec[colIdx["Email"]],
			SAMAccountName:  rec[colIdx["SAMAccountName"]],
			Department:      rec[colIdx["Department"]],
			ValidDepartment: rec[colIdx["Valid-Department"]],
		})
	}

	// Load YAML
	yamlFile, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var teamsYAML TeamYAMLList
	if err := yaml.Unmarshal(yamlFile, &teamsYAML); err != nil {
		return err
	}

	// Build map: ValidDepartment -> (TeamID, DepartmentName)
	type teamInfo struct {
		TeamID   string
		DeptName string
	}
	teamMap := make(map[string]teamInfo)
	for _, t := range teamsYAML {
		if t.TeamID != "" {
			teamMap[t.DepartmentName] = teamInfo{TeamID: t.TeamID, DeptName: t.DepartmentName}
		}
	}

	// Prepare not-synced CSV
	notSyncedF, err := os.Create(notSyncedCSV)
	if err != nil {
		return err
	}
	defer notSyncedF.Close()
	notSyncedW := csv.NewWriter(notSyncedF)
	defer notSyncedW.Flush()
	notSyncedW.Write([]string{"nama", "email", "status"})

	// Prepare successfully-synced CSV
	successSyncedCSV := "output/user-successfully-sync.csv"
	successSyncedF, err := os.Create(successSyncedCSV)
	if err != nil {
		return err
	}
	defer successSyncedF.Close()
	successSyncedW := csv.NewWriter(successSyncedF)
	defer successSyncedW.Flush()
	successSyncedW.Write([]string{"nama", "email", "team_id", "status"})

	for _, user := range users {
		log.Printf("[In-Progress] Processing user: %s (%s) - Department: %s", user.CN, user.Email, user.ValidDepartment)
		team, ok := teamMap[user.ValidDepartment]
		if !ok || team.TeamID == "" {
			notSyncedW.Write([]string{user.CN, user.Email, "No TeamID mapping for department: " + user.ValidDepartment})
			continue
		}
		userID := ""
		if user.SAMAccountName != "" {
			resp, err := client.Post("core/get", map[string]interface{}{
				"class":         "User",
				"key":           fmt.Sprintf("SELECT User WHERE login=\"%s\"", user.SAMAccountName),
				"output_fields": "contactid,login,email",
			})
			if err == nil && resp != nil {
				var respMap map[string]interface{}
				if err := json.Unmarshal(resp, &respMap); err == nil {
					objsRaw, ok := respMap["objects"]
					if ok {
						if objs, ok := objsRaw.(map[string]interface{}); ok && len(objs) > 0 {
							for _, v := range objs {
								if obj, ok := v.(map[string]interface{}); ok {
									if fields, ok := obj["fields"].(map[string]interface{}); ok {
										switch idVal := fields["contactid"].(type) {
										case string:
											userID = idVal
										case float64:
											userID = fmt.Sprintf("%.0f", idVal)
										}
									}
								}
							}
						}
					}
				}
			}
		}
		if userID == "" {
			notSyncedW.Write([]string{user.CN, user.Email, "User not found in iTop (by login)"})
			continue
		}
		resp, err := client.Post("core/get", map[string]interface{}{
			"class":         "Team",
			"key":           team.TeamID,
			"output_fields": "persons_list",
		})
		personsList := []map[string]interface{}{}
		if err == nil && resp != nil {
			var respMap map[string]interface{}
			if err := json.Unmarshal(resp, &respMap); err == nil {
				teamKey := fmt.Sprintf("Team::%s", team.TeamID)
				if obj, ok := respMap["objects"].(map[string]interface{}); ok {
					if teamObj, ok := obj[teamKey].(map[string]interface{}); ok {
						if fields, ok := teamObj["fields"].(map[string]interface{}); ok {
							if pl, ok := fields["persons_list"].([]interface{}); ok {
								for _, p := range pl {
									if pm, ok := p.(map[string]interface{}); ok {
										entry := map[string]interface{}{
											"person_id": pm["person_id"],
											"role_id":   pm["role_id"],
										}
										personsList = append(personsList, entry)
									}
								}
							}
						}
					}
				}
			}
		}
		alreadyInTeam := false
		for _, p := range personsList {
			if fmt.Sprintf("%v", p["person_id"]) == userID {
				alreadyInTeam = true
				break
			}
		}
		if alreadyInTeam {
			successSyncedW.Write([]string{user.CN, user.Email, team.TeamID, "Already in team (sync ke department: " + team.DeptName + ")"})
			continue
		}
		personsList = append(personsList, map[string]interface{}{
			"person_id": userID,
			"role_id":   "0",
		})
		updateResp, err := client.Post("core/update", map[string]interface{}{
			"class":   "Team",
			"key":     team.TeamID,
			"comment": fmt.Sprintf("Menambahkan Person::%s (%s) ke Team::%s", userID, user.CN, team.TeamID),
			"fields": map[string]interface{}{
				"persons_list": personsList,
			},
		})
		if err != nil {
			notSyncedW.Write([]string{user.CN, user.Email, "Failed to add to team: " + err.Error()})
			continue
		}
		var updateMap map[string]interface{}
		if err := json.Unmarshal(updateResp, &updateMap); err != nil {
			notSyncedW.Write([]string{user.CN, user.Email, "Failed to parse update response: " + err.Error()})
			continue
		}
		code, _ := updateMap["code"].(float64)
		msg, _ := updateMap["message"].(string)
		objects, _ := updateMap["objects"].(map[string]interface{})
		teamKey := "Team::" + team.TeamID
		teamObj, ok := objects[teamKey].(map[string]interface{})
		if code == 0 && ok {
			fields, _ := teamObj["fields"].(map[string]interface{})
			if pl, ok := fields["persons_list"].([]interface{}); ok {
				found := false
				for _, p := range pl {
					if pm, ok := p.(map[string]interface{}); ok {
						if fmt.Sprintf("%v", pm["person_id"]) == userID {
							found = true
							break
						}
					}
				}
				if found {
					successSyncedW.Write([]string{user.CN, user.Email, team.TeamID, "Successfully added to team (sync ke department: " + team.DeptName + ")"})
					continue
				}
			}
		}
		failMsg := "Failed to add to team: "
		if msg != "" {
			failMsg += msg
		} else {
			failMsg += "unknown error or user not present in persons_list after update"
		}
		notSyncedW.Write([]string{user.CN, user.Email, failMsg})
	}

	return nil
}
