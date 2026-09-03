package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const directoryEnv = "YOP_CONFIG_DIR"

func Dir() string {
	if dir := os.Getenv(directoryEnv); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".yop"
	}
	return filepath.Join(dir, "yop")
}

func Path(name string) string {
	return filepath.Join(Dir(), name)
}

func Load() map[string]string {
	raw, err := os.ReadFile(Path("config.json"))
	if err != nil {
		return map[string]string{}
	}
	values := map[string]string{}
	if json.Unmarshal(raw, &values) != nil {
		return map[string]string{}
	}
	return values
}

func Save(values map[string]string) error {
	return WriteJSON("config.json", values)
}

func WriteJSON(name string, value any) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".yop-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, Path(name)); err != nil {
		return err
	}
	return nil
}

func ReadJSON(name string, value any) error {
	raw, err := os.ReadFile(Path(name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return err
	}
	return nil
}

func Exists(name string) bool {
	_, err := os.Stat(Path(name))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func Remove(name string) error {
	err := os.Remove(Path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func WithLock(name string, action func() error) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	lockPath := Path(name + ".lock")
	for attempt := 0; attempt < 100; attempt++ {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			defer os.Remove(lockPath)
			return action()
		} else if !errors.Is(err, os.ErrExist) {
			return err
		}
		if info, err := os.Stat(lockPath); err == nil && time.Since(info.ModTime()) > 10*time.Second {
			_ = os.Remove(lockPath)
			continue
		}
		time.Sleep(2 * time.Millisecond)
	}
	return errors.New("configuration lock is busy")
}
