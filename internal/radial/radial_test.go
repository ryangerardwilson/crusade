package radial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickReturnsTheSelectedMode(t *testing.T) {
	previous := runPicker
	t.Cleanup(func() { runPicker = previous })
	var data string
	runPicker = func(_ string, extra []string) error {
		values := envMap(extra)
		raw, err := os.ReadFile(values["CRUSADE_RADIAL_DATA"])
		if err != nil {
			return err
		}
		data = string(raw)
		return os.WriteFile(values["CRUSADE_RADIAL_OUT"], []byte("select\talign\n"), 0o644)
	}
	got, err := Pick("box")
	if err != nil || got != "align" {
		t.Fatalf("got %q err %v", got, err)
	}
	if !strings.Contains(data, `"preferred": "box"`) || !strings.Contains(data, `"label": "NLP"`) {
		t.Fatalf("payload:\n%s", data)
	}
	order := []string{`"name": "do"`, `"name": "cal"`, `"name": "box"`, `"name": "nlp"`, `"name": "align"`}
	last := -1
	for _, name := range order {
		idx := strings.Index(data, name)
		if idx <= last {
			t.Fatalf("order %s in\n%s", name, data)
		}
		last = idx
	}
}

func TestPickCancelAndBadChoice(t *testing.T) {
	previous := runPicker
	t.Cleanup(func() { runPicker = previous })
	runPicker = func(string, []string) error { return nil }
	got, err := Pick("do")
	if err != nil || got != "" {
		t.Fatalf("cancel got %q err %v", got, err)
	}
	runPicker = func(_ string, extra []string) error {
		return os.WriteFile(envMap(extra)["CRUSADE_RADIAL_OUT"], []byte("nope\tdo\n"), 0o644)
	}
	got, err = Pick("do")
	if err == nil || got != "" || !strings.Contains(err.Error(), "unusable") {
		t.Fatalf("bad choice got %q err %v", got, err)
	}
}

func TestPickReportsAClosedMenu(t *testing.T) {
	previous := runPicker
	t.Cleanup(func() { runPicker = previous })
	runPicker = func(string, []string) error { return os.ErrClosed }
	got, err := Pick("do")
	if got != "" || err == nil {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestPickerCommandNeedsQuickshell(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := pickerCommand(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "quickshell") {
		t.Fatal(err)
	}
}

func TestQMLFilesAreEmbedded(t *testing.T) {
	dir := t.TempDir()
	if err := extractQML(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shell.qml", "RingField.qml", "OrbitTile.qml"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	shell, err := os.ReadFile(filepath.Join(dir, "shell.qml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shell), "Qt.Key_M") || !strings.Contains(string(shell), "ControlModifier") {
		t.Fatal("ctrl+m does not open the selected radial mode")
	}
}

func envMap(extra []string) map[string]string {
	values := map[string]string{}
	for _, item := range extra {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}
