#!/usr/bin/env node

const crypto = require("node:crypto");
const fs = require("node:fs");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const version = require("../package.json").version;
const platforms = { darwin: "darwin", linux: "linux", win32: "windows" };
const architectures = { x64: "amd64", arm64: "arm64" };
const platform = platforms[process.platform];
const architecture = architectures[process.arch];

if (!platform || !architecture) {
  throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);
}

const extension = platform === "windows" ? ".zip" : ".tar.gz";
const archive = `yop-cli-${version}-${platform}-${architecture}${extension}`;
const base = (process.env.YOP_CLI_DOWNLOAD_BASE ||
  `https://github.com/Yeepay-Open-Platform/cli/releases/download/v${version}`).replace(/\/$/, "");
const url = `${base}/${archive}`;
const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "yop-cli-"));
const archivePath = path.join(tempDir, archive);
const destinationDir = path.join(__dirname, "..", "bin");
const binaryName = `yop-cli${platform === "windows" ? ".exe" : ""}`;

function download(source, destination, redirects = 0) {
  return new Promise((resolve, reject) => {
    let output;
    let request;
    const deadline = setTimeout(() => {
      request.destroy(new Error("Download timed out after 120 seconds"));
    }, 120_000);
    const fail = (error) => {
      clearTimeout(deadline);
      if (output) output.destroy();
      reject(error);
    };
    request = https.get(source, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location && redirects < 3) {
        clearTimeout(deadline);
        response.resume();
        resolve(download(new URL(response.headers.location, source).toString(), destination, redirects + 1));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        fail(new Error(`Download failed with HTTP ${response.statusCode}`));
        return;
      }
      output = fs.createWriteStream(destination, { mode: 0o600 });
      response.pipe(output);
      output.on("finish", () => output.close(() => {
        clearTimeout(deadline);
        resolve();
      }));
      output.on("error", (error) => {
        request.destroy();
        fail(error);
      });
    });
    request.setTimeout(10_000, () => request.destroy(new Error("Download connection timed out")));
    request.on("error", fail);
  });
}

function expectedChecksum() {
  const lines = fs.readFileSync(path.join(__dirname, "..", "checksums.txt"), "utf8").split("\n");
  const match = lines.find((line) => line.trim().endsWith(`  ${archive}`));
  if (!match) throw new Error(`Checksum not found for ${archive}`);
  return match.trim().split(/\s+/)[0];
}

function verifyChecksum(file, expected) {
  const actual = crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
  if (actual !== expected) throw new Error(`Checksum mismatch for ${archive}`);
}

async function install() {
  try {
    await download(url, archivePath);
    verifyChecksum(archivePath, expectedChecksum());
    fs.mkdirSync(destinationDir, { recursive: true });
    if (platform === "windows") {
      execFileSync("powershell.exe", ["-NoProfile", "-Command", "Expand-Archive", "-LiteralPath", archivePath, "-DestinationPath", tempDir, "-Force"]);
    } else {
      execFileSync("tar", ["-xzf", archivePath, "-C", tempDir]);
    }
    const destination = path.join(destinationDir, binaryName);
    fs.copyFileSync(path.join(tempDir, binaryName), destination);
    fs.chmodSync(destination, 0o755);
    console.log(`yop-cli ${version} installed`);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

install().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
