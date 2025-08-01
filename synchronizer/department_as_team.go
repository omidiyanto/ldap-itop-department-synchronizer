package synchronizer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	itopclient "ldap-itop/itopclient"

	"gopkg.in/yaml.v2"
)

type DepartmentYAML struct {
	DepartmentName string   `yaml:"DepartmentName"`
	SubList        []string `yaml:"SubList"`
	TeamID         string   `yaml:"TeamID,omitempty"`
}

type DepartmentYAMLList []DepartmentYAML

func SyncTeamsToItop(yamlPath string, client *itopclient.ITopClient, orgID string) error {
	// Read YAML
	data, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var deptList DepartmentYAMLList
	if err := yaml.Unmarshal(data, &deptList); err != nil {
		return err
	}

	// Get existing teams from iTop
	params := map[string]interface{}{
		"class":         "Team",
		"key":           "SELECT Team",
		"output_fields": "id,name",
	}
	resp, err := client.Post("core/get", params)
	if err != nil {
		return err
	}
	var result struct {
		Objects map[string]struct {
			Fields struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"fields"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}

	existingTeams := make(map[string]string) // name -> id
	existingTeamIDs := make(map[string]bool) // id -> true
	for _, obj := range result.Objects {
		name := strings.TrimSpace(obj.Fields.Name)
		if name != "" {
			existingTeams[strings.ToUpper(name)] = obj.Fields.ID
			existingTeamIDs[obj.Fields.ID] = true
		}
	}

	changed := false
	for i, d := range deptList {
		if d.DepartmentName == "" {
			continue
		}
		teamName := d.DepartmentName
		// 1. If TeamID exists in YAML, check if still exists in iTop
		if d.TeamID != "" {
			if _, found := existingTeamIDs[d.TeamID]; found {
				// TeamID still valid, skip
				continue
			} else {
				log.Printf("[INFO] TeamID %s for '%s' not found in iTop, will create new.", d.TeamID, teamName)
			}
		}
		// 2. If not, check by name
		teamID, exists := existingTeams[strings.ToUpper(teamName)]
		if exists {
			if d.TeamID != teamID {
				deptList[i].TeamID = teamID
				changed = true
				log.Printf("[INFO] Found team '%s' in iTop with ID %s, updating YAML.", teamName, teamID)
			}
			continue
		}
		// 3. Create team if not exists
		params := map[string]interface{}{
			"class":         "Team",
			"comment":       fmt.Sprintf("Creating department %s", teamName),
			"output_fields": "id,name",
			"fields": map[string]interface{}{
				"name":   teamName,
				"org_id": orgID,
				"status": "active",
			},
		}
		resp, err := client.Post("core/create", params)
		if err != nil {
			log.Printf("[ERROR] Failed to create team %s: %v\nResponse: %s", teamName, err, string(resp))
			return fmt.Errorf("failed to create team %s: %w", teamName, err)
		}
		var createResult struct {
			Objects map[string]struct {
				Fields struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"fields"`
			} `json:"objects"`
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		if err := json.Unmarshal(resp, &createResult); err != nil {
			log.Printf("[ERROR] Failed to parse create team response for %s: %v\nResponse: %s", teamName, err, string(resp))
			return err
		}
		if createResult.Code != 0 {
			log.Printf("[ERROR] iTop API error creating team %s: %s (code %d)\nResponse: %s", teamName, createResult.Message, createResult.Code, string(resp))
			return fmt.Errorf("iTop API error creating team %s: %s (code %d)", teamName, createResult.Message, createResult.Code)
		}
		for _, obj := range createResult.Objects {
			deptList[i].TeamID = obj.Fields.ID
			changed = true
			log.Printf("[OK] Created team '%s' with ID %s", teamName, obj.Fields.ID)
			break
		}
	}

	if changed {
		out, err := yaml.Marshal(&deptList)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(yamlPath, out, 0644); err != nil {
			return err
		}
	}
	return nil
}
