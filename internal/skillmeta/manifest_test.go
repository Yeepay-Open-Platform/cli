package skillmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTreeValidatesRepositorySkills(t *testing.T) {
	manifests, err := LoadTree(filepath.Join("..", "..", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 {
		t.Fatalf("manifest count = %d, want 1", len(manifests))
	}
	manifest := manifests[0]
	if manifest.Name != "telemetry-pilot" || manifest.Metadata.Version != "0.1.0" {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if len(manifest.Metadata.Requires.Bins) != 1 || manifest.Metadata.Requires.Bins[0] != "yop-cli" {
		t.Fatalf("required bins = %v", manifest.Metadata.Requires.Bins)
	}
	if manifest.Metadata.CLIHelp != "yop-cli track --help" || !manifest.Metadata.Telemetry {
		t.Fatalf("CLI contract = %+v", manifest)
	}
}

func TestLoadTreeRejectsBrokenContracts(t *testing.T) {
	for _, test := range []struct {
		name      string
		skills    map[string]string
		wantError string
	}{
		{
			name:      "missing required binary",
			skills:    map[string]string{"demo": "---\nname: demo\ndescription: Test skill\nmetadata:\n  version: 1.0.0\n  requires:\n    bins: []\n  cliHelp: \"yop-cli track --help\"\n  telemetry: true\n---\n"},
			wantError: "requires.bins",
		},
		{
			name:      "missing skill dependency",
			skills:    map[string]string{"demo": skillFile("demo", "missing", "")},
			wantError: "requires missing skill",
		},
		{
			name: "cyclic skill dependency",
			skills: map[string]string{
				"alpha": skillFile("alpha", "beta", ""),
				"beta":  skillFile("beta", "alpha", ""),
			},
			wantError: "dependency cycle",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range test.skills {
				dir := filepath.Join(root, name)
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := LoadTree(root)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("LoadTree error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestLoadTreeUsesSemanticVersioning(t *testing.T) {
	for _, version := range []string{"0.0.0", "1.2.3-alpha.1", "1.2.3+sha.abc", "1.2.3-rc.1+build.9"} {
		t.Run("valid "+version, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "demo", skillFileWithVersion("demo", version))
			if _, err := LoadTree(root); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, version := range []string{"01.0.0", "1.0.0-alpha..1", "1.0.0-01", "1.0"} {
		t.Run("invalid "+version, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "demo", skillFileWithVersion("demo", version))
			if _, err := LoadTree(root); err == nil || !strings.Contains(err.Error(), "metadata.version") {
				t.Fatalf("LoadTree error = %v, want metadata.version error", err)
			}
		})
	}
}

func skillFile(name, requiredSkill, cliHelp string) string {
	if cliHelp == "" {
		cliHelp = "yop-cli track --help"
	}
	content := strings.Replace(skillFileWithVersion(name, "1.0.0"), "yop-cli track --help", cliHelp, 1)
	if requiredSkill != "" {
		content = strings.Replace(content, "    skills: []", "    skills: [\""+requiredSkill+"\"]", 1)
	}
	return content
}

func skillFileWithVersion(name, version string) string {
	return "---\nname: " + name + "\ndescription: Test skill\nmetadata:\n  version: " + version + "\n  requires:\n    bins: [\"yop-cli\"]\n    skills: []\n  cliHelp: \"yop-cli track --help\"\n  telemetry: true\n---\n"
}

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
