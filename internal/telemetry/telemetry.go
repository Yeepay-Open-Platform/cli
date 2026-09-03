package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Yeepay-Open-Platform/cli/internal/config"
)

const (
	payloadEnv = "YOP_TELEMETRY_PAYLOAD"
	webhookEnv = "YOP_TELEMETRY_WEBHOOK"
)

var (
	validKey = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	phone    = regexp.MustCompile(`(^|[^0-9])(1[3-9][0-9])[0-9]{4}([0-9]{4})([^0-9]|$)`)
	email    = regexp.MustCompile(`([A-Za-z0-9])[A-Za-z0-9._%+\-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})`)
)

type Event struct {
	EventID      string         `json:"event_id"`
	Timestamp    int64          `json:"ts"`
	EventType    string         `json:"event_type"`
	Skill        string         `json:"skill"`
	SkillVersion string         `json:"skill_version,omitempty"`
	CLIVersion   string         `json:"cli_version"`
	OS           string         `json:"os"`
	Arch         string         `json:"arch"`
	InstallID    string         `json:"install_id"`
	Props        map[string]any `json:"props"`
}

type Input struct {
	Skill        string
	EventType    string
	SkillVersion string
	Props        string
}

func Track(input Input, version, defaultWebhook, executable string, debug io.Writer) {
	if disabled(config.Load()) {
		return
	}
	if strings.TrimSpace(input.Skill) == "" || !validEventType(input.EventType) {
		debugf(debug, "telemetry dropped: skill and event must be valid")
		return
	}
	props, ok := sanitizeProps(input.Props)
	if !ok {
		debugf(debug, "telemetry dropped: props must be a JSON object")
		return
	}
	values := config.Load()
	webhook := values["telemetry.webhook"]
	if webhook == "" {
		webhook = defaultWebhook
	}
	if webhook == "" {
		debugf(debug, "telemetry dropped: webhook is not configured")
		return
	}
	installID, reserved, err := prepare(input.Skill, input.EventType, time.Now())
	if err != nil {
		debugf(debug, "telemetry dropped: %v", err)
		return
	}
	if !reserved {
		return
	}
	eventID, err := uuid()
	if err != nil {
		debugf(debug, "telemetry dropped: %v", err)
		return
	}
	event := Event{
		EventID: eventID, Timestamp: time.Now().UnixMilli(), EventType: input.EventType,
		Skill: input.Skill, SkillVersion: input.SkillVersion, CLIVersion: version,
		OS: runtime.GOOS, Arch: runtime.GOARCH, InstallID: installID, Props: props,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		debugf(debug, "telemetry dropped: %v", err)
		return
	}
	cmd := exec.Command(executable, "__track-send")
	cmd.Env = append(os.Environ(), payloadEnv+"="+base64.StdEncoding.EncodeToString(raw), webhookEnv+"="+webhook)
	if err := cmd.Start(); err != nil {
		debugf(debug, "telemetry dropped: %v", err)
		return
	}
	_ = cmd.Process.Release()
}

func Send() {
	raw, err := base64.StdEncoding.DecodeString(os.Getenv(payloadEnv))
	if err != nil || len(raw) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv(webhookEnv), bytes.NewReader(raw))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		resp, requestErr := http.DefaultClient.Do(req)
		err = requestErr
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				err = fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
			}
		}
	}
	if err != nil && os.Getenv("YOP_TELEMETRY_DEBUG") == "1" {
		writeDeadletter(raw, err)
	}
}

func disabled(values map[string]string) bool {
	return os.Getenv("YOP_TELEMETRY") == "0" || config.Exists("telemetry-disabled") || strings.EqualFold(values["telemetry.enabled"], "false")
}

func validEventType(value string) bool {
	switch value {
	case "skill_start", "skill_end", "skill_error", "custom":
		return true
	default:
		return false
	}
}

func sanitizeProps(raw string) (map[string]any, bool) {
	if raw == "" {
		return map[string]any{}, true
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return nil, false
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]any, 10)
	for _, key := range keys {
		if len(result) == 10 || !validKey.MatchString(key) {
			continue
		}
		switch value := values[key].(type) {
		case string:
			result[key] = truncate(mask(value), 200)
		case float64, bool, nil:
			result[key] = value
		}
	}
	return result, true
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func mask(value string) string {
	value = phone.ReplaceAllString(value, `${1}${2}****${3}${4}`)
	return email.ReplaceAllString(value, `${1}***@${2}`)
}

func prepare(skill, eventType string, now time.Time) (installID string, reserved bool, err error) {
	err = config.WithLock("telemetry", func() error {
		var loadErr error
		installID, loadErr = loadInstallID()
		if loadErr != nil {
			return loadErr
		}
		reserved, loadErr = reserve(skill, eventType, now)
		return loadErr
	})
	return installID, reserved, err
}

func loadInstallID() (string, error) {
	var state struct {
		InstallID string `json:"install_id"`
	}
	if config.ReadJSON("install.json", &state) == nil && state.InstallID != "" {
		return state.InstallID, nil
	}
	generated, err := uuid()
	if err != nil {
		return "", err
	}
	state.InstallID = generated
	if err := config.WriteJSON("install.json", state); err != nil {
		return "", err
	}
	return state.InstallID, nil
}

func reserve(skill, eventType string, now time.Time) (bool, error) {
	state := map[string]int64{}
	if err := config.ReadJSON("telemetry-state.json", &state); err != nil && !errors.Is(err, os.ErrNotExist) {
		state = map[string]int64{}
	}
	key := skill + "\x00" + eventType
	if sentAt := state[key]; sentAt > 0 && now.UnixMilli()-sentAt < int64(time.Minute/time.Millisecond) {
		return false, nil
	}
	state[key] = now.UnixMilli()
	if err := config.WriteJSON("telemetry-state.json", state); err != nil {
		return false, err
	}
	return true, nil
}

func uuid() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate random UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func debugf(out io.Writer, format string, args ...any) {
	if os.Getenv("YOP_TELEMETRY_DEBUG") == "1" {
		fmt.Fprintf(out, format+"\n", args...)
	}
}

func writeDeadletter(payload []byte, sendErr error) {
	entry, _ := json.Marshal(map[string]any{
		"failed_at": time.Now().UnixMilli(), "error": sendErr.Error(), "event": json.RawMessage(payload),
	})
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(config.Path("telemetry-deadletter.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(entry, '\n'))
}
