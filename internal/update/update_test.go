package update

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Yeepay-Open-Platform/cli/internal/config"
)

func TestVersionComparisonAndChannel(t *testing.T) {
	for _, test := range []struct {
		candidate string
		current   string
		newer     bool
	}{
		{"0.0.0-beta.1", "0.0.0-beta.0", true},
		{"0.0.0", "0.0.0-beta.9", true},
		{"1.0.0-beta.0", "0.9.9", true},
		{"1.0.0-beta.1", "1.0.0-beta.2", false},
		{"1.0.0", "1.0.0", false},
	} {
		if got := IsNewer(test.candidate, test.current); got != test.newer {
			t.Errorf("IsNewer(%q, %q) = %t, want %t", test.candidate, test.current, got, test.newer)
		}
	}
	if ChannelForVersion("0.0.0-beta.0") != "beta" || ChannelForVersion("1.0.0") != "latest" {
		t.Fatal("release channel selection does not follow the current version")
	}
}

func TestCheckCachedUsesMatchingChannel(t *testing.T) {
	t.Setenv("YOP_CONFIG_DIR", t.TempDir())
	if err := config.WriteJSON("update-state.json", state{LatestVersion: "0.0.0-beta.1", CheckedAt: 1, Channel: "beta"}); err != nil {
		t.Fatal(err)
	}
	info := CheckCached("0.0.0-beta.0")
	if info == nil || info.Latest != "0.0.0-beta.1" {
		t.Fatalf("CheckCached() = %#v", info)
	}
	if CheckCached("0.0.0") != nil {
		t.Fatal("stable version consumed beta cache")
	}
}

func TestRunCheckQueriesCurrentChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/beta" {
			t.Errorf("registry path = %q, want /beta", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"0.0.0-beta.1"}`))
	}))
	defer server.Close()
	oldBase, oldClient := registryBase, defaultClient
	registryBase, defaultClient = server.URL, server.Client()
	defer func() { registryBase, defaultClient = oldBase, oldClient }()

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", "--json"}, "0.0.0-beta.0", &stdout, &stderr); code != 0 {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["action"] != "update_available" || result["channel"] != "beta" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDetectFromResolved(t *testing.T) {
	if got := detectFromResolved("/usr/local/lib/node_modules/@yeepay/yop-cli/bin/yop-cli", true, false); got.Method != InstallNPM || !got.Available {
		t.Fatalf("npm detection = %#v", got)
	}
	if got := detectFromResolved("/home/me/.pnpm/node_modules/@yeepay/yop-cli/bin/yop-cli", false, true); got.Method != InstallPNPM || !got.Available {
		t.Fatalf("pnpm detection = %#v", got)
	}
	if got := detectFromResolved("/usr/local/bin/yop-cli", true, true); got.Method != InstallManual {
		t.Fatalf("manual detection = %#v", got)
	}
}
