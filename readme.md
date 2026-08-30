# Panoptes

Prometheus, Grafana and Alertmanager, wired together but watching
nothing until you tell them to. A CLI called `panoptes` is the only
door in: it adds targets, it adds alert rules, nothing else touches
the config.

The loop it's built around: register a target, Prometheus starts
scraping it, attach an alert rule, kill the target, watch the rule go
`inactive` → `pending` → `firing` and land in Alertmanager routed by
severity. Grafana sits on top with a couple of live panels.

## Stack

- **Prometheus** — scrapes targets, evaluates alert rules
- **Alertmanager** — receives firing alerts, groups and routes them by severity
- **Grafana** — a provisioned dashboard (target up/down, CPU usage) reading from Prometheus
- **node_exporter** — the one thing being monitored out of the box, on host networking so it sees real host metrics
- **panoptes** (`cli/`) — a Go binary that adds/lists/removes targets and rules by writing config files atomically and telling Prometheus to reload

Nothing in `prometheus/targets/` or `prometheus/rules/` should be
hand-edited. The CLI writes those files (temp file + atomic rename, so
a crash mid-write can't leave Prometheus reading a half-written file)
and triggers `/-/reload` itself. Edit them by hand and you're on your
own for consistency.

## Prerequisites

- Docker and Docker Compose
- These ports free on the host: `9090` (Prometheus), `9093`
  (Alertmanager), `3000` (Grafana), `9100` (node_exporter)
- Nothing else. The CLI has its own Dockerfile and never runs directly
  on the host, so no local Go install is needed.

## Setup

```bash
git clone <this-repo-url>
cd panoptes
cp .env.example .env        # set a real GRAFANA_ADMIN_PASSWORD
docker compose up -d
docker compose build cli
```

`docker compose ps` should show four containers up — Prometheus,
Alertmanager and Grafana with port mappings, `node_exporter` without
one (it's host networking). Grafana is reachable at
`http://localhost:3000`, login `admin` / whatever you set for
`GRAFANA_ADMIN_PASSWORD` (falls back to `admin` if you skip the `.env`
step, which is fine locally and a bad idea anywhere reachable over a
network).

## Using the CLI

The CLI runs as a one-shot container, not a long-lived service:

```bash
docker compose run --rm cli <command>
```

```
panoptes add target --type <kind> --name <friendly-name> --address <host:port>
panoptes list targets [--type <kind>]
panoptes remove target --type <kind> --name <friendly-name>
panoptes add rule --name <friendly-name> [--for <duration>] [--severity <sev>]
panoptes list rules
panoptes remove rule --type <kind> --name <friendly-name>
panoptes reload
```

`--type` is the exporter kind (`node` for node_exporter — it's also
the Prometheus job name for that target, and the file it lands in:
`prometheus/targets/node.json`). `--name` is a friendly identifier you
pick, kept separate from Prometheus's own auto-generated `instance`
label so the two don't get confused.

`add rule` finds the target by name, generates
`up{instance_name="<name>"} == 0` as the alert expression, writes
`prometheus/rules/<kind>__<name>.yml`, and reloads Prometheus in the
same step. There's no `--expr` flag — one target, one down-alert, kept
deliberately simple for now.

Example, start to finish:

```bash
docker compose run --rm cli add target --type node --name my-laptop --address host.docker.internal:9100
docker compose run --rm cli list targets
docker compose run --rm cli add rule --name my-laptop --for 2m --severity critical
docker compose run --rm cli list rules
```

## Watching an alert actually fire

This is the part worth doing once, by hand, so the loop stops being
theoretical:

```bash
# target should be up within one scrape (~15s)
curl -s localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {labels, health}'

# kill the thing being monitored
docker compose stop node_exporter

# after ~30s the target flips down; after the full `for: 2m` the rule fires
curl -s localhost:9090/api/v1/rules | jq '.data.groups[].rules[] | {name, state}'
curl -s localhost:9093/api/v2/alerts | jq '.[] | {labels, status}'

# bring it back
docker compose start node_exporter
```

The alert should show up in Alertmanager with `severity: critical`,
routed on the faster pacing configured for that severity in
`alertmanager/alertmanager.yml` — a `warning`-severity rule would use
slower grouping instead.

## Layout

```
alertmanager/alertmanager.yml       routing tree, grouped by severity
prometheus/prometheus.yml           scrape config, points at targets/ and rules/
prometheus/targets/                 target files the CLI writes (file_sd)
prometheus/rules/                   alert rule files the CLI writes
grafana/provisioning/               datasource + dashboard-provider config
grafana/dashboards/                 dashboard JSON (target up/down, CPU %)
cli/main.go                         command dispatch
cli/internal/targets/               target file read/modify/write
cli/internal/rules/                 rule file read/modify/write
cli/internal/atomicfile/            temp-file + rename + optional flock, shared by both
cli/internal/prom/                  the /-/reload call
docker-compose.yml                  all five services
```

## Configuration

Set in `.env` (copy `.env.example` to start):

- `GRAFANA_ADMIN_PASSWORD` — Grafana's admin login, defaults to
  `admin` if unset

Set for the `cli` service in `docker-compose.yml`, not usually
something you need to touch:

- `PROMETHEUS_URL` — where the CLI sends `/-/reload` (`http://prometheus:9090` inside the compose network)
- `TARGETS_DIR` / `RULES_DIR` — where the CLI reads and writes target/rule files

## Status

Working end to end: the stack, target management, rule management
with reload, and the Grafana dashboard. Not built yet: actual alert
delivery outside the Alertmanager UI (Slack/Discord), a web UI over
the same target/rule logic, and anything involving fleet automation.
