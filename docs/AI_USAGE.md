# AI usage

This repository was built with Claude Code (Anthropic) working autonomously from `CLAUDE.md` and `SPECS.md`.

- AI wrote the Go services, edge agent, simulators, Terraform (AWS + provider), React console, scripts and docs.
- Review: every layer has a shallow happy-path test (Go unit/integration, agent e2e, provider acceptance, Playwright, k6). `scripts/test-all.sh` is the gate; CI re-runs the cloud-free layers on every push.
- Generated code (oapi-codegen, sqlc, openapi-typescript) is checked for drift in CI, so hand edits cannot hide.
- Cloud behaviour was verified by running the deployed stack and capturing CLI/screenshot proof under `proof/`; resources were destroyed and verified afterwards.
