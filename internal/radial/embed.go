package radial

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed qml/*
var qmlFS embed.FS

func extractQML(dir string) error {
	entries, err := qmlFS.ReadDir("qml")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := qmlFS.ReadFile("qml/" + entry.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
