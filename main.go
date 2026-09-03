package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Yeepay-Open-Platform/cli/internal/config"
	"github.com/Yeepay-Open-Platform/cli/internal/telemetry"
)

const commandName = "yop-cli"

var (
	version          = "dev"
	telemetryWebhook = ""
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "__track-send" {
		telemetry.Send()
		return 0
	}
	printTelemetryNoticeOnce(stderr)
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Fprintf(stdout, "%s %s\n", commandName, version)
		return 0
	}
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "track":
		return runTrack(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 1
	}
}

func printTelemetryNoticeOnce(stderr io.Writer) {
	shouldPrint := false
	_ = config.WithLock("notice", func() error {
		if config.Exists("notice.json") {
			return nil
		}
		marker := struct {
			TelemetryDisclosed bool `json:"telemetry_disclosed"`
		}{TelemetryDisclosed: true}
		if err := config.WriteJSON("notice.json", marker); err != nil {
			return err
		}
		shouldPrint = true
		return nil
	})
	if shouldPrint {
		fmt.Fprintln(stderr, "提示：yop-cli 会发送匿名 skill 使用事件（不采集业务身份）；可运行 `yop-cli config telemetry off` 完全关闭。")
	}
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if isHelp(args) {
		printConfigHelp(stdout)
		return 0
	}
	if len(args) == 2 && isHelp(args[1:]) {
		switch args[0] {
		case "set":
			fmt.Fprintln(stdout, "Usage: yop-cli config set <key> <value>")
			return 0
		case "get":
			fmt.Fprintln(stdout, "Usage: yop-cli config get <key>")
			return 0
		case "telemetry":
			fmt.Fprintln(stdout, "Usage: yop-cli config telemetry <on|off>")
			return 0
		}
	}
	if len(args) == 2 && args[0] == "get" {
		if value, ok := config.Load()[args[1]]; ok {
			fmt.Fprintln(stdout, value)
		}
		return 0
	}
	if len(args) == 3 && args[0] == "set" {
		err := config.WithLock("config", func() error {
			values := config.Load()
			values[args[1]] = args[2]
			return config.Save(values)
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if len(args) == 2 && args[0] == "telemetry" && (args[1] == "on" || args[1] == "off") {
		err := config.WithLock("config", func() error {
			values := config.Load()
			values["telemetry.enabled"] = fmt.Sprintf("%t", args[1] == "on")
			if err := config.Save(values); err != nil {
				return err
			}
			if args[1] == "off" {
				return config.WriteJSON("telemetry-disabled", true)
			}
			return config.Remove("telemetry-disabled")
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(stderr, "usage: yop-cli config set <key> <value> | get <key> | telemetry <on|off>")
	return 1
}

func runTrack(args []string, stdout, stderr io.Writer) int {
	if isHelp(args) {
		printTrackHelp(stdout)
		return 0
	}
	flags := flag.NewFlagSet("track", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := telemetry.Input{}
	flags.StringVar(&input.Skill, "skill", "", "skill name")
	flags.StringVar(&input.EventType, "event", "", "event type")
	flags.StringVar(&input.SkillVersion, "skill-version", "", "skill version")
	flags.StringVar(&input.Props, "props", "", "flat JSON properties")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		if os.Getenv("YOP_TELEMETRY_DEBUG") == "1" {
			fmt.Fprintln(stderr, "telemetry dropped: invalid track arguments")
		}
		return 0
	}
	executable, err := os.Executable()
	if err != nil {
		return 0
	}
	telemetry.Track(input, version, telemetryWebhook, executable, stderr)
	return 0
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`yop-cli - Yeepay Open Platform CLI

Usage:
  yop-cli --version
  yop-cli config set <key> <value>
  yop-cli config get <key>
  yop-cli config telemetry <on|off>
  yop-cli track --skill <name> --event <type> [--skill-version <version>] [--props <json>]`))
}

func printConfigHelp(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`Manage persisted configuration.

Usage:
  yop-cli config set <key> <value>
  yop-cli config get <key>
  yop-cli config telemetry <on|off>`))
}

func printTrackHelp(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`Send a non-blocking anonymous skill telemetry event.

Usage:
  yop-cli track --skill <name> --event <type> [--skill-version <version>] [--props <json>]

Events: skill_start, skill_end, skill_error, custom`))
}

func isHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")
}
