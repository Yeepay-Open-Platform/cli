package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yeepay-Open-Platform/cli/internal/skillmeta"
)

var (
	testBinary string
	buildOnce  sync.Once
	buildErr   error
)

func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "yop-cli-test-")
		if err != nil {
			buildErr = err
			return
		}
		testBinary = filepath.Join(dir, "yop-cli")
		cmd := exec.Command("go", "build", "-ldflags", "-X github.com/Yeepay-Open-Platform/cli/internal/build.Version=1.2.3", "-o", testBinary, ".")
		cmd.Dir = "."
		if output, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %w\n%s", err, output)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return testBinary
}

func runCLI(t *testing.T, configDir string, extraEnv []string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binary(t), args...)
	cmd.Env = append(os.Environ(), append([]string{"YOP_CONFIG_DIR=" + configDir, "YOP_CLI_NO_UPDATE_NOTIFIER=1"}, extraEnv...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestVersionAndConfigPersistAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	stdout, _, err := runCLI(t, dir, nil, "--version")
	if err != nil || strings.TrimSpace(stdout) != "yop-cli 1.2.3" {
		t.Fatalf("--version: stdout=%q err=%v", stdout, err)
	}

	if _, _, err := runCLI(t, dir, nil, "config", "set", "telemetry.webhook", "https://example.test/hook"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	stdout, _, err = runCLI(t, dir, nil, "config", "get", "telemetry.webhook")
	if err != nil || strings.TrimSpace(stdout) != "https://example.test/hook" {
		t.Fatalf("config get: stdout=%q err=%v", stdout, err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var config map[string]string
	if err := json.Unmarshal(raw, &config); err != nil || config["telemetry.webhook"] != "https://example.test/hook" {
		t.Fatalf("config file = %s, err=%v", raw, err)
	}
}

func TestCorruptConfigBehavesAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCLI(t, dir, nil, "config", "get", "missing")
	if err != nil || stdout != "" {
		t.Fatalf("get corrupt config: stdout=%q err=%v", stdout, err)
	}
}

type receivedEvent struct {
	EventID      string         `json:"event_id"`
	Timestamp    int64          `json:"ts"`
	EventType    string         `json:"event_type"`
	Skill        string         `json:"skill"`
	SkillVersion string         `json:"skill_version"`
	CLIVersion   string         `json:"cli_version"`
	OS           string         `json:"os"`
	Arch         string         `json:"arch"`
	InstallID    string         `json:"install_id"`
	Props        map[string]any `json:"props"`
}

func eventServer(t *testing.T, delay time.Duration) (*httptest.Server, <-chan receivedEvent) {
	t.Helper()
	events := make(chan receivedEvent, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		defer r.Body.Close()
		var event receivedEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("decode event: %v", err)
			return
		}
		events <- event
		w.WriteHeader(http.StatusNoContent)
	}))
	return server, events
}

func configureWebhook(t *testing.T, dir, webhook string) {
	t.Helper()
	if _, _, err := runCLI(t, dir, nil, "config", "set", "telemetry.webhook", webhook); err != nil {
		t.Fatalf("configure webhook: %v", err)
	}
}

func receiveEvent(t *testing.T, events <-chan receivedEvent) receivedEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for telemetry event")
		return receivedEvent{}
	}
}

func expectNoEvent(t *testing.T, events <-chan receivedEvent) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected telemetry event: %+v", event)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTrackReturnsBeforeWebhookAndSendsEnvelope(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 900*time.Millisecond)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	started := time.Now()
	_, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start", "--skill-version", "2.0.0", "--props", `{"action":"open","event_id":"caller"}`)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("track waited for webhook: %s", elapsed)
	}

	event := receiveEvent(t, events)
	if event.EventID == "" || event.EventID == "caller" || event.Timestamp == 0 || event.InstallID == "" {
		t.Fatalf("system envelope fields missing or overridden: %+v", event)
	}
	if event.EventType != "skill_start" || event.Skill != "demo" || event.SkillVersion != "2.0.0" || event.CLIVersion != "1.2.3" || event.OS != runtime.GOOS || event.Arch != runtime.GOARCH {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	if event.Props["action"] != "open" {
		t.Fatalf("props = %#v", event.Props)
	}
}

func TestTelemetryOptOutsSendNoRequests(t *testing.T) {
	server, events := eventServer(t, 0)
	defer server.Close()

	for _, test := range []struct {
		name string
		off  func(*testing.T, string)
	}{
		{name: "environment", off: func(t *testing.T, dir string) {
			_, _, err := runCLI(t, dir, []string{"YOP_TELEMETRY=0"}, "track", "--skill", "demo", "--event", "skill_start")
			if err != nil {
				t.Fatalf("track: %v", err)
			}
		}},
		{name: "config", off: func(t *testing.T, dir string) {
			if _, _, err := runCLI(t, dir, nil, "config", "telemetry", "off"); err != nil {
				t.Fatalf("telemetry off: %v", err)
			}
			if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
				t.Fatalf("track: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			configureWebhook(t, dir, server.URL)
			test.off(t, dir)
			expectNoEvent(t, events)
		})
	}
}

func TestFirstRunTelemetryNoticeAppearsOnce(t *testing.T) {
	dir := t.TempDir()
	_, firstStderr, err := runCLI(t, dir, nil, "--version")
	if err != nil {
		t.Fatal(err)
	}
	_, secondStderr, err := runCLI(t, dir, nil, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(firstStderr, "yop-cli config telemetry off") || !strings.Contains(firstStderr, "匿名") {
		t.Fatalf("first-run notice missing disclosure or opt-out: %q", firstStderr)
	}
	if secondStderr != "" {
		t.Fatalf("notice repeated: %q", secondStderr)
	}
}

func TestTrackDebouncesCustomEventsByAction(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	track := func(props string) {
		t.Helper()
		args := []string{"track", "--skill", "demo", "--event", "custom"}
		if props != "" {
			args = append(args, "--props", props)
		}
		if _, _, err := runCLI(t, dir, nil, args...); err != nil {
			t.Fatal(err)
		}
	}

	track(`{"action":"scenario_confirmed"}`)
	if got := receiveEvent(t, events).Props["action"]; got != "scenario_confirmed" {
		t.Fatalf("first action = %v", got)
	}
	track(`{"action":"script_run"}`)
	if got := receiveEvent(t, events).Props["action"]; got != "script_run" {
		t.Fatalf("distinct action was debounced away: %v", got)
	}
	track(`{"action":"scenario_confirmed"}`)
	expectNoEvent(t, events)
	track(`{"note":"no action key"}`)
	receiveEvent(t, events)
	track(`{"note":"still no action key"}`)
	expectNoEvent(t, events)
}

func TestTrackDebouncesAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	for range 2 {
		if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
			t.Fatal(err)
		}
	}
	first := receiveEvent(t, events)
	expectNoEvent(t, events)

	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "other", "--event", "skill_start"); err != nil {
		t.Fatal(err)
	}
	second := receiveEvent(t, events)
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_end"); err != nil {
		t.Fatal(err)
	}
	third := receiveEvent(t, events)
	if first.InstallID != second.InstallID || first.InstallID != third.InstallID {
		t.Fatalf("install_id changed across processes: %q, %q, %q", first.InstallID, second.InstallID, third.InstallID)
	}
}

func TestTrackSanitizesPropsAndSilentlyDropsInvalidEvents(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	stdout, stderr, err := runCLI(t, dir, []string{"YOP_TELEMETRY_DEBUG=1"}, "track", "--skill", "demo", "--event", "not_valid")
	if err != nil || stdout != "" || !strings.Contains(stderr, "dropped") {
		t.Fatalf("invalid event: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	expectNoEvent(t, events)

	props := map[string]any{
		"email":      "alice@example.com",
		"phone":      "13812345678",
		"long_value": strings.Repeat("界", 205),
		"Bad-Key":    "drop",
		"nested":     map[string]any{"secret": "drop"},
		"array":      []any{"drop"},
	}
	for index := 0; index < 12; index++ {
		props[fmt.Sprintf("value_%02d", index)] = index
	}
	raw, err := json.Marshal(props)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "custom", "--props", string(raw)); err != nil {
		t.Fatal(err)
	}
	event := receiveEvent(t, events)
	if len(event.Props) != 10 {
		t.Fatalf("props count = %d, want 10: %#v", len(event.Props), event.Props)
	}
	if event.Props["email"] != "a***@example.com" || event.Props["phone"] != "138****5678" {
		t.Fatalf("PII was not masked: %#v", event.Props)
	}
	if len([]rune(event.Props["long_value"].(string))) != 200 {
		t.Fatalf("long value was not truncated")
	}
	if _, exists := event.Props["Bad-Key"]; exists {
		t.Fatal("invalid key was retained")
	}
	if _, exists := event.Props["nested"]; exists {
		t.Fatal("nested value was retained")
	}
}

func TestTrackFailureWritesDeadletterOnlyInDebugMode(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			dir := t.TempDir()
			server, _ := eventServer(t, 0)
			webhook := server.URL
			server.Close()
			configureWebhook(t, dir, webhook)
			env := []string{}
			if debug {
				env = append(env, "YOP_TELEMETRY_DEBUG=1")
			}
			if _, _, err := runCLI(t, dir, env, "track", "--skill", "demo", "--event", "skill_error"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "telemetry-deadletter.log")
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				_, err := os.Stat(path)
				if err == nil || !debug {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			_, err := os.Stat(path)
			if debug && err != nil {
				t.Fatalf("debug deadletter missing: %v", err)
			}
			if !debug && !os.IsNotExist(err) {
				t.Fatalf("deadletter exists without debug: %v", err)
			}
		})
	}
}

func TestTrackSendsAfterDebounceWindowAndRecoversFromCorruptState(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
		t.Fatal(err)
	}
	receiveEvent(t, events)

	statePath := filepath.Join(dir, "telemetry-state.json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]int64{}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	for key := range state {
		state[key] = time.Now().Add(-61 * time.Second).UnixMilli()
	}
	raw, _ = json.Marshal(state)
	if err := os.WriteFile(statePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
		t.Fatal(err)
	}
	receiveEvent(t, events)

	if err := os.WriteFile(statePath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
		t.Fatal(err)
	}
	receiveEvent(t, events)
}

func TestConcurrentTrackCallsShareInstallIDAndDebounce(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)

	commands := make([]*exec.Cmd, 8)
	for index := range commands {
		commands[index] = exec.Command(binary(t), "track", "--skill", "demo", "--event", "skill_start")
		commands[index].Env = append(os.Environ(), "YOP_CONFIG_DIR="+dir)
		if err := commands[index].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	first := receiveEvent(t, events)
	expectNoEvent(t, events)

	for index := range 4 {
		if _, _, err := runCLI(t, dir, nil, "track", "--skill", fmt.Sprintf("skill_%d", index), "--event", "skill_start"); err != nil {
			t.Fatal(err)
		}
	}
	for range 4 {
		event := receiveEvent(t, events)
		if event.InstallID != first.InstallID {
			t.Fatalf("concurrent install_id changed: %q != %q", event.InstallID, first.InstallID)
		}
	}
}

func TestTelemetryOptOutSurvivesCorruptConfig(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)
	if _, _, err := runCLI(t, dir, nil, "config", "telemetry", "off"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "skill_start"); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, events)
}

func TestTrackMasksEmailBeforeTruncation(t *testing.T) {
	dir := t.TempDir()
	server, events := eventServer(t, 0)
	defer server.Close()
	configureWebhook(t, dir, server.URL)
	props := fmt.Sprintf(`{"contact":%q}`, strings.Repeat("x", 190)+"alice@example.com")
	if _, _, err := runCLI(t, dir, nil, "track", "--skill", "demo", "--event", "custom", "--props", props); err != nil {
		t.Fatal(err)
	}
	contact := receiveEvent(t, events).Props["contact"].(string)
	if strings.Contains(contact, "alice") || len([]rune(contact)) > 200 {
		t.Fatalf("email was exposed at truncation boundary: %q", contact)
	}
}

func TestConcurrentFirstRunsPrintNoticeOnce(t *testing.T) {
	dir := t.TempDir()
	type result struct {
		stderr string
		err    error
	}
	results := make(chan result, 8)
	for range 8 {
		go func() {
			_, stderr, err := runCLI(t, dir, nil, "--version")
			results <- result{stderr: stderr, err: err}
		}()
	}
	notices := 0
	for range 8 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		notices += strings.Count(result.stderr, "yop-cli config telemetry off")
	}
	if notices != 1 {
		t.Fatalf("notice count = %d, want 1", notices)
	}
}

func TestCommandHelpIsAvailableAtDocumentedPaths(t *testing.T) {
	dir := t.TempDir()
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--help"}, want: "yop-cli track"},
		{args: []string{"config", "--help"}, want: "yop-cli config set"},
		{args: []string{"config", "set", "--help"}, want: "yop-cli config set <key> <value>"},
		{args: []string{"config", "get", "--help"}, want: "yop-cli config get <key>"},
		{args: []string{"config", "telemetry", "--help"}, want: "yop-cli config telemetry <on|off>"},
		{args: []string{"track", "--help"}, want: "yop-cli track --skill <name>"},
		{args: []string{"update", "--help"}, want: "yop-cli update [--check]"},
	} {
		stdout, _, err := runCLI(t, dir, nil, test.args...)
		if err != nil || !strings.Contains(stdout, test.want) {
			t.Errorf("%v: stdout=%q err=%v, want %q", test.args, stdout, err, test.want)
		}
	}
}

func TestSkillCLIHelpCommandsAreExecutable(t *testing.T) {
	manifests, err := skillmeta.LoadTree("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range manifests {
		fields := strings.Fields(manifest.Metadata.CLIHelp)
		if len(fields) < 2 || fields[0] != "yop-cli" {
			t.Fatalf("%s cliHelp = %q", manifest.Name, manifest.Metadata.CLIHelp)
		}
		stdout, stderr, err := runCLI(t, t.TempDir(), nil, fields[1:]...)
		if err != nil || strings.TrimSpace(stdout) == "" {
			t.Fatalf("%s cliHelp: stdout=%q stderr=%q err=%v", manifest.Name, stdout, stderr, err)
		}
	}
}

func TestConcurrentConfigUpdatesKeepAllKeys(t *testing.T) {
	dir := t.TempDir()
	commands := make([]*exec.Cmd, 8)
	for index := range commands {
		key := fmt.Sprintf("key_%d", index)
		commands[index] = exec.Command(binary(t), "config", "set", key, fmt.Sprintf("value_%d", index))
		commands[index].Env = append(os.Environ(), "YOP_CONFIG_DIR="+dir)
		if err := commands[index].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	config := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	for index := range 8 {
		key := fmt.Sprintf("key_%d", index)
		if config[key] != fmt.Sprintf("value_%d", index) {
			t.Fatalf("%s = %q", key, config[key])
		}
	}
}
