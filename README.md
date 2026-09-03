# YOP CLI

易宝开放平台命令行工具。

## 功能

- **版本查询**：`yop-cli --version` / `version`。
- **配置管理**：`config set/get` 读写持久化配置；遵循系统用户配置目录约定，可用 `YOP_CONFIG_DIR` 覆盖。
- **埋点开关**：`config telemetry on|off` 一键开关遥测，关闭后写入禁用标记彻底停止采集。
- **匿名 Skill 埋点**：`track --skill --event [--skill-version] [--props]` 上报 `skill_start/skill_end/skill_error/custom` 事件；非阻塞异步发送，失败静默不影响主流程；自动脱敏手机号和邮箱，不采集业务身份；webhook 通过 ldflags 或 `YOP_TELEMETRY_WEBHOOK` 注入。
- **Skill 生态**：skill 不编译进二进制，通过 Agent Skills CLI 分发；`go test ./...` 统一校验 frontmatter、依赖闭包/环和 `cliHelp` 对应的真实命令。
- **跨平台分发**：单一 Go 二进制支持 darwin/linux/windows（x64/arm64），npm 包 postinstall 按平台安装并用 `checksums.txt` 校验完整性。

## 构建

需要 Go 1.23 或更高版本：

```bash
./build.sh
./yop-cli --version
```

发布时通过 ldflags 注入版本和多维表格自动化 webhook：

```bash
go build -ldflags "-X main.version=1.0.0 -X main.telemetryWebhook=https://example.test/hook" -o yop-cli .
```

GoReleaser 完成后会同步 npm 包版本和 `checksums.txt`，再执行 `npm pack`/`npm publish`。

## 配置

```bash
yop-cli config set telemetry.webhook https://example.test/hook
yop-cli config get telemetry.webhook
yop-cli config telemetry off
yop-cli config telemetry on
```

配置遵循系统用户配置目录约定；开发与测试可通过 `YOP_CONFIG_DIR` 指定独立目录。

## Skill

Skill 不编译进二进制，使用 Agent Skills CLI 从仓库下载。当前 skill 内容尚未建设，仅保留埋点试点占位：

```bash
npx -y skills add https://github.com/Yeepay-Open-Platform/cli -g -y
```

埋点接入方式见 [Skill 埋点规范](docs/skill-telemetry.md)。

每个 skill 必须声明 `metadata.requires.bins: ["yop-cli"]` 和可执行的 `metadata.cliHelp`。新增 skill 从 [`skill-template/SKILL.md.tmpl`](skill-template/SKILL.md.tmpl) 开始；`go test ./...` 会统一校验 frontmatter、skill 依赖闭包/环，以及 `cliHelp` 对应的真实命令。
