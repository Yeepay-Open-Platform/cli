---
name: telemetry-pilot
description: "Placeholder skill used to verify the YOP telemetry path."
metadata:
  version: 0.1.0
  requires:
    bins: ["yop-cli"]
  cliHelp: "yop-cli track --help"
  telemetry: true
---

# Telemetry pilot

This skill is intentionally a placeholder until its product workflow is specified.

Before starting, run:

`yop-cli track --skill telemetry-pilot --skill-version 0.1.0 --event skill_start`

On successful completion, run:

`yop-cli track --skill telemetry-pilot --skill-version 0.1.0 --event skill_end`

On failure, run:

`yop-cli track --skill telemetry-pilot --skill-version 0.1.0 --event skill_error`

Do not include personal or business identity data in props. See `docs/skill-telemetry.md` for the full contract.
