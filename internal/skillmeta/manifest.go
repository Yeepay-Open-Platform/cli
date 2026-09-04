package skillmeta

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const requiredBinary = "yop-cli"

var (
	validName    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	validVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
)

type Manifest struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Metadata    Metadata `yaml:"metadata"`
}

type Metadata struct {
	Requires  Requirements `yaml:"requires"`
	CLIHelp   string       `yaml:"cliHelp"`
	Telemetry bool         `yaml:"telemetry"`
}

type Requirements struct {
	Bins   []string `yaml:"bins"`
	Skills []string `yaml:"skills"`
}

func LoadTree(root string) ([]Manifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}
	manifests := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := load(filepath.Join(root, entry.Name(), "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", entry.Name(), err)
		}
		if err := validate(manifest, entry.Name()); err != nil {
			return nil, fmt.Errorf("skill %q: %w", entry.Name(), err)
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	if err := validateDependencies(manifests); err != nil {
		return nil, err
	}
	return manifests, nil
}

func load(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	content := strings.TrimPrefix(string(raw), "\uFEFF")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return Manifest{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	closing := -1
	for index, line := range lines[1:] {
		if strings.TrimRight(line, "\r") == "---" {
			closing = index + 1
			break
		}
	}
	if closing < 0 {
		return Manifest{}, fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	var manifest Manifest
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:closing], "\n")), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	return manifest, nil
}

func validate(manifest Manifest, directory string) error {
	if manifest.Name != directory {
		return fmt.Errorf("name %q must match directory %q", manifest.Name, directory)
	}
	if !validName.MatchString(manifest.Name) || len(manifest.Name) > 64 {
		return fmt.Errorf("name %q must use lowercase letters, digits, and single hyphens", manifest.Name)
	}
	if manifest.Description == "" || utf8.RuneCountInString(manifest.Description) > 1024 {
		return fmt.Errorf("description must contain 1-1024 characters")
	}
	if !validVersion.MatchString(manifest.Version) {
		return fmt.Errorf("version %q must be semantic versioning", manifest.Version)
	}
	if !contains(manifest.Metadata.Requires.Bins, requiredBinary) {
		return fmt.Errorf("metadata.requires.bins must include %q", requiredBinary)
	}
	if !manifest.Metadata.Telemetry {
		return fmt.Errorf("skills requiring %s must enable telemetry", requiredBinary)
	}
	if !strings.HasPrefix(manifest.Metadata.CLIHelp, requiredBinary+" ") || !strings.HasSuffix(manifest.Metadata.CLIHelp, " --help") {
		return fmt.Errorf("cliHelp must be a %s command ending in --help", requiredBinary)
	}
	for _, dependency := range manifest.Metadata.Requires.Skills {
		if !validName.MatchString(dependency) {
			return fmt.Errorf("requires skill %q has an invalid name", dependency)
		}
	}
	return nil
}

func validateDependencies(manifests []Manifest) error {
	byName := make(map[string]Manifest, len(manifests))
	for _, manifest := range manifests {
		byName[manifest.Name] = manifest
	}
	for _, manifest := range manifests {
		for _, dependency := range manifest.Metadata.Requires.Skills {
			if _, ok := byName[dependency]; !ok {
				return fmt.Errorf("skill %q requires missing skill %q", manifest.Name, dependency)
			}
		}
	}

	const (
		visiting = 1
		visited  = 2
	)
	state := make(map[string]int, len(manifests))
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == visiting {
			return fmt.Errorf("dependency cycle includes skill %q", name)
		}
		if state[name] == visited {
			return nil
		}
		state[name] = visiting
		for _, dependency := range byName[name].Metadata.Requires.Skills {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = visited
		return nil
	}
	for _, manifest := range manifests {
		if err := visit(manifest.Name); err != nil {
			return err
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
