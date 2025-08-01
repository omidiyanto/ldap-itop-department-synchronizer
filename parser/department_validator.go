package parser

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/xrash/smetrics"
	"gopkg.in/yaml.v2"
)

type DepartmentYAML struct {
	DepartmentName string   `yaml:"DepartmentName"`
	SubList        []string `yaml:"SubList"`
}

type DepartmentYAMLList []DepartmentYAML

// ValidateAndAssignDepartment validates and assigns the best DepartmentName for each user
func ValidateAndAssignDepartment(users []User, yamlPath, usersOut, reportOut string, threshold float64) error {
	// Read YAML
	data, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var deptList DepartmentYAMLList
	if err := yaml.Unmarshal(data, &deptList); err != nil {
		return err
	}

	usersFile, err := os.Create(usersOut)
	if err != nil {
		return err
	}
	defer usersFile.Close()
	usersWriter := csv.NewWriter(usersFile)
	defer usersWriter.Flush()
	usersWriter.Write([]string{"CN", "Email", "SAMAccountName", "Department", "Valid-Department"})

	reportFile, err := os.Create(reportOut)
	if err != nil {
		return err
	}
	defer reportFile.Close()
	reportWriter := csv.NewWriter(reportFile)
	defer reportWriter.Flush()
	reportWriter.Write([]string{"CN", "Email", "SAMAccountName", "Department", "Predicted-Valid-Department", "Confidence-Score"})

	for _, u := range users {
		bestDept := ""
		bestScore := 0.0
		for _, d := range deptList {
			// Compare to DepartmentName
			score := smetrics.JaroWinkler(strings.ToUpper(u.Department), strings.ToUpper(d.DepartmentName), 0.7, 4)
			if score > bestScore {
				bestScore = score
				bestDept = d.DepartmentName
			}
			// Compare to SubList
			for _, sub := range d.SubList {
				if strings.TrimSpace(sub) == "" {
					continue
				}
				subScore := smetrics.JaroWinkler(strings.ToUpper(u.Department), strings.ToUpper(sub), 0.7, 4)
				if subScore > bestScore {
					bestScore = subScore
					bestDept = d.DepartmentName
				}
			}
		}
		if bestScore >= threshold {
			usersWriter.Write([]string{u.CN, u.Email, u.SAMAccountName, u.Department, bestDept})
		} else {
			// Report: show best guess and confidence
			reportWriter.Write([]string{u.CN, u.Email, u.SAMAccountName, u.Department, bestDept, fmt.Sprintf("%.2f%%", bestScore*100)})
		}
	}
	return nil
}
