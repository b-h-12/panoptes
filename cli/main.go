package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"monictl/internal/targets"
)

var targetsDir = envOr("TARGETS_DIR", "/etc/prometheus/targets")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}

	verb, noun, args := os.Args[1], os.Args[2], os.Args[3:]

	var err error
	switch {
	case verb == "add" && noun == "target":
		err = addTarget(args)
	case verb == "list" && noun == "targets":
		err = listTargets(args)
	case verb == "remove" && noun == "target":
		err = removeTarget(args)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "monictl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  monictl add target --type <kind> --name <friendly-name> --address <host:port>
  monictl list targets [--type <kind>]
  monictl remove target --type <kind> --name <friendly-name>`)
}

func addTarget(args []string) error {
	fs := flag.NewFlagSet("add target", flag.ExitOnError)
	targetType := fs.String("type", "", "target kind, e.g. node")
	name := fs.String("name", "", "friendly instance name")
	address := fs.String("address", "", "host:port to scrape")
	fs.Parse(args)

	if *targetType == "" || *name == "" || *address == "" {
		return fmt.Errorf("--type, --name and --address are all required")
	}

	group := targets.Group{
		Targets: []string{*address},
		Labels: map[string]string{
			"job":           *targetType,
			"kind":          *targetType,
			"instance_name": *name,
		},
	}

	path := filepath.Join(targetsDir, *targetType+".json")
	if err := targets.Add(path, group); err != nil {
		return err
	}
	fmt.Printf("added target %q (%s) to %s\n", *name, *address, path)
	return nil
}

func listTargets(args []string) error {
	fs := flag.NewFlagSet("list targets", flag.ExitOnError)
	targetType := fs.String("type", "node", "target kind, e.g. node")
	fs.Parse(args)

	path := filepath.Join(targetsDir, *targetType+".json")
	groups, err := targets.Load(path)
	if err != nil {
		return err
	}

	if len(groups) == 0 {
		fmt.Println("(no targets)")
		return nil
	}
	for _, g := range groups {
		fmt.Printf("%s\t%v\t%v\n", g.Labels["instance_name"], g.Targets, g.Labels)
	}
	return nil
}

func removeTarget(args []string) error {
	fs := flag.NewFlagSet("remove target", flag.ExitOnError)
	targetType := fs.String("type", "", "target kind, e.g. node")
	name := fs.String("name", "", "friendly instance name")
	fs.Parse(args)

	if *targetType == "" || *name == "" {
		return fmt.Errorf("--type and --name are both required")
	}

	path := filepath.Join(targetsDir, *targetType+".json")
	removed, err := targets.Remove(path, *name)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("no target named %q found in %s", *name, path)
	}
	fmt.Printf("removed target %q from %s\n", *name, path)
	return nil
}
