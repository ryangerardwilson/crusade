package radial

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Pick opens the radial menu and returns the chosen mode.
// An empty mode means the menu closed without a choice.
func Pick(preferred string) (string, error) {
	dir, err := os.MkdirTemp("", "crusade-radial-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := extractQML(dir); err != nil {
		return "", err
	}
	dataPath := filepath.Join(dir, "modes.json")
	if err := writeModes(dataPath, preferred); err != nil {
		return "", err
	}
	outPath := filepath.Join(dir, "result.tsv")
	err = runPicker(dir, []string{
		"CRUSADE_RADIAL_DATA=" + dataPath,
		"CRUSADE_RADIAL_OUT=" + outPath,
	})
	if err != nil {
		if _, statErr := os.Stat(outPath); os.IsNotExist(statErr) {
			return "", err
		}
	}
	return readChoice(outPath)
}

var runPicker = launchQuickshell

func launchQuickshell(dir string, extra []string) error {
	command, err := pickerCommand(dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(os.Environ(), extra...)
	cmd.Dir = dir
	return cmd.Run()
}

func pickerCommand(dir string) ([]string, error) {
	if _, err := exec.LookPath("quickshell"); err != nil {
		return nil, fmt.Errorf("quickshell is required for the radial menu")
	}
	if _, err := exec.LookPath("uwsm-app"); err == nil {
		return []string{"uwsm-app", "--", "quickshell", "--path", dir}, nil
	}
	return []string{"quickshell", "--path", dir}, nil
}

type modeItem struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
}

type modePayload struct {
	Preferred string     `json:"preferred"`
	Modes     []modeItem `json:"modes"`
}

func writeModes(path, preferred string) error {
	body := modePayload{
		Preferred: preferred,
		Modes: []modeItem{
			{Name: "do", Label: "DO", Blurb: "the list"},
			{Name: "cal", Label: "CAL", Blurb: "the week"},
			{Name: "box", Label: "BOX", Blurb: "the day"},
			{Name: "nlp", Label: "NLP", Blurb: "practice"},
			{Name: "align", Label: "ALIGN", Blurb: "outcomes"},
		},
	}
	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func readChoice(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", nil
	}
	action, name, ok := strings.Cut(text, "\t")
	if !ok || action != "select" || !knownMode(name) {
		return "", fmt.Errorf("radial menu returned an unusable choice")
	}
	return name, nil
}

func knownMode(name string) bool {
	switch name {
	case "do", "align", "box", "cal", "nlp":
		return true
	default:
		return false
	}
}
