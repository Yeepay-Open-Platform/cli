#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");

const root = path.join(__dirname, "..");
const version = process.argv[2];
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version || "")) {
  throw new Error("usage: node scripts/prepare-release.js <semver>");
}

const sourceChecksums = path.join(root, "dist", "checksums.txt");
if (!fs.existsSync(sourceChecksums)) {
  throw new Error("dist/checksums.txt is missing; run goreleaser first");
}

const packagePath = path.join(root, "package.json");
const manifest = JSON.parse(fs.readFileSync(packagePath, "utf8"));
manifest.version = version;
fs.writeFileSync(packagePath, `${JSON.stringify(manifest, null, 2)}\n`);
fs.copyFileSync(sourceChecksums, path.join(root, "checksums.txt"));
