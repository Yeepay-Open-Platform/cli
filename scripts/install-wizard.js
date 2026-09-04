#!/usr/bin/env node

const { execFile } = require("child_process");

// @clack/prompts is ESM-only since v1; load it via dynamic import() so this
// CommonJS script works on all supported Node versions (require() of an ESM
// package throws ERR_REQUIRE_ESM before Node 22.12). Assigned in the entry
// point below before main() runs.
let p;

const SKILLS_REPO = "Yeepay-Open-Platform/cli";
const SKILL_NAME_PATTERN = /yeepay-/;
// eslint-disable-next-line no-control-regex
const ANSI_PATTERN = /\x1b\[[0-9;]*m/g;
const isWindows = process.platform === "win32";

// ---------------------------------------------------------------------------
// i18n
// ---------------------------------------------------------------------------

const messages = {
  zh: {
    setup:          "正在设置 Yop CLI...",
    step2:          "安装 AI Skills",
    step2Skip:      "已安装，跳过",
    step2Spinner:   "正在安装 Skills...",
    step2Done:      "Skills 已安装",
    step2Fail:      "Skills 安装失败。运行以下命令重试: npx skills add %s -y -g",
    done:           "安装完成！",
    cancelled:      "安装已取消",
  },
  en: {
    setup:          "Setting up Yop CLI...",
    step2:          "Install AI skills",
    step2Skip:      "Already installed. Skipped",
    step2Spinner:   "Installing skills...",
    step2Done:      "Skills installed",
    step2Fail:      "Failed to install skills. Run manually: npx skills add %s -y -g",
    done:           "You are all set!",
    cancelled:      "Installation cancelled",
  },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function handleCancel(value, msg) {
  if (p.isCancel(value)) {
    p.cancel(msg.cancelled);
    process.exit(0);
  }
  return value;
}

function runSilentAsync(cmd, args, opts = {}) {
  const actualCmd = isWindows ? "cmd.exe" : cmd;
  const actualArgs = isWindows ? ["/c", cmd, ...args] : args;
  return new Promise((resolve, reject) => {
    execFile(actualCmd, actualArgs, {
      stdio: ["ignore", "pipe", "pipe"],
      ...opts,
    }, (err, stdout) => {
      if (err) reject(err);
      else resolve(stdout);
    });
  });
}

function fmt(template, ...values) {
  let i = 0;
  return template.replace(/%s/g, () => values[i++] ?? "");
}

/** Parse --lang from process.argv, returns "zh", "en", or null. */
function parseLangArg() {
  const args = process.argv.slice(2);
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--lang" && args[i + 1]) {
      const val = args[i + 1].toLowerCase();
      if (val === "zh" || val === "en") return val;
    }
    if (args[i].startsWith("--lang=")) {
      const val = args[i].split("=")[1].toLowerCase();
      if (val === "zh" || val === "en") return val;
    }
  }
  return null;
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

async function stepSelectLang() {
  const fromArg = parseLangArg();
  if (fromArg) return fromArg;

  const lang = await p.select({
    message: "请选择语言 / Select language",
    options: [
      { value: "zh", label: "中文" },
      { value: "en", label: "English" },
    ],
  });
  return handleCancel(lang, messages.zh);
}

async function skillsAlreadyInstalled() {
  try {
    const out = await runSilentAsync("npx", ["-y", "skills", "ls", "-g"], {
      timeout: 120000,
    });
    return SKILL_NAME_PATTERN.test(out.toString().replace(ANSI_PATTERN, ""));
  } catch (_) {
    return false;
  }
}

async function stepInstallSkills(msg) {
  const s = p.spinner();
  s.start(msg.step2Spinner);
  try {
    if (await skillsAlreadyInstalled()) {
      s.stop(msg.step2Skip);
      return;
    }
    await runSilentAsync("npx", ["-y", "skills", "add", SKILLS_REPO, "-y", "-g"], {
      timeout: 120000,
    });
    s.stop(msg.step2Done);
  } catch (_) {
    s.stop(fmt(msg.step2Fail, SKILLS_REPO));
    process.exit(1);
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const isInteractive = !!process.stdin.isTTY;
  const lang = isInteractive ? await stepSelectLang() : (parseLangArg() || "en");
  const msg = messages[lang];

  if (isInteractive) {
    p.intro(msg.setup);
    await stepInstallSkills(msg);
    p.outro(msg.done);
  } else {
    console.log(msg.setup);
    await stepInstallSkills(msg);
  }
}

(async () => {
  p = await import("@clack/prompts");
  await main();
})().catch((err) => {
  const msg = "Unexpected error: " + (err.message || err);
  if (p) p.cancel(msg);
  else console.error(msg);
  process.exit(1);
});
