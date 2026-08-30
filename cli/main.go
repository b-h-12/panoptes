package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"panoptes/internal/prom"
	"panoptes/internal/rules"
	"panoptes/internal/targets"
)

var targetsDir = envOr("TARGETS_DIR", "/etc/prometheus/targets")
var rulesDir = envOr("RULES_DIR", "/etc/prometheus/rules")
var promURL = envOr("PROMETHEUS_URL", "http://localhost:9090")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	if os.Args[1] == "reload" {
		if err := reloadPrometheus(); err != nil {
			fmt.Fprintln(os.Stderr, "panoptes:", err)
			os.Exit(1)
		}
		return
	}

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
	case verb == "add" && noun == "rule":
		err = addRule(args)
	case verb == "list" && noun == "rules":
		err = listRules(args)
	case verb == "remove" && noun == "rule":
		err = removeRule(args)
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "panoptes:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  panoptes add target --type <kind> --name <friendly-name> --address <host:port>
  panoptes list targets [--type <kind>]
  panoptes remove target --type <kind> --name <friendly-name>
  panoptes add rule --name <friendly-name> [--for <duration>] [--severity <sev>]
  panoptes list rules
  panoptes remove rule --type <kind> --name <friendly-name>
  panoptes reload`)
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

func addRule(args []string) error {
	fs := flag.NewFlagSet("add rule", flag.ExitOnError)
	name := fs.String("name", "", "friendly instance name (must already exist as a target)")
	forDuration := fs.String("for", "2m", "how long the target must be down before firing")
	severity := fs.String("severity", "critical", "alert severity")
	fs.Parse(args)

	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	_, job, err := targets.FindByName(targetsDir, *name)
	if err != nil {
		return err
	}

	rule := rules.Rule{
		Job:          job,
		InstanceName: *name,
		Expr:         fmt.Sprintf(`up{instance_name=%q} == 0`, *name),
		For:          *forDuration,
		Severity:     *severity,
	}

	path, err := rules.Add(rulesDir, rule)
	if err != nil {
		return err
	}
	fmt.Printf("added rule %q for %q to %s\n", rule.AlertName(), *name, path)

	if err := prom.Reload(promURL); err != nil {
		return fmt.Errorf("rule written but failed to reload prometheus: %w", err)
	}
	fmt.Println("reloaded prometheus")
	return nil
}

func listRules(args []string) error {
	fs := flag.NewFlagSet("list rules", flag.ExitOnError)
	fs.Parse(args)

	rs, err := rules.List(rulesDir)
	if err != nil {
		return err
	}

	if len(rs) == 0 {
		fmt.Println("(no rules)")
		return nil
	}
	for _, r := range rs {
		fmt.Printf("%s\t%s\n", r.AlertName(), r.InstanceName)
	}
	return nil
}

func removeRule(args []string) error {
	fs := flag.NewFlagSet("remove rule", flag.ExitOnError)
	targetType := fs.String("type", "", "target kind, e.g. node")
	name := fs.String("name", "", "friendly instance name")
	fs.Parse(args)

	if *targetType == "" || *name == "" {
		return fmt.Errorf("--type and --name are both required")
	}

	removed, err := rules.Remove(rulesDir, *targetType, *name)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("no rule found for %q (%s)", *name, *targetType)
	}
	fmt.Printf("removed rule for %q\n", *name)

	if err := prom.Reload(promURL); err != nil {
		return fmt.Errorf("rule removed but failed to reload prometheus: %w", err)
	}
	fmt.Println("reloaded prometheus")
	return nil
}

func reloadPrometheus() error {
	if err := prom.Reload(promURL); err != nil {
		return err
	}
	fmt.Println("reloaded prometheus")
	return nil
}
