package targets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	groups, err := Load(path)
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected empty slice, got %v", groups)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Error("expected error loading invalid JSON, got nil")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	want := []Group{
		{Targets: []string{"10.0.0.1:9100"}, Labels: map[string]string{"instance_name": "web1", "job": "node"}},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].Labels["instance_name"] != "web1" {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAddAppendsToEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	group := Group{Targets: []string{"10.0.0.1:9100"}, Labels: map[string]string{"instance_name": "web1"}}
	if err := Add(path, group); err != nil {
		t.Fatalf("Add: %v", err)
	}

	groups, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
}

func TestAddRejectsDuplicateInstanceName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	group := Group{Targets: []string{"10.0.0.1:9100"}, Labels: map[string]string{"instance_name": "web1"}}
	if err := Add(path, group); err != nil {
		t.Fatalf("first Add: %v", err)
	}

	err := Add(path, group)
	if err == nil {
		t.Fatal("expected error adding duplicate instance_name, got nil")
	}
}

func TestRemoveLastTargetLeavesEmptyArrayNotNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	group := Group{Targets: []string{"10.0.0.1:9100"}, Labels: map[string]string{"instance_name": "web1"}}
	if err := Add(path, group); err != nil {
		t.Fatalf("Add: %v", err)
	}

	removed, err := Remove(path, "web1")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(raw) != "[]\n" {
		t.Errorf("got file content %q, want %q (must not be null)", raw, "[]\n")
	}
}

func TestRemoveNonexistentReturnsFalseNotError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.json")

	removed, err := Remove(path, "nobody")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed {
		t.Error("expected removed=false for a target that was never added")
	}
}

func TestFindByNameAcrossMultipleTypeFiles(t *testing.T) {
	dir := t.TempDir()

	nodeGroup := Group{Targets: []string{"10.0.0.1:9100"}, Labels: map[string]string{"instance_name": "web1"}}
	if err := Add(filepath.Join(dir, "node.json"), nodeGroup); err != nil {
		t.Fatalf("Add node: %v", err)
	}
	httpGroup := Group{Targets: []string{"https://example.com"}, Labels: map[string]string{"instance_name": "homepage"}}
	if err := Add(filepath.Join(dir, "http.json"), httpGroup); err != nil {
		t.Fatalf("Add http: %v", err)
	}

	group, targetType, err := FindByName(dir, "homepage")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if targetType != "http" {
		t.Errorf("got targetType %q, want %q", targetType, "http")
	}
	if group.Labels["instance_name"] != "homepage" {
		t.Errorf("got group %v, want instance_name=homepage", group)
	}
}

func TestFindByNameNotFoundReturnsError(t *testing.T) {
	dir := t.TempDir()

	if _, _, err := FindByName(dir, "nobody"); err == nil {
		t.Error("expected error for a name that doesn't exist, got nil")
	}
}
