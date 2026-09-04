#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const extension = process.platform === "win32" ? ".exe" : "";
const binary = path.join(__dirname, "..", "bin", `yop-cli${extension}`);

// Intercept "install" subcommand — run the setup wizard directly,
// no binary needed. NOTE: "install" is a reserved word here; the Go
// binary must never grow a subcommand named "install" — it would be
// shadowed by this branch.
if (process.argv[2] === "install") {
  require("./install-wizard.js");
  return;
}

if (!fs.existsSync(binary)) {
  const installed = spawnSync(process.execPath, [path.join(__dirname, "install.js")], {
    stdio: "inherit",
  });
  if (installed.status !== 0) process.exit(installed.status || 1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
