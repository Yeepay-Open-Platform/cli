# Runtime updates may promote npx use to a global install

`yop-cli update` detects npm or pnpm package paths and installs the selected version globally, then verifies the replacement binary and synchronizes global Skills. This deliberately trades npx-only purity for a recoverable, directly invocable self-update path; `--check` skips installation and Skills synchronization but may write the one-time telemetry disclosure marker, and manual binaries receive a Release URL instead of being overwritten.
