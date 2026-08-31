package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlertNameDerivesFromJob(t *testing.T) {
	r := Rule{Job: "node"}
	if got := r.AlertName(); got != "NodeDown" {
		t.Errorf("got %q, want %q", got, "NodeDown")
	}
}

func TestAlertNameEmptyJobFallsBackToDown(t *testing.T) {
	r := Rule{}
	if got := r.AlertName(); got != "Down" {
		t.Errorf("got %q, want %q", got, "Down")
	}
}

func TestAlertNameOverrideWins(t *testing.T) {
	r := Rule{Job: "node", AlertNameOverride: "HighCPU"}
	if got := r.AlertName(); got != "HighCPU" {
		t.Errorf("got %q, want %q", got, "HighCPU")
	}
}

func TestSummaryTextDefault(t *testing.T) {
	r := Rule{Job: "node", InstanceName: "web1"}
	want := "node target web1 is down"
	if got := r.SummaryText(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSummaryTextOverrideWins(t *testing.T) {
	r := Rule{Job: "node", InstanceName: "web1", Summary: "web1 CPU usage is high"}
	if got := r.SummaryText(); got != "web1 CPU usage is high" {
		t.Errorf("got %q, want the override", got)
	}
}

func TestDescriptionTextDefaultMatchesOriginalWording(t *testing.T) {
	r := Rule{InstanceName: "web1", For: "2m"}
	want := "Target web1 has been unreachable for more than 2m."
	if got := r.DescriptionText(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDescriptionTextWithSummaryOverrideIsGeneric(t *testing.T) {
	r := Rule{InstanceName: "web1", For: "30s", Summary: "web1 CPU usage is high"}
	want := "web1 CPU usage is high (condition has held for more than 30s)."
	if got := r.DescriptionText(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPathFormat(t *testing.T) {
	r := Rule{Job: "node", InstanceName: "web1"}
	got := Path("/rules", r)
	want := filepath.Join("/rules", "node__web1.yml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderDefaultDownRule(t *testing.T) {
	r := Rule{
		Job:          "node",
		InstanceName: "web1",
		Expr:         `up{instance_name="web1"} == 0`,
		For:          "2m",
		Severity:     "critical",
	}
	data, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := string(data)

	for _, want := range []string{
		"alert: NodeDown",
		`expr: up{instance_name="web1"} == 0`,
		"for: 2m",
		"severity: critical",
		`summary: "node target web1 is down"`,
		`description: "Target web1 has been unreachable for more than 2m."`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered rule missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderCustomCheckRule(t *testing.T) {
	r := Rule{
		Job:               "node",
		InstanceName:      "web1",
		Expr:              `100 - (avg by (instance_name) (rate(node_cpu_seconds_total{mode="idle",instance_name="web1"}[5m])) * 100) > 90`,
		For:               "30s",
		Severity:          "critical",
		AlertNameOverride: "HighCPU",
		Summary:           "web1 CPU usage is high",
	}
	data, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := string(data)

	for _, want := range []string{
		"alert: HighCPU",
		`summary: "web1 CPU usage is high"`,
		`description: "web1 CPU usage is high (condition has held for more than 30s)."`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered rule missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestAddListRemoveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	r := Rule{Job: "node", InstanceName: "web1", Expr: "up == 0", For: "2m", Severity: "critical"}
	path, err := Add(dir, r)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected rule file to exist: %v", err)
	}

	list, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Job != "node" || list[0].InstanceName != "web1" {
		t.Errorf("got %v, want one rule for node/web1", list)
	}

	removed, err := Remove(dir, "node", "web1")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected rule file to be gone after Remove")
	}
}

func TestRemoveNonexistentReturnsFalseNotError(t *testing.T) {
	dir := t.TempDir()

	removed, err := Remove(dir, "node", "nobody")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Error("expected removed=false for a rule that was never added")
	}
}

func TestListSkipsFilesWithoutDoubleUnderscore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-a-rule.yml"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	list, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected List to skip malformed filenames, got %v", list)
	}
}
