# <Panoptes>

Prometheus, Grafana and Alertmanager, wired together but watching
nothing until you tell them to. A CLI called `monictl` is the only door
in: it adds targets, it adds alert rules, nothing else touches the
config.

## Prerequisites
- Docker + Docker Compose