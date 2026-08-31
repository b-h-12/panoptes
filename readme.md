# Panoptes

Prometheus, Grafana and Alertmanager, wired together but watching
nothing until you tell them to. A CLI called `panoptes` is the only
door in: it adds targets, it adds alert rules, nothing else touches
the config.

The loop it's built around: register a target, Prometheus starts
scraping it, attach an alert rule, kill the target, watch the rule go
`inactive` → `pending` → `firing` and land in Alertmanager routed by
severity — from there it reaches a real page (PagerDuty), a chat
channel (Slack), and a push notification (ntfy), all from one alert.
Grafana sits on top with a couple of live panels.

## Stack

- **Prometheus** — scrapes targets, evaluates alert rules
- **Alertmanager** — receives firing alerts, groups and routes them by
  severity, and fans critical alerts out to PagerDuty, Slack, and ntfy
- **Grafana** — a provisioned dashboard (target up/down, CPU usage)
  reading from Prometheus
- **node_exporter** — the one thing being monitored out of the box, on
  host networking so it sees real host metrics
- **panoptes** (`cli/`) — a Go binary that adds/lists/removes targets
  and rules by writing config files atomically and telling Prometheus
  to reload
- **ntfy-adapter** (`ntfy-adapter/`) — a small Go webhook adapter that
  turns Alertmanager's webhook payload into a clean push notification
  on [ntfy.sh](https://ntfy.sh) (Alertmanager doesn't speak ntfy's
  format natively)

Nothing in `prometheus/targets/` or `prometheus/rules/` should be
hand-edited. The CLI writes those files (temp file + atomic rename, so
a crash mid-write can't leave Prometheus reading a half-written file)
and triggers `/-/reload` itself. Edit them by hand and you're on your
own for consistency.

## Prerequisites

- Docker and Docker Compose
- These ports free on the host: `9090` (Prometheus), `9093`
  (Alertmanager), `3000` (Grafana), `9100` (node_exporter)
- Nothing else. The CLI and the ntfy adapter each have their own
  Dockerfile and never run directly on the host, so no local Go
  install is needed.

## Setup

```bash
git clone <this-repo-url>
cd panoptes
cp .env.example .env        # set a real GRAFANA_ADMIN_PASSWORD and NTFY_TOPIC
docker compose up -d
docker compose build cli ntfy-adapter
```

`docker compose ps` should show all containers up — Prometheus,
Alertmanager, Grafana and `ntfy-adapter` with port mappings or internal
networking, `node_exporter` without one (it's host networking).
Grafana is reachable at `http://localhost:3000`, login `admin` /
whatever you set for `GRAFANA_ADMIN_PASSWORD` (falls back to `admin`
if you skip the `.env` step, which is fine locally and a bad idea
anywhere reachable over a network).

### Alerting integrations (PagerDuty / Slack)

`NTFY_TOPIC` (in `.env`) is the only piece of alerting config that's a
plain environment variable — pick something unguessable, since public
ntfy.sh topics are unauthenticated (e.g. `yourname-a1b2c3d4`), and open
`https://ntfy.sh/<your-topic>` in a browser or the ntfy app to watch it.

PagerDuty and Slack need real credentials, which don't belong in
version control at all — not even as an env var default. They're read
from two files under `alertmanager/secrets/` (git-ignored):

```
alertmanager/secrets/pagerduty_routing_key   # PagerDuty service's Events API v2 integration key
alertmanager/secrets/slack_webhook_url       # Slack Incoming Webhook URL
```

Create both files with the real values before starting Alertmanager.
If a file doesn't exist, `pagerduty_configs`/`slack_configs` in
`alertmanager/alertmanager.yml` will fail at send-time (not at
startup) — check `docker compose logs alertmanager` if a critical
alert doesn't reach one of them.

## Using the CLI

The CLI runs as a one-shot container, not a long-lived service:

```bash
docker compose run --rm cli <command>
```

A thin wrapper script at `bin/panoptes` does the same thing with
shorter typing — add `bin/` to your `PATH` (or symlink it onto one)
and just run `panoptes <command>` directly; it still executes inside
Docker underneath, nothing changes about how it works.

```
panoptes add target --type <kind> --name <friendly-name> --address <host:port>
panoptes list targets [--type <kind>]
panoptes remove target --type <kind> --name <friendly-name>
panoptes add rule --name <friendly-name> [--expr <promql> | --check <cpu|memory|disk> --above <percent>] [--alert-name <name>] [--summary <text>] [--for <duration>] [--severity <sev>]
panoptes list rules
panoptes remove rule --type <kind> --name <friendly-name>
panoptes reload
```

`--type` is the exporter kind (`node` for node_exporter — it's also
the Prometheus job name for that target, and the file it lands in:
`prometheus/targets/node.json`). `--name` is a friendly identifier you
pick, kept separate from Prometheus's own auto-generated `instance`
label so the two don't get confused.

`add rule` finds the target by name and writes
`prometheus/rules/<kind>__<name>.yml`, reloading Prometheus in the
same step. The alert expression comes from one of three places, in
order of how much PromQL you need to know:

- **nothing** — defaults to `up{instance_name="<name>"} == 0`, i.e.
  the target-down check
- **`--check <cpu|memory|disk> --above <percent>`** — a named preset
  that generates the right node_exporter expression for you (e.g.
  `--check cpu --above 90` alerts when CPU busy% exceeds 90), with a
  sensible default alert name (`HighCPU`/`HighMemory`/`HighDisk`) and
  summary already filled in
- **`--expr "<promql>"`** — anything else, written by hand

`--expr` and `--check` are mutually exclusive. `--alert-name` and
`--summary` override the defaults either the down-check or a `--check`
preset would otherwise use.

Example, start to finish:

```bash
docker compose run --rm cli add target --type node --name my-laptop --address host.docker.internal:9100
docker compose run --rm cli list targets
docker compose run --rm cli add rule --name my-laptop --for 2m --severity critical
docker compose run --rm cli add rule --name my-laptop --check cpu --above 90 --for 30s
docker compose run --rm cli list rules
```

(Note: rule files are keyed by `<job>__<instance_name>.yml`, so adding
a second rule for the same target replaces the first rather than
adding alongside it — one rule file per target today.)

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
slower grouping instead. If PagerDuty/Slack/ntfy secrets are set up,
the same alert reaches all three within a few seconds of firing, and
again when it resolves.

## Layout

```
alertmanager/alertmanager.yml       routing tree, grouped by severity, fans out to PagerDuty/Slack/ntfy
alertmanager/secrets/               PagerDuty routing key + Slack webhook URL (git-ignored)
prometheus/prometheus.yml           scrape config, points at targets/ and rules/
prometheus/targets/                 target files the CLI writes (file_sd)
prometheus/rules/                   alert rule files the CLI writes
grafana/provisioning/               datasource + dashboard-provider config
grafana/dashboards/                 dashboard JSON (target up/down, CPU %)
cli/main.go                         command dispatch
cli/internal/targets/               target file read/modify/write
cli/internal/rules/                 rule file read/modify/write
cli/internal/checks/                cpu/memory/disk expression presets for `add rule --check`
cli/internal/atomicfile/            temp-file + rename + optional flock, shared by targets/rules
cli/internal/prom/                  the /-/reload call
bin/panoptes                        wrapper script: `panoptes <cmd>` instead of `docker compose run --rm cli <cmd>`
ntfy-adapter/                       webhook -> ntfy.sh push notification adapter
docker-compose.yml                  all services
```

## Configuration

Set in `.env` (copy `.env.example` to start):

- `GRAFANA_ADMIN_PASSWORD` — Grafana's admin login, defaults to
  `admin` if unset
- `NTFY_TOPIC` — the ntfy.sh topic alerts get pushed to; pick something
  unguessable, since ntfy.sh topics are public and unauthenticated

Read from git-ignored files under `alertmanager/secrets/`, not `.env`
(see [Alerting integrations](#alerting-integrations-pagerduty--slack)
above):

- `pagerduty_routing_key`
- `slack_webhook_url`

Set for the `cli` service in `docker-compose.yml`, not usually
something you need to touch:

- `PROMETHEUS_URL` — where the CLI sends `/-/reload` (`http://prometheus:9090` inside the compose network)
- `TARGETS_DIR` / `RULES_DIR` — where the CLI reads and writes target/rule files

## Status

Working end to end: the stack, target management, rule management
(including CPU/memory/disk presets, not just up/down) with reload, the
Grafana dashboard, and real alert delivery to PagerDuty, Slack, and
ntfy. Not built yet: a web UI over the same target/rule logic, a
natural-language rule vocabulary beyond the three built-in presets,
and anything involving fleet automation.
