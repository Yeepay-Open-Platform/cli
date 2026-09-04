package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Yeepay-Open-Platform/cli/internal/config"
)

const (
	PackageName = "@yeepay/yop-cli"
	SkillsRepo  = "Yeepay-Open-Platform/cli"

	cacheTTL       = 24 * time.Hour
	fetchTimeout   = 15 * time.Second
	installTimeout = 10 * time.Minute
	skillsTimeout  = 2 * time.Minute
	verifyTimeout  = 10 * time.Second
	maxBody        = 256 << 10
)

var (
	registryBase       = "https://registry.npmjs.org/@yeepay/yop-cli"
	defaultClient      = &http.Client{Timeout: fetchTimeout}
	gitDescribePattern = regexp.MustCompile(`-\d+-g[0-9a-f]{7,}(?:-dirty)?$`)
	validPrerelease    = regexp.MustCompile(`^[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*$`)
)

type Info struct {
	Current string
	Latest  string
}

func (i *Info) Message() string {
	return fmt.Sprintf("yop-cli %s → %s available, run: yop-cli update", i.Current, i.Latest)
}

type state struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
	Channel       string `json:"channel"`
}

func ChannelForVersion(version string) string {
	parsed := parseVersion(version)
	if parsed != nil && strings.HasPrefix(parsed.prerelease, "beta.") {
		return "beta"
	}
	return "latest"
}

func StartBackgroundCheck(currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	command := exec.Command(executable, "__update-check")
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	if command.Start() == nil {
		_ = command.Process.Release()
	}
}

func CheckCached(currentVersion string) *Info {
	if shouldSkip(currentVersion) {
		return nil
	}
	var cached state
	if config.ReadJSON("update-state.json", &cached) != nil || cached.Channel != ChannelForVersion(currentVersion) {
		return nil
	}
	if !IsNewer(cached.LatestVersion, currentVersion) {
		return nil
	}
	return &Info{Current: currentVersion, Latest: cached.LatestVersion}
}

func RefreshCache(currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	_ = config.WithLock("update", func() error {
		var cached state
		channel := ChannelForVersion(currentVersion)
		if config.ReadJSON("update-state.json", &cached) == nil && cached.Channel == channel && time.Since(time.Unix(cached.CheckedAt, 0)) < cacheTTL {
			return nil
		}
		latest, err := FetchLatest(currentVersion)
		if err != nil {
			return nil
		}
		return config.WriteJSON("update-state.json", state{LatestVersion: latest, CheckedAt: time.Now().Unix(), Channel: channel})
	})
}

func shouldSkip(version string) bool {
	if os.Getenv("YOP_CLI_NO_UPDATE_NOTIFIER") != "" || version == "DEV" || version == "dev" || version == "" {
		return true
	}
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return !isRelease(version)
}

func isRelease(version string) bool {
	version = strings.TrimPrefix(version, "v")
	return parseVersion(version) != nil && !gitDescribePattern.MatchString(version)
}

func FetchLatest(currentVersion string) (string, error) {
	channel := ChannelForVersion(currentVersion)
	req, err := http.NewRequest(http.MethodGet, registryBase+"/"+channel, nil)
	if err != nil {
		return "", err
	}
	resp, err := defaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}
	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if !isRelease(result.Version) {
		return "", fmt.Errorf("npm registry returned invalid version %q", result.Version)
	}
	return result.Version, nil
}

type parsedVersion struct {
	core       [3]int
	prerelease string
}

func parseVersion(value string) *parsedVersion {
	value = strings.TrimPrefix(value, "v")
	if index := strings.Index(value, "+"); index >= 0 {
		value = value[:index]
	}
	prerelease := ""
	if index := strings.Index(value, "-"); index >= 0 {
		prerelease, value = value[index+1:], value[:index]
		if prerelease == "" || !validPrerelease.MatchString(prerelease) {
			return nil
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return nil
	}
	var core [3]int
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return nil
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return nil
		}
		core[index] = number
	}
	return &parsedVersion{core: core, prerelease: prerelease}
}

func IsNewer(candidate, current string) bool {
	left, right := parseVersion(candidate), parseVersion(current)
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	for index := range left.core {
		if left.core[index] != right.core[index] {
			return left.core[index] > right.core[index]
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease) > 0
}

func comparePrerelease(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(a) && index < len(b); index++ {
		if comparison := compareIdentifier(a[index], b[index]); comparison != 0 {
			return comparison
		}
	}
	if len(a) < len(b) {
		return -1
	}
	return 1
}

func compareIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	switch {
	case leftErr == nil && rightErr == nil:
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

type InstallMethod string

const (
	InstallNPM    InstallMethod = "npm"
	InstallPNPM   InstallMethod = "pnpm"
	InstallManual InstallMethod = "manual"
)

type Detection struct {
	Method       InstallMethod
	ResolvedPath string
	Available    bool
}

func (d Detection) CanAutoUpdate() bool { return d.Method != InstallManual && d.Available }

func DetectInstallMethod() Detection {
	executable, err := os.Executable()
	if err != nil {
		return Detection{Method: InstallManual}
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		resolved = executable
	}
	return detectFromResolved(resolved, commandExists("npm"), commandExists("pnpm"))
}

func detectFromResolved(resolved string, npmAvailable, pnpmAvailable bool) Detection {
	if !strings.Contains(filepath.ToSlash(resolved), "/node_modules/") {
		return Detection{Method: InstallManual, ResolvedPath: resolved}
	}
	if strings.Contains(filepath.ToSlash(resolved), "/.pnpm/") || strings.Contains(filepath.ToSlash(resolved), "/pnpm/") {
		return Detection{Method: InstallPNPM, ResolvedPath: resolved, Available: pnpmAvailable}
	}
	return Detection{Method: InstallNPM, ResolvedPath: resolved, Available: npmAvailable}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

type commandResult struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	err    error
}

func (r *commandResult) combined() string { return r.stdout.String() + r.stderr.String() }

func runCommand(timeout time.Duration, name string, args ...string) *commandResult {
	result := &commandResult{}
	path, err := exec.LookPath(name)
	if err != nil {
		result.err = fmt.Errorf("%s not found in PATH", name)
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, args...)
	command.Stdout, command.Stderr = &result.stdout, &result.stderr
	result.err = command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.err = fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return result
}

type Updater struct {
	detection     Detection
	backupCreated bool
}

func NewUpdater() *Updater { return &Updater{detection: DetectInstallMethod()} }

func (u *Updater) Detection() Detection { return u.detection }

func (u *Updater) Install(version string) *commandResult {
	if u.detection.Method == InstallPNPM {
		return runCommand(installTimeout, "pnpm", "add", "-g", PackageName+"@"+version)
	}
	return runCommand(installTimeout, "npm", "install", "-g", PackageName+"@"+version)
}

func (u *Updater) SyncSkills() *commandResult {
	if u.detection.Method == InstallPNPM && u.detection.Available {
		return runCommand(skillsTimeout, "pnpm", "dlx", "skills", "add", SkillsRepo, "-y", "-g")
	}
	return runCommand(skillsTimeout, "npx", "-y", "skills", "add", SkillsRepo, "-y", "-g")
}

func (u *Updater) VerifyBinary(expectedVersion string) error {
	executable, err := exec.LookPath("yop-cli")
	if err != nil {
		executable, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate binary: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, executable, "--version").Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("binary verification timed out after %s", verifyTimeout)
	}
	if err != nil {
		return fmt.Errorf("binary not executable: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 || strings.TrimPrefix(fields[len(fields)-1], "v") != strings.TrimPrefix(expectedVersion, "v") {
		return fmt.Errorf("expected version %s, got %q", expectedVersion, strings.TrimSpace(string(output)))
	}
	return nil
}

func (u *Updater) resolveExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(executable)
}

func Run(args []string, currentVersion string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	check := flags.Bool("check", false, "check without installing")
	jsonOutput := flags.Bool("json", false, "structured output")
	force := flags.Bool("force", false, "force reinstall")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: yop-cli update [--check] [--json] [--force]")
		return 1
	}
	latest, err := FetchLatest(currentVersion)
	if err != nil {
		return reportError(*jsonOutput, stdout, stderr, "network", fmt.Sprintf("failed to check latest version: %v", err), "")
	}
	updater := NewUpdater()
	detection := updater.Detection()
	if !*force && !IsNewer(latest, currentVersion) {
		var skills *commandResult
		if !*check {
			skills = updater.SyncSkills()
		}
		return reportResult(*jsonOutput, stdout, stderr, map[string]any{
			"ok": true, "action": "up_to_date", "current_version": currentVersion,
			"latest_version": latest, "channel": ChannelForVersion(currentVersion),
			"changelog": changelogURL(),
		}, "yop-cli is up to date ("+currentVersion+")", skills)
	}
	if *check {
		return reportResult(*jsonOutput, stdout, stderr, map[string]any{
			"ok": true, "action": "update_available", "current_version": currentVersion,
			"latest_version": latest, "auto_update": detection.CanAutoUpdate(),
			"channel": ChannelForVersion(currentVersion), "url": releaseURL(latest),
			"changelog": changelogURL(),
		}, fmt.Sprintf("Update available: %s → %s\n  Release:   %s\n  Changelog: %s", currentVersion, latest, releaseURL(latest), changelogURL()), nil)
	}
	updater.CleanupStaleFiles()
	if !detection.CanAutoUpdate() {
		skills := updater.SyncSkills()
		message := fmt.Sprintf("Automatic update unavailable for %s.\n  Release:   %s\n  Changelog: %s", detection.ResolvedPath, releaseURL(latest), changelogURL())
		return reportResult(*jsonOutput, stdout, stderr, map[string]any{
			"ok": true, "action": "manual_required", "current_version": currentVersion,
			"latest_version": latest, "url": releaseURL(latest), "changelog": changelogURL(),
		}, message, skills)
	}
	restore, err := updater.PrepareSelfReplace()
	if err != nil {
		return reportError(*jsonOutput, stdout, stderr, "update_error", err.Error(), "")
	}
	if !*jsonOutput {
		fmt.Fprintf(stderr, "Updating yop-cli %s → %s via %s ...\n", currentVersion, latest, detection.Method)
	}
	installed := updater.Install(latest)
	if installed.err != nil {
		restore()
		return reportError(*jsonOutput, stdout, stderr, "update_error", fmt.Sprintf("%s install failed: %v", detection.Method, installed.err), truncate(installed.combined(), 4000))
	}
	if err := updater.VerifyBinary(latest); err != nil {
		restore()
		hint := fmt.Sprintf("automatic rollback is unavailable; reinstall with %s install -g %s@%s", detection.Method, PackageName, currentVersion)
		if updater.CanRestorePreviousVersion() {
			hint = "the previous version has been restored"
		}
		return reportError(*jsonOutput, stdout, stderr, "update_error", "new binary verification failed: "+err.Error(), hint)
	}
	skills := updater.SyncSkills()
	return reportResult(*jsonOutput, stdout, stderr, map[string]any{
		"ok": true, "action": "updated", "previous_version": currentVersion,
		"current_version": latest, "latest_version": latest, "url": releaseURL(latest),
		"changelog": changelogURL(),
	}, fmt.Sprintf("Successfully updated yop-cli from %s to %s\n  Changelog: %s", currentVersion, latest, changelogURL()), skills)
}

func reportResult(asJSON bool, stdout, stderr io.Writer, result map[string]any, message string, skills *commandResult) int {
	if skills != nil {
		result["skills_updated"] = skills.err == nil
		if skills.err != nil {
			result["ok"] = false
			result["skills_error"] = truncate(skills.combined()+skills.err.Error(), 4000)
		}
	}
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(result)
	} else {
		fmt.Fprintln(stderr, message)
		if skills != nil {
			if skills.err == nil {
				fmt.Fprintln(stderr, "Skills updated")
			} else {
				fmt.Fprintf(stderr, "Skills update failed: %v\n", skills.err)
			}
		}
	}
	if ok, _ := result["ok"].(bool); !ok {
		return 1
	}
	return 0
}

func reportError(asJSON bool, stdout, stderr io.Writer, kind, message, detail string) int {
	if asJSON {
		errorValue := map[string]any{"type": kind, "message": message}
		if detail != "" {
			errorValue["detail"] = detail
		}
		_ = json.NewEncoder(stdout).Encode(map[string]any{"ok": false, "error": errorValue})
	} else {
		fmt.Fprintln(stderr, message)
		if detail != "" {
			fmt.Fprintln(stderr, detail)
		}
	}
	return 1
}

func releaseURL(version string) string {
	return "https://github.com/Yeepay-Open-Platform/cli/releases/tag/v" + strings.TrimPrefix(version, "v")
}

func changelogURL() string {
	return "https://github.com/Yeepay-Open-Platform/cli/blob/main/CHANGELOG.md"
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "..."
}
