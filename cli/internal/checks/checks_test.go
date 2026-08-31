package checks

import (
	"strings"
	"testing"
)

func TestBuildCPU(t *testing.T) {
	c, err := Build("cpu", "web1", 90)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if c.AlertName != "HighCPU" {
		t.Errorf("got AlertName %q, want %q", c.AlertName, "HighCPU")
	}
	if c.Summary != "web1 CPU usage is high" {
		t.Errorf("got Summary %q", c.Summary)
	}
	for _, want := range []string{`instance_name="web1"`, "node_cpu_seconds_total", "> 90"} {
		if !strings.Contains(c.Expr, want) {
			t.Errorf("expr %q missing %q", c.Expr, want)
		}
	}
}

func TestBuildMemory(t *testing.T) {
	c, err := Build("memory", "web1", 85)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if c.AlertName != "HighMemory" {
		t.Errorf("got AlertName %q, want %q", c.AlertName, "HighMemory")
	}
	for _, want := range []string{"node_memory_MemAvailable_bytes", "node_memory_MemTotal_bytes", `instance_name="web1"`, "> 85"} {
		if !strings.Contains(c.Expr, want) {
			t.Errorf("expr %q missing %q", c.Expr, want)
		}
	}
}

func TestBuildDisk(t *testing.T) {
	c, err := Build("disk", "web1", 90)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if c.AlertName != "HighDisk" {
		t.Errorf("got AlertName %q, want %q", c.AlertName, "HighDisk")
	}
	for _, want := range []string{"node_filesystem_avail_bytes", "node_filesystem_size_bytes", `mountpoint="/"`, `instance_name="web1"`, "> 90"} {
		if !strings.Contains(c.Expr, want) {
			t.Errorf("expr %q missing %q", c.Expr, want)
		}
	}
}

func TestBuildUnknownCheckReturnsError(t *testing.T) {
	_, err := Build("gpu", "web1", 50)
	if err == nil {
		t.Fatal("expected error for unknown check, got nil")
	}
	if !strings.Contains(err.Error(), "cpu") || !strings.Contains(err.Error(), "memory") || !strings.Contains(err.Error(), "disk") {
		t.Errorf("expected error to list available checks, got %q", err.Error())
	}
}

func TestNamesSortedAndComplete(t *testing.T) {
	got := Names()
	want := []string{"cpu", "disk", "memory"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}
