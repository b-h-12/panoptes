package checks

import (
	"fmt"
	"sort"
	"strings"
)

type Check struct {
	Expr      string
	AlertName string
	Summary   string
}

type preset struct {
	build     func(instanceName string, above float64) string
	alertName string
	summary   func(instanceName string) string
}

var presets = map[string]preset{
	"cpu": {
		build: func(instanceName string, above float64) string {
			return fmt.Sprintf(`100 - (avg by (instance_name) (rate(node_cpu_seconds_total{mode="idle",instance_name=%q}[5m])) * 100) > %g`, instanceName, above)
		},
		alertName: "HighCPU",
		summary: func(instanceName string) string {
			return fmt.Sprintf("%s CPU usage is high", instanceName)
		},
	},
	"memory": {
		build: func(instanceName string, above float64) string {
			return fmt.Sprintf(`100 - ((node_memory_MemAvailable_bytes{instance_name=%q} / node_memory_MemTotal_bytes{instance_name=%q}) * 100) > %g`, instanceName, instanceName, above)
		},
		alertName: "HighMemory",
		summary: func(instanceName string) string {
			return fmt.Sprintf("%s memory usage is high", instanceName)
		},
	},
	"disk": {
		build: func(instanceName string, above float64) string {
			return fmt.Sprintf(`100 - ((node_filesystem_avail_bytes{instance_name=%q,mountpoint="/"} / node_filesystem_size_bytes{instance_name=%q,mountpoint="/"}) * 100) > %g`, instanceName, instanceName, above)
		},
		alertName: "HighDisk",
		summary: func(instanceName string) string {
			return fmt.Sprintf("%s disk usage is high", instanceName)
		},
	},
}

func Names() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Build(name, instanceName string, above float64) (Check, error) {
	p, ok := presets[name]
	if !ok {
		return Check{}, fmt.Errorf("unknown check %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return Check{
		Expr:      p.build(instanceName, above),
		AlertName: p.alertName,
		Summary:   p.summary(instanceName),
	}, nil
}
