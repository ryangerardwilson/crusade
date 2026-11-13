package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const ConfigBootstrapText = "{}\n"

type Config struct{}

func configBase() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".config")
	}
	return base
}

func Path() string {
	return filepath.Join(configBase(), "crusade", "config.json")
}

func legacyPath() string {
	return filepath.Join(configBase(), "xyz", "config.json")
}

func Ensure() error {
	if err := migrateLegacyConfig(); err != nil {
		return err
	}
	configPath := Path()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return os.WriteFile(configPath, []byte(ConfigBootstrapText), 0o644)
	}
	return nil
}

// migrateLegacyConfig moves a config and do list saved under the old xyz
// directory into the crusade directory. An existing crusade file is kept.
func migrateLegacyConfig() error {
	if err := moveIfMissing(legacyPath(), Path()); err != nil {
		return err
	}
	legacyDo := filepath.Join(filepath.Dir(legacyPath()), "do")
	nextDo := filepath.Join(filepath.Dir(Path()), "do")
	if err := moveIfMissing(legacyDo, nextDo); err != nil {
		return err
	}
	return removeEmptyLegacyDir()
}

func moveIfMissing(from, to string) error {
	if _, err := os.Stat(to); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	info, err := os.Stat(from)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

func removeEmptyLegacyDir() error {
	err := os.Remove(filepath.Dir(legacyPath()))
	if err == nil || os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	return err
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
