package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCreatesEmptyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ConfigBootstrapText {
		t.Fatalf("config = %q", string(data))
	}
}

func TestLoadDropsLegacyPathKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	configPath := filepath.Join(home, "config", "xyz", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\n  \"data_yaml_path\": \"~/Documents/notes/xyz/tasks.yaml\",\n  \"data_csv_path\": \"~/legacy.csv\",\n  \"kept\": true\n}\n")
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "data_yaml_path") || strings.Contains(text, "data_csv_path") || strings.Contains(text, "tasks.yaml") {
		t.Fatalf("legacy paths remained: %s", text)
	}
	if !strings.Contains(text, "\"kept\"") {
		t.Fatalf("other keys were removed: %s", text)
	}
}
