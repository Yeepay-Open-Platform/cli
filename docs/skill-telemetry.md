# Skill 埋点规范

Skill 自行决定埋点位置；CLI 只约束事件格式与调用方式。每个已接入的 `SKILL.md` 在 frontmatter 中声明：

```yaml
metadata:
  version: 1.0.0
  requires:
    bins: ["yop-cli"]
  cliHelp: "yop-cli <command> --help"
  telemetry: true
```

## 标准片段

把下面的要求放入 skill 指令，并把 `<skill-name>` 和版本替换为实际值：

```markdown
执行 skill 的实质工作前调用（默认只报这一次）：
`yop-cli track --skill <skill-name> --skill-version <version> --event skill_start`

仅当 skill 是多步流程、需要观测成功率/耗时/失败归因时，才补充：
成功完成后调用：
`yop-cli track --skill <skill-name> --skill-version <version> --event skill_end`
执行失败时调用：
`yop-cli track --skill <skill-name> --skill-version <version> --event skill_error`

关键业务动作可选调用：
`yop-cli track --skill <skill-name> --skill-version <version> --event custom --props '{"action":"<snake_case_action>"}'`
```

`track` 始终立即成功返回，网络失败不会中断 skill。默认只埋 `skill_start`；`skill_end`、`skill_error` 仅用于需要成功率/耗时/失败观测的多步流程。业务动作统一使用 `custom`。

## Props 与合规

- props 必须是单层 JSON 对象，key 使用 snake_case，最多 10 个 key。
- 值最长 200 个字符；对象、数组和不合规 key 会被丢弃。
- CLI 会掩码手机号和邮箱，但 skill 作者仍不得传入姓名、证件号、商户号等身份或业务敏感信息。
- `install_id` 是随机 UUID，只用于匿名 UV，禁止与业务身份关联。
- 同一 skill 的同一事件类型在 60 秒内只发送一次；`custom` 事件按 `props.action` 各自独立去重（无 action 的 custom 事件互相去重）。
- 用户可通过 `YOP_TELEMETRY=0` 或 `yop-cli config telemetry off` 完全关闭上报。
