# Usage guide

This is the full reference for running panoptes day to day — every CLI
flag, every file format on disk, how the alerting pipeline actually
routes a notification, and what each Grafana panel is showing you. The
README covers install; this assumes the stack is already up.

- [Architecture](#architecture)
- [Quick start](#quick-start)
- [CLI reference](#cli-reference)
- [On-disk file formats](#on-disk-file-formats)
- [Alerting pipeline](#alerting-pipeline)
- [Grafana dashboard](#grafana-dashboard)
- [Walkthroughs](#walkthroughs)
- [Troubleshooting](#troubleshooting)
- [Current limitations](#current-limitations)

## Architecture

Five containers, one CLI:

```
node_exporter --scrape--> prometheus --evaluate rules--> alertmanager --route--> paged receiver
                               ^                                                  |  |  |
                               |                                                  |  |  ntfy-adapter -> ntfy.sh
                          targets/*.json                                          |  slack_configs -> Slack
                          rules/*.yml                                             pagerduty_configs -> PagerDuty
                               ^
                          panoptes (cli)
                               |
                            grafana (reads Prometheus, shows targets/CPU/mem/disk/alerts)
```

`panoptes` is the only thing that writes into `prometheus/targets/`
and `prometheus/rules/`. It writes via a temp-file-plus-rename (so
Prometheus never reads a half-written file) and then calls
Prometheus's `/-/reload` endpoint itself — you never restart a
container just to pick up a target or rule change.

Alertmanager doesn't talk to PagerDuty, Slack, or ntfy the same way.
PagerDuty and Slack have first-class support built into Alertmanager
(`pagerduty_configs`, `slack_configs`). ntfy doesn't, so there's a
small adapter service (`ntfy-adapter/`) sitting in between: Alertmanager
POSTs its normal webhook JSON at it, and the adapter reformats that
into a plain HTTP POST with the headers ntfy expects.

## Quick start

Full setup detail is in the [README](../readme.md). The short version,
once you've cloned the repo:

```bash
cp .env.example .env
docker compose up -d
docker compose build cli ntfy-adapter
```

Everything below assumes that's done and `docker compose ps` shows
Prometheus, Alertmanager, Grafana, and `ntfy-adapter` up.

## CLI reference

The CLI is a one-shot binary, not a long-running service. Run it
either through compose directly:

```bash
docker compose run --rm cli <command>
```

or through the wrapper at `bin/panoptes`, which does exactly that
underneath and exists so you can type less:

```bash
bin/panoptes <command>
# or, with bin/ on your PATH:
panoptes <command>
```

The rest of this section uses the short form. Swap in whichever
invocation you're actually using.

### `add target`

```
panoptes add target --type <kind> --name <friendly-name> --address <host:port>
```

- `--type` — the exporter kind. It doubles as the Prometheus job name
  and the filename the target lands in
  (`prometheus/targets/<type>.json`). The CLI accepts any string here,
  but `prometheus/prometheus.yml` currently defines exactly one scrape
  job, hardcoded to `node` and `targets/node.json` — so `node` is the
  only value that actually gets scraped. Use anything else and the
  command succeeds, the file gets written, and Prometheus just never
  reads it: no error anywhere, the target simply never shows up. Adding
  a second kind means adding a matching `scrape_configs` entry (and
  `file_sd_configs` file) to `prometheus.yml` yourself first.
- `--name` — a friendly identifier you pick, stored as the
  `instance_name` label. Deliberately separate from Prometheus's own
  `instance` label, which it derives from the address — the two exist
  side by side and won't get confused with each other.
- `--address` — `host:port` for Prometheus to scrape. From inside the
  Prometheus container, `host.docker.internal:9100` reaches
  node_exporter running with host networking on this same machine; a
  remote node_exporter just needs its real reachable address.

All three flags are required. Adding a `--name` that already exists
(for that `--type`) fails rather than silently overwriting it — remove
it first if you want to change its address.

### `list targets`

```
panoptes list targets [--type <kind>]
```

`--type` defaults to `node`. Prints one line per target: name,
address(es), and the full label set. Reads the file directly, no
Prometheus API call involved, so this shows what's configured, not
necessarily what Prometheus has picked up yet (target files refresh on
a 30s interval — see [Troubleshooting](#troubleshooting) if that gap
is confusing you).

### `remove target`

```
panoptes remove target --type <kind> --name <friendly-name>
```

Both flags required. Errors if no target with that name exists under
that type. Note this only removes the target — an alert rule
referencing it keeps existing, now pointed at a target that no longer
scrapes. Remove the rule too if you're decommissioning something for
good (see [`remove rule`](#remove-rule)).

### `add rule`

```
panoptes add rule --name <friendly-name>
  [--expr <promql> | --check <cpu|memory|disk> --above <percent>]
  [--alert-name <name>] [--summary <text>]
  [--for <duration>] [--severity <sev>]
```

`--name` must match an existing target — the CLI looks it up to derive
the Prometheus job (`kind` label) the rule attaches to. Everything else
is optional, but the expression comes from exactly one of three places:

**Nothing specified** — defaults to a down-check:
```promql
up{instance_name="<name>"} == 0
```

**`--check <cpu|memory|disk> --above <percent>`** — one of three
built-in presets, each expanding to a real node_exporter expression
scoped to that one target via its `instance_name` label:

| Check | Expression |
|---|---|
| `cpu` | `100 - (avg by (instance_name) (rate(node_cpu_seconds_total{mode="idle",instance_name="<name>"}[5m])) * 100) > <above>` |
| `memory` | `100 - ((node_memory_MemAvailable_bytes{instance_name="<name>"} / node_memory_MemTotal_bytes{instance_name="<name>"}) * 100) > <above>` |
| `disk` | `100 - ((node_filesystem_avail_bytes{instance_name="<name>",mountpoint="/"} / node_filesystem_size_bytes{instance_name="<name>",mountpoint="/"}) * 100) > <above>` |

Each preset also fills in a default alert name (`HighCPU`,
`HighMemory`, `HighDisk`) and a default summary annotation
(`"<name> CPU usage is high"`, etc.) — you don't need `--alert-name` or
`--summary` unless you want something else. `--above` is required when
`--check` is given; there's no sensible default threshold.

**`--expr "<promql>"`** — anything you write by hand. Same rule
mechanics apply (`--for`, `--severity`, labels), the CLI just doesn't
generate the expression for you.

`--expr` and `--check` are mutually exclusive — the command errors if
you pass both.

`--for` (default `2m`) is how long the expression has to stay true
before the alert moves from `pending` to `firing`. `--severity`
(default `critical`) sets the `severity` label, which is what
Alertmanager's routing tree keys on — see
[Alerting pipeline](#alerting-pipeline).

`--alert-name` and `--summary` override whatever default the
down-check or a `--check` preset would otherwise use — useful if you
want two different CPU thresholds to show up as visibly different
alerts (e.g. `--alert-name CPUWarning` at 80% and the default
`HighCPU` at 95%, though see the file-naming caveat below before you
try that).

On success, `add rule` writes the rule file and calls Prometheus's
`/-/reload` in the same step. If the reload fails, the rule is still
on disk — you'll see a message saying so, and a plain `panoptes
reload` once Prometheus is reachable again picks it up.

**One rule file per target.** Rule files are named
`<job>__<instance_name>.yml`, so a second `add rule` for the same
`--name` overwrites the first rather than adding a second alert
alongside it. If you want both a down-alert and a CPU-threshold alert
on the same target, that's not supported yet — see
[Current limitations](#current-limitations).

### `list rules`

```
panoptes list rules
```

No flags. Lists every rule file under `prometheus/rules/`, showing the
derived alert name and the target it's attached to. This reads files
on disk (parsed from the `<job>__<name>.yml` filename, not the YAML
body), so it always reflects what's configured — not necessarily what
Prometheus has actually loaded, if a reload failed silently at some
point. Cross-check against `curl -s localhost:9090/api/v1/rules` if
you suspect drift.

### `remove rule`

```
panoptes remove rule --type <kind> --name <friendly-name>
```

Both flags required (note this one needs `--type`, unlike `add rule`,
since removal doesn't have a target lookup to derive it from). Deletes
the rule file and reloads Prometheus. Errors if no matching rule file
exists.

### `reload`

```
panoptes reload
```

POSTs to Prometheus's `/-/reload` directly. You shouldn't normally
need this — `add rule` and `remove rule` already call it — but it's
there for when a reload failed earlier and you want to retry it
without touching any files.

## On-disk file formats

Useful if you want to read the config directly instead of going
through `list targets`/`list rules`, or you're debugging why
Prometheus isn't seeing what you expect. Don't hand-edit these — the
CLI is the only writer, and there's no lock protecting you from a
concurrent `panoptes` invocation if you do.

**`prometheus/targets/<kind>.json`** — a Prometheus `file_sd_configs`
file, a JSON array of target groups:

```json
[
  {
    "targets": ["host.docker.internal:9100"],
    "labels": {
      "job": "node",
      "kind": "node",
      "instance_name": "my-laptop"
    }
  }
]
```

Every key under `labels` gets attached to every metric series scraped
from that group — that's how `instance_name` ends up queryable in
PromQL and how alert rules can scope an expression to one specific
target.

**`prometheus/rules/<job>__<name>.yml`** — a standard Prometheus rule
group, one rule per file:

```yaml
groups:
  - name: node__my-laptop
    rules:
      - alert: NodeDown
        expr: up{instance_name="my-laptop"} == 0
        for: 2m
        labels:
          severity: critical
          kind: node
          instance_name: my-laptop
        annotations:
          summary: "node target my-laptop is down"
          description: "Target my-laptop has been unreachable for more than 2m."
```

The `kind` and `instance_name` labels on the rule itself (not just the
target) are what let Alertmanager group and route on them the same way
it would on any other label.

## Alerting pipeline

`alertmanager/alertmanager.yml` defines one routing tree, keyed on the
`severity` label every rule carries:

| Severity | Receiver | `group_wait` | `group_interval` | `repeat_interval` |
|---|---|---|---|---|
| `critical` | `paged` | 10s | 1m | 1h |
| `warning` | `default-null` | 1m | 10m | 12h |
| (default, no match) | `default-null` | 30s | 5m | 4h |

`default-null` is a receiver with no integrations — alerts route
there but nothing fires. `paged` is where the real notifications live,
fanning every critical alert out to all three of PagerDuty, Slack, and
ntfy at once. If you want warning-severity alerts to actually notify
somewhere, that's a one-line change in `alertmanager.yml` — point the
`warning` route at `paged` too, or give it its own receiver.

### Setting up the receivers

**ntfy** is the simplest — just an env var. Set `NTFY_TOPIC` in
`.env` to something unguessable (ntfy.sh topics are public and
unauthenticated — anyone who knows your topic name can read your
alerts or post fake ones). Open `https://ntfy.sh/<your-topic>` in a
browser or the ntfy app to watch it live.

**PagerDuty and Slack** need real credentials, which is why they're
read from files under `alertmanager/secrets/` instead of environment
variables — that directory is git-ignored, so nothing sensitive ends
up in version control even by accident:

```
alertmanager/secrets/pagerduty_routing_key   # one line: your PagerDuty service's Events API v2 integration key
alertmanager/secrets/slack_webhook_url       # one line: your Slack Incoming Webhook URL
```

Create both files with the real values before starting Alertmanager
(or restart it after creating them — it reads them at send-time, not
just at startup, so you can also add a missing one later without a
restart). If a file is missing or empty when an alert fires,
`pagerduty_configs`/`slack_configs` fails silently from Alertmanager's
perspective — no error at startup, just a missing notification. Check
`docker compose logs alertmanager` if PagerDuty or Slack isn't getting
anything.

### What the ntfy-adapter actually does

Alertmanager's native webhook payload doesn't match what ntfy expects
— ntfy wants a plain HTTP POST with the message as the body and
metadata as headers (`Title`, `Priority`, `Tags`), not a JSON envelope.
`ntfy-adapter/main.go` is the translation:

- A **firing** alert becomes a POST with `Title` set to the alert
  name, `Tags: warning`, and `Priority: default` — or, if
  `severity: critical`, `Tags: rotating_light` and `Priority: urgent`,
  which is what makes ntfy treat it as an urgent push (bypassing Do
  Not Disturb on most ntfy clients).
- A **resolved** alert becomes `<alertname> resolved` /
  `<instance_name> has recovered`, tagged `white_check_mark`, always
  at default priority — recoveries don't need to interrupt anyone.
- The message body is the rule's `summary` annotation if present,
  otherwise a generic `<name> is <status>` fallback.

It's a genuinely small service — one HTTP handler, no state, no
database — kept that way on purpose. If you want a Discord or Teams
version, this file is the template: parse Alertmanager's webhook JSON,
reformat, POST to whatever the destination actually expects.

## Grafana dashboard

One provisioned dashboard, "Node Overview", five panels, all reading
from Prometheus:

- **Target Up** — a stat panel on `up{job="node"}`, one tile per
  target, red when `0` (down) and green when `1` (up). The fastest way
  to see at a glance whether anything's unreachable right now.
- **CPU Usage %** — `100 - idle%`, i.e. how busy each target's CPU
  actually is, one line per `instance_name`.
- **Memory Usage %** — `100 - available%` against total memory, same
  per-instance breakdown.
- **Disk Usage %** — same shape, scoped to the root filesystem
  (`mountpoint="/"`) specifically — a target with multiple mounted
  volumes only shows root here, not everything.
- **Active Alerts** — a table on Prometheus's built-in `ALERTS` metric,
  showing every currently `pending` or `firing` alert with its labels.
  This is the fastest way to see alert state without leaving Grafana
  for Prometheus's own UI or Alertmanager's.

An empty stack (no targets added yet) shows the panels present but
blank — that's expected, not broken. Add a target and give it one
scrape interval (~15s) to start showing data.

## Walkthroughs

### Monitor a new host, down-alert only

```bash
panoptes add target --type node --name web-1 --address 203.0.113.10:9100
panoptes add rule --name web-1
```

That's the whole thing — default `--for 2m`, default `--severity
critical`, default down-check expression. Within one scrape interval
the target shows `up` in Grafana; within `for: 2m` of it actually going
down, the alert fires and reaches every configured receiver.

### Add a CPU threshold alert instead of (or alongside) the down-check

```bash
panoptes add rule --name web-1 --check cpu --above 90 --for 5m
```

Remember this **replaces** whatever rule already existed for `web-1`
(see [the one-rule-per-target caveat](#add-rule)) — it doesn't stack
with a down-check rule you added earlier. If you want both watched
concurrently today, you'd need two differently-`--type`d targets
pointing at the same address, which is a real workaround but not a
clean one.

### Watch the full pipeline fire, without waiting two minutes

Useful for testing the alert path itself rather than the "target
actually went down" scenario:

```bash
panoptes add target --type node --name test --address host.docker.internal:9100
panoptes add rule --name test --for 15s
docker compose stop node_exporter
sleep 20
curl -s localhost:9090/api/v1/rules | jq '.data.groups[].rules[] | {name, state}'
curl -s localhost:9093/api/v2/alerts | jq '.[] | {labels, status}'
```

Expect `state: "firing"` in Prometheus and one alert in Alertmanager's
list within a few seconds of the 15s `for` elapsing. If PagerDuty,
Slack, and ntfy are all configured, all three notify within
`group_wait` (10s for critical) of that. Bring `node_exporter` back
with `docker compose start node_exporter` and everything resolves,
notifications included.

### Decommission a target cleanly

```bash
panoptes remove rule --type node --name web-1
panoptes remove target --type node --name web-1
```

Rule first, then target — removing the target first leaves a rule
file referencing a target that no longer exists, which isn't harmful
(the expression just never matches anything) but is clutter worth not
leaving around.

## Troubleshooting

**Target added but never shows up as scraped.** Check the file
actually landed where Prometheus is looking:
`docker compose exec prometheus cat /etc/prometheus/targets/node.json`.
The usual cause is the target file's directory not lining up with the
`file_sd_configs` path in `prometheus/prometheus.yml`. Also give it the
full 30s refresh interval before assuming it's actually stuck.

**Target shows `down` with `context deadline exceeded`.** Almost
always a reachability problem, not a panoptes problem. From inside the
Prometheus container: `docker compose exec prometheus getent hosts
host.docker.internal` should resolve if you're scraping something on
the host itself. For a remote address, confirm it's actually reachable
from wherever the Prometheus container runs, not just from your own
machine.

**`add rule` or `reload` fails with a connection error.** Prometheus
needs `--web.enable-lifecycle` in its compose command for `/-/reload`
to work at all — it's off by default upstream since arbitrary reload
is a minor attack surface. It's already set in this repo's
`docker-compose.yml`; if you've forked and modified that file, check
it's still there.

**Rule written but never reaches `firing`.** Rule files aren't
hot-reloaded the way target files are — `add rule` calls `/-/reload`
for you, but if that call itself failed (check the CLI's own output;
it says so explicitly rather than failing silently) the rule sits on
disk unloaded until you run `panoptes reload` again. Separately, the
`for:` clock resets if the condition flickers back to false even
briefly, so a flapping target can look "stuck" in `pending` when it's
actually just never holding long enough.

**Alert fires in Prometheus but never appears in Alertmanager.** Check
`prometheus/prometheus.yml`'s `alerting.alertmanagers` block points at
the right address (`alertmanager:9093`, the compose service name — not
`localhost`), and check `docker compose logs prometheus` for
connection errors.

**Alert reaches Alertmanager but not PagerDuty/Slack/ntfy.** For
PagerDuty/Slack, check `docker compose logs alertmanager` — a missing
or malformed file under `alertmanager/secrets/` fails at send-time
with no startup error. For ntfy, confirm `NTFY_TOPIC` is actually set
in `.env` (not just `.env.example`) and check
`docker compose logs ntfy-adapter` — it logs every publish failure
with the underlying error.

**Grafana panels show no data.** Confirm a target actually exists and
is up — an empty stack has nothing to plot, and that's not a bug.
Otherwise check `docker compose logs grafana | grep -i provisioning`
for a datasource or dashboard-provisioning error.

**Port already in use on `docker compose up`.** Something else on the
host already has `9090`, `9093`, `3000`, or `9100`. Free it, or change
the host-side half of the port mapping in `docker-compose.yml` (the
container-side port and every internal reference stay the same either
way).

## Current limitations

Worth knowing going in, not hidden anywhere else in the docs:

- **One rule file per target.** Adding a second rule for the same
  `--name` overwrites the first. Multiple concurrent alerts on one
  target (e.g. down-check *and* a CPU threshold) isn't supported yet.
- **ntfy topics are public.** Anyone who knows your `NTFY_TOPIC` string
  can read your alerts or publish fake ones to it. Treat the topic name
  itself as the only access control there is.
- **No web UI.** Every read and write goes through the CLI. Grafana is
  read-only visualization, not a management interface.
- **No natural-language rule vocabulary beyond the three presets.**
  `--check` covers cpu/memory/disk; anything else needs `--expr` and
  actual PromQL.
- **No fleet automation.** Targets are registered one `panoptes add
  target` call at a time — nothing auto-discovers or auto-registers new
  machines.
- **`--type` only really supports `node`.** The scrape config in
  `prometheus/prometheus.yml` is one hardcoded job for `node`/
  `node_exporter` — see the [`add target`](#add-target) caveat above.
  Any other `--type` writes a target file Prometheus never scrapes,
  with no error to tell you so.
