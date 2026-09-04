# yop-cli

> 易宝开放平台（YeePay）官方命令行工具 —— 面向商户开发者和 Coding Agent。
> 对外发布：[Yeepay-Open-Platform/cli](https://github.com/Yeepay-Open-Platform/cli)

---

## 这是什么

yop-cli 由两部分组成：**Skills** 与 **CLI**。

**Skills 是核心。** 本仓库的 `skills/` 收录易宝支付提供的 Agent Skills，旨在帮助开发者和技术团队快速理解、集成并应用易宝支付的各项产品能力，提升 AI Agent 在支付场景下的开发效率。安装后，在对话中提及对应技能覆盖的业务关键词时，Agent 会自动加载相应 `SKILL.md` 并按其中纪律协助完成选型、接入与排障。

**CLI 让 Agent 直接落地执行。** AI Agent 通过 Skill + 单一静态二进制即可完成与易宝开放平台的能力对接，轻量接入、无需额外部署服务组件。CLI 当前提供配置管理等基础能力，未来将持续补充直连开放平台 API 的命令（详见 [路线图](#路线图)）。

## 核心能力

- **为 Agent 原生设计** — Skills 开箱即用，适配主流 AI 编程工具，Agent 无需额外适配即可协助商户接入易宝
- **覆盖支付全场景** — 入网 / 收单 / 退款 / 分账 / 出款 / 对账 / 金融 / 航旅易达，沉淀易宝开放平台最佳实践
- **面客交互纪律** — Skill 内置选型、确认、排障的标准交互流程，以商户可理解的语言沟通，避免直接输出接口字段清单
- **接入零门槛** — 单一静态二进制分发，支持 macOS / Linux / Windows（x64 / arm64），`npx` 一条命令即可使用
- **轻量可扩展** — Skill 不编译进二进制，独立分发、独立迭代，新增技能不影响已有能力

## Skills 列表

| 技能 ID | 版本 | 说明 | 文档 |
| ------- | ---- | ---- | ---- |
| `yeepay-payment-integration` | [1.3.0](./skills/yeepay-payment-integration/SKILL.md) | 易宝支付接入、联调与排障（入网 / 收单 / 退款 / 分账 / 出款 / 对账 / 金融 / 航旅易达机票） | [SKILL.md](./skills/yeepay-payment-integration/SKILL.md) |

## 安装与快速开始

### 环境要求

- Node.js `v20.12`+（`npm`/`npx`）
- Go `v1.23`+（仅源码构建需要）

### 快速开始（AI Agent）

> 以下步骤面向 AI Agent，均可直接执行完成。

**第 1 步 — 安装**

```bash
npx -y @yeepay/yop-cli@latest install
```

**第 2 步 — 验证**

```bash
yop-cli --version
```

安装完成后，在对话中描述支付接入需求（如「我的小程序要接易宝支付」），Agent 会自动加载对应 Skill 并协助完成接入。

### 从源码构建

```bash
git clone https://github.com/Yeepay-Open-Platform/cli.git
cd cli
./build.sh
./yop-cli --version
```

发布流程：GoReleaser 构建多平台二进制（macOS / Linux / Windows，x64 / arm64）并同步 `checksums.txt`，npm 包 postinstall 按平台下载对应二进制并校验完整性。

## CLI 命令

| 命令 | 描述 |
| ---- | ---- |
| `yop-cli --version` | 查看版本 |
| `yop-cli config set <key> <value>` | 写入持久化配置 |
| `yop-cli config get <key>` | 读取配置 |

配置遵循系统用户配置目录约定；开发与测试可通过 `YOP_CONFIG_DIR` 指定独立目录。

## 目录结构

```text
cli/                                  仓库根目录
├── README.md                         本文件
├── main.go                           CLI 入口
├── internal/                         CLI 内部实现
├── skills/
│   └── yeepay-payment-integration/   技能包（Agent 加载此目录）
│       ├── SKILL.md
│       ├── scripts/
│       └── references/
├── skill-template/                   新增 Skill 的模板
└── docs/
```

后续新增技能时，在 `skills/` 下增加同级目录即可，无需改动已有技能包。

## 路线图

yop-cli 目前处于早期阶段，核心方向：

1. **Skill 生态** — 持续补充各业务域的接入 Skill（当前聚焦支付接入）
2. **API 直连命令** — 补齐开放平台接口的命令行直连能力，减少对文档查阅和手写 curl 的依赖
3. **商户身份与凭证管理** — 安全的商户号 / 密钥配置与多环境（沙箱 / 生产）切换

## 隐私说明

yop-cli 包含可选的匿名使用统计，用于改进产品体验；不涉及任何业务身份信息。可通过 `yop-cli config telemetry off` 一键关闭，`yop-cli config telemetry on` 重新开启。

## 参与贡献

欢迎通过 Issue 或 Pull Request 反馈问题与建议。

新增 Skill 从 [`skill-template/SKILL.md.tmpl`](skill-template/SKILL.md.tmpl) 开始：每个 Skill 必须声明 `metadata.requires.bins: ["yop-cli"]` 和可执行的 `metadata.cliHelp`；`go test ./...` 统一校验 frontmatter、依赖闭包与 `cliHelp` 对应的真实命令。

## 相关链接

| 链接 | 说明 |
| ---- | ---- |
| [易宝开放平台](https://open.yeepay.com) | 商户入驻、应用与密钥管理 |
| [GitHub 仓库](https://github.com/Yeepay-Open-Platform/cli) | 源码与发版 |
| [GitHub Issues](https://github.com/Yeepay-Open-Platform/cli/issues) | 问题反馈与建议 |
