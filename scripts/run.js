#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const extension = process.platform === "win32" ? ".exe" : "";
const binary = path.join(__dirname, "..", "bin", `yop-cli${extension}`);

// Recover a Windows self-update interrupted after the running binary was
// renamed but before npm installed its replacement.
const oldBinary = `${binary}.old`;
function restoreOldBinary() {
  try {
    fs.rmSync(binary, { force: true });
    fs.renameSync(oldBinary, binary);
  } catch (_) {
    // Best effort; the normal missing-binary path below still attempts repair.
  }
}
if (process.platform === "win32" && fs.existsSync(oldBinary)) {
  if (!fs.existsSync(binary)) {
    restoreOldBinary();
  } else if (spawnSync(binary, ["--version"], { stdio: "ignore", timeout: 10_000 }).status === 0) {
    fs.rmSync(oldBinary, { force: true });
  } else {
    restoreOldBinary();
  }
}

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
