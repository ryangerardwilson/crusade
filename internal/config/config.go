package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const ConfigBootstrapText = "{}\n"

type Config struct{}

func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(base, "xyz", "config.json")
}

func Ensure() error {
	configPath := Path()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return os.WriteFile(configPath, []byte(ConfigBootstrapText), 0o644)
	}
	return nil
}

func Load() (Config, error) {
	if err := Ensure(); err != nil {
		return Config{}, err
	}
	if err := dropLegacyPaths(); err != nil {
		return Config{}, err
	}
	return Config{}, nil
}

func dropLegacyPaths() error {
	configPath := Path()
	data, err := os.ReadFile(configPath)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return err
	}
	var raw map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
		stripped := stripTrailingCommas(string(data))
		if retryErr := json.Unmarshal([]byte(stripped), &raw); retryErr != nil {
			return nil
		}
	}
	if _, hasYAML := raw["data_yaml_path"]; !hasYAML {
		if _, hasCSV := raw["data_csv_path"]; !hasCSV {
			return nil
		}
	}
	delete(raw, "data_yaml_path")
	delete(raw, "data_csv_path")
	var encoded []byte
	if len(raw) == 0 {
		encoded = []byte(ConfigBootstrapText)
	} else {
		encoded, err = json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		encoded = append(encoded, '\n')
	}
	return os.WriteFile(configPath, encoded, 0o644)
}

func stripTrailingCommas(text string) string {
	re := regexp.MustCompile(`,\s*([}\]])`)
	return re.ReplaceAllString(text, "$1")
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
