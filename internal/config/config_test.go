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
	if _, err := os.Stat(filepath.Join(home, "config", "xyz")); !os.IsNotExist(err) {
		t.Fatalf("xyz config was created: %v", err)
	}
}

func TestLoadMovesTheXyzConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	legacyDir := filepath.Join(home, "config", "xyz")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\n  \"data_yaml_path\": \"~/Documents/notes/xyz/tasks.yaml\",\n  \"data_csv_path\": \"~/legacy.csv\",\n  \"kept\": true\n}\n")
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "do"), []byte("mail the letter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
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
	doBody, err := os.ReadFile(filepath.Join(filepath.Dir(Path()), "do"))
	if err != nil {
		t.Fatal(err)
	}
	if string(doBody) != "mail the letter\n" {
		t.Fatalf("do list = %q", doBody)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("xyz config remained: %v", err)
	}
}

func TestLoadKeepsAnExistingCrusadeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	crusadeDir := filepath.Join(home, "config", "crusade")
	legacyDir := filepath.Join(home, "config", "xyz")
	if err := os.MkdirAll(crusadeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crusadeDir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), []byte("{\"kept\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "do"), []byte("old list\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(crusadeDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("config = %q", data)
	}
	moved, err := os.ReadFile(filepath.Join(crusadeDir, "do"))
	if err != nil {
		t.Fatal(err)
	}
	if string(moved) != "old list\n" {
		t.Fatalf("do list = %q", moved)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "config.json")); err != nil {
		t.Fatal(err)
	}
}
