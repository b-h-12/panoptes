package rules

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"panoptes/internal/atomicfile"
)

type Rule struct {
	Job               string
	InstanceName      string
	Expr              string
	For               string
	Severity          string
	AlertNameOverride string
	Summary           string
}

func (r Rule) AlertName() string {
	if r.AlertNameOverride != "" {
		return r.AlertNameOverride
	}
	if r.Job == "" {
		return "Down"
	}
	return strings.ToUpper(r.Job[:1]) + r.Job[1:] + "Down"
}

func (r Rule) SummaryText() string {
	if r.Summary != "" {
		return r.Summary
	}
	return fmt.Sprintf("%s target %s is down", r.Job, r.InstanceName)
}

func (r Rule) DescriptionText() string {
	if r.Summary != "" {
		return fmt.Sprintf("%s (condition has held for more than %s).", r.Summary, r.For)
	}
	return fmt.Sprintf("Target %s has been unreachable for more than %s.", r.InstanceName, r.For)
}

var tmpl = template.Must(template.New("rule").Parse(`groups:
  - name: {{.Job}}__{{.InstanceName}}
    rules:
      - alert: {{.AlertName}}
        expr: {{.Expr}}
        for: {{.For}}
        labels:
          severity: {{.Severity}}
          kind: {{.Job}}
          instance_name: {{.InstanceName}}
        annotations:
          summary: "{{.SummaryText}}"
          description: "{{.DescriptionText}}"
`))

func Path(dir string, r Rule) string {
	return filepath.Join(dir, r.Job+"__"+r.InstanceName+".yml")
}

func Render(r Rule) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Add(dir string, r Rule) (string, error) {
	data, err := Render(r)
	if err != nil {
		return "", err
	}
	path := Path(dir, r)
	if err := atomicfile.Write(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func List(dir string) ([]Rule, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}

	rules := make([]Rule, 0, len(matches))
	for _, path := range matches {
		base := strings.TrimSuffix(filepath.Base(path), ".yml")
		job, instanceName, ok := strings.Cut(base, "__")
		if !ok {
			continue
		}
		rules = append(rules, Rule{Job: job, InstanceName: instanceName})
	}
	return rules, nil
}

func Remove(dir, job, instanceName string) (bool, error) {
	path := Path(dir, Rule{Job: job, InstanceName: instanceName})
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
