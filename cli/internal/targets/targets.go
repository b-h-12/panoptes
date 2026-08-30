package targets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"panoptes/internal/atomicfile"
)

type Group struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func Load(path string) ([]Group, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Group{}, nil
	}
	if err != nil {
		return nil, err
	}
	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return groups, nil
}

func Save(path string, groups []Group) error {
	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data, 0o644)
}

func Add(path string, group Group) error {
	unlock, err := atomicfile.Lock(path)
	if err != nil {
		return err
	}
	defer unlock()

	groups, err := Load(path)
	if err != nil {
		return err
	}

	name := group.Labels["instance_name"]
	for _, g := range groups {
		if g.Labels["instance_name"] == name {
			return fmt.Errorf("target %q already exists", name)
		}
	}

	groups = append(groups, group)
	return Save(path, groups)
}

func Remove(path, name string) (bool, error) {
	unlock, err := atomicfile.Lock(path)
	if err != nil {
		return false, err
	}
	defer unlock()

	groups, err := Load(path)
	if err != nil {
		return false, err
	}

	kept := []Group{}
	removed := false
	for _, g := range groups {
		if g.Labels["instance_name"] == name {
			removed = true
			continue
		}
		kept = append(kept, g)
	}
	if !removed {
		return false, nil
	}
	return true, Save(path, kept)
}

func FindByName(dir, name string) (group Group, targetType string, err error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return Group{}, "", err
	}
	for _, path := range matches {
		groups, err := Load(path)
		if err != nil {
			return Group{}, "", err
		}
		for _, g := range groups {
			if g.Labels["instance_name"] == name {
				return g, strings.TrimSuffix(filepath.Base(path), ".json"), nil
			}
		}
	}
	return Group{}, "", fmt.Errorf("no target named %q found in %s", name, dir)
}
