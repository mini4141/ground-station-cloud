# Ground Station Cloud

[![CI](https://github.com/mini4141/ground-station-cloud/actions/workflows/ci.yml/badge.svg)](https://github.com/mini4141/ground-station-cloud/actions/workflows/ci.yml)

Ground segment as a service: satellite operators book passes on a virtual network of ground stations.
Go services compute visibility with SGP4, schedule passes and command Docker edge agents over MQTT (mutual TLS).
Agents drive a simulated modem (TCP) and antenna rotator (GS-232 over serial), stream 1 Hz telemetry and upload mission data.
A React console shows scheduling, SLAs and billing, and the station fleet is managed with a custom Terraform provider.

Status: see [PROGRESS.md](PROGRESS.md). Proof: [proof/README.md](proof/README.md). Write-up: [docs/PORTFOLIO.md](docs/PORTFOLIO.md).

Architecture, quickstart and deploy/destroy sections are filled in as phases complete.
