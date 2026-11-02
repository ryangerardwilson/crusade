package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
	"github.com/ryangerardwilson/crusade/internal/version"
)

func TestNoArgsMatchesHelp(t *testing.T) {
	var noArgs bytes.Buffer
	var help bytes.Buffer
	if code := Run(nil, &noArgs, &bytes.Buffer{}); code != 0 {
		t.Fatalf("no args code = %d", code)
	}
	if code := Run([]string{"help"}, &help, &bytes.Buffer{}); code != 0 {
		t.Fatalf("help code = %d", code)
	}
	if noArgs.String() != help.String() {
		t.Fatalf("no args output differed from help")
	}
	if !strings.Contains(help.String(), "open a mode:") || !strings.Contains(help.String(), "crusade -m box") || !strings.Contains(help.String(), "crusade -m nlp") || !strings.Contains(help.String(), "crusade -m align -r limit 10") || !strings.Contains(help.String(), "crusade -m do -c") || !strings.Contains(help.String(), "resume the last mode") {
		t.Fatalf("help output missing expected contract text:\n%s", help.String())
	}
	if strings.Contains(help.String(), "\x1b") {
		t.Fatalf("help output should be unstyled")
	}
}

func TestVersionIsSingleLine(t *testing.T) {
	var out bytes.Buffer
	if code := Run([]string{"version"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("version code = %d", code)
	}
	want := version.Version + "\n"
	if out.String() != want {
		t.Fatalf("version output = %q, want %q", out.String(), want)
	}
}

func TestConfigCreatesBootstrap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("EDITOR", "/usr/bin/true")
	var out bytes.Buffer
	if code := Run([]string{"config"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("config code = %d output=%q", code, out.String())
	}
	data, err := os.ReadFile(filepath.Join(home, "config", "xyz", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != config.ConfigBootstrapText {
		t.Fatalf("bootstrap config = %q", string(data))
	}
}

func TestAddListEditDeleteFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	defer func() { nowFunc = time.Now }()

	addArgs := []string{
		"-m", "align", "-c",
		"x", "2026-01-26 00:00",
		"y", "ship the patch",
		"z", "reduce drift",
		"p", "7",
		"q", "8",
		"r", "6",
	}
	if code := Run(addArgs, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add code = %d", code)
	}
	var list bytes.Buffer
	if code := Run([]string{"-m", "align", "-r", "limit", "5"}, &list, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list code = %d output=%q", code, list.String())
	}
	if !strings.Contains(list.String(), "id: 1") || !strings.Contains(list.String(), "ship the patch") {
		t.Fatalf("list output missing item:\n%s", list.String())
	}
	if code := Run([]string{"-m", "align", "-u", "1", "y", "updated outcome", "p", "8.5"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("edit code = %d", code)
	}
	var edited bytes.Buffer
	if code := Run([]string{"-m", "align", "-r"}, &edited, &bytes.Buffer{}); code != 0 {
		t.Fatalf("edited list code = %d", code)
	}
	if !strings.Contains(edited.String(), "updated outcome") || !strings.Contains(edited.String(), "p: 8.5") {
		t.Fatalf("edited output missing update:\n%s", edited.String())
	}
	if code := Run([]string{"-m", "align", "-d", "1"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("delete code = %d", code)
	}
	var empty bytes.Buffer
	if code := Run([]string{"-m", "align", "-r"}, &empty, &bytes.Buffer{}); code != 0 {
		t.Fatalf("empty list code = %d", code)
	}
	if !strings.Contains(empty.String(), "No upcoming items.") {
		t.Fatalf("empty list output = %s", empty.String())
	}
}

func TestUpgradeUsesOverrideScript(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	script := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" > \"$XYZ_MARKER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XYZ_INSTALL_SCRIPT", script)
	t.Setenv("XYZ_MARKER", marker)
	if code := Run([]string{"upgrade"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("upgrade code = %d", code)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "upgrade" {
		t.Fatalf("marker = %q", string(data))
	}
}

func TestListAndEditStayOnThinkRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	defer func() { nowFunc = time.Now }()

	think, err := model.NewEvent("2026-02-01 00:00:00", "ship the patch", "reduce drift", "7", "8", "6")
	if err != nil {
		t.Fatal(err)
	}
	do, err := model.NewDoEvent("2026-02-02 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	path := store.DatabasePath()
	if err := store.Save(path, []store.Record{{ID: 1, Event: think}, {ID: 2, Event: do}}); err != nil {
		t.Fatal(err)
	}

	var list bytes.Buffer
	if code := Run([]string{"-m", "align", "-r"}, &list, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list code = %d output=%q", code, list.String())
	}
	if !strings.Contains(list.String(), "ship the patch") || strings.Contains(list.String(), "mail the letter") || !strings.Contains(list.String(), "crusade align items (1)") {
		t.Fatalf("list mixed modes:\n%s", list.String())
	}

	var edited bytes.Buffer
	if code := Run([]string{"-m", "align", "-u", "2", "y", "changed"}, &edited, &bytes.Buffer{}); code == 0 {
		t.Fatalf("edit do task succeeded: %s", edited.String())
	}
	if !strings.Contains(edited.String(), "do record") {
		t.Fatalf("edit do task output = %q", edited.String())
	}

	var cleared bytes.Buffer
	if code := Run([]string{"-m", "align", "-u", "1", "z", ""}, &cleared, &bytes.Buffer{}); code == 0 || !strings.Contains(cleared.String(), "impact") {
		t.Fatalf("empty z edit code=%d output=%q", code, cleared.String())
	}
	var again bytes.Buffer
	if code := Run([]string{"-m", "align", "-r"}, &again, &bytes.Buffer{}); code != 0 || !strings.Contains(again.String(), "reduce drift") {
		t.Fatalf("think impact was cleared:\n%s", again.String())
	}

	var doList bytes.Buffer
	if code := Run([]string{"-m", "do", "-r"}, &doList, &bytes.Buffer{}); code != 0 {
		t.Fatalf("do list code = %d output=%q", code, doList.String())
	}
	if !strings.Contains(doList.String(), "mail the letter") || strings.Contains(doList.String(), "ship the patch") || !strings.Contains(doList.String(), "crusade do items (1)") {
		t.Fatalf("do list mixed modes:\n%s", doList.String())
	}
	var updateWrong bytes.Buffer
	if code := Run([]string{"-m", "do", "-u", "1", "nope"}, &updateWrong, &bytes.Buffer{}); code == 0 || !strings.Contains(updateWrong.String(), "align record") {
		t.Fatalf("do update align code=%d output=%q", code, updateWrong.String())
	}
	var deleteWrong bytes.Buffer
	if code := Run([]string{"-m", "do", "-d", "1"}, &deleteWrong, &bytes.Buffer{}); code == 0 || !strings.Contains(deleteWrong.String(), "align record") {
		t.Fatalf("do delete align code=%d output=%q", code, deleteWrong.String())
	}
	var deleteDo bytes.Buffer
	if code := Run([]string{"-m", "align", "-d", "2"}, &deleteDo, &bytes.Buffer{}); code == 0 || !strings.Contains(deleteDo.String(), "do record") {
		t.Fatalf("align delete do code=%d output=%q", code, deleteDo.String())
	}
	var updated bytes.Buffer
	if code := Run([]string{"-m", "do", "-u", "2", "sent the letter", "2026-06-13"}, &updated, &bytes.Buffer{}); code != 0 {
		t.Fatalf("do update code=%d output=%q", code, updated.String())
	}
	var created bytes.Buffer
	if code := Run([]string{"-m", "do", "-c", "write the note"}, &created, &bytes.Buffer{}); code != 0 {
		t.Fatalf("do create code=%d output=%q", code, created.String())
	}
	var after bytes.Buffer
	if code := Run([]string{"-m", "do", "-r"}, &after, &bytes.Buffer{}); code != 0 {
		t.Fatalf("do list after = %d", code)
	}
	if !strings.Contains(after.String(), "sent the letter") || !strings.Contains(after.String(), "2026-06-13 23:59:59") || !strings.Contains(after.String(), "write the note") || !strings.Contains(after.String(), "2026-01-01 23:59:59") {
		t.Fatalf("do records:\n%s", after.String())
	}
	var old bytes.Buffer
	if code := Run([]string{"list", "outcomes"}, &old, &bytes.Buffer{}); code == 0 || !strings.Contains(old.String(), "Unknown command") {
		t.Fatalf("old list command code=%d output=%q", code, old.String())
	}
}

func TestBoxCreateAndUpdateEditAllBoxesInYAML(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(root, "missing-outline.txt"))
	t.Setenv("VISUAL", "")
	script := filepath.Join(root, "edit-boxes")
	writeBoxEditor(t, script, "version: 1\nboxes:\n  - label: Move\n  - label: Work\n    children:\n      - label: Email\n")
	t.Setenv("EDITOR", script)

	var created bytes.Buffer
	if code := Run([]string{"-m", "box", "-c"}, &created, &bytes.Buffer{}); code != 0 {
		t.Fatalf("create code = %d output=%q", code, created.String())
	}
	if !strings.Contains(created.String(), "saved 3") {
		t.Fatalf("create output = %q", created.String())
	}
	outline, err := store.LoadOutline(store.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	email, ok := outline.ItemByID("work/email")
	if !ok || email.Label != "Email" {
		t.Fatalf("email = %#v ok=%v", email, ok)
	}

	writeBoxEditor(t, script, "version: 1\nboxes:\n  - label: Plan\n")
	var updated bytes.Buffer
	if code := Run([]string{"-m", "box", "-u"}, &updated, &bytes.Buffer{}); code != 0 {
		t.Fatalf("update code = %d output=%q", code, updated.String())
	}
	outline, err = store.LoadOutline(store.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outline.ItemByID("work/email"); ok {
		t.Fatal("replaced outline kept the old box")
	}
	if _, ok := outline.ItemByID("plan"); !ok {
		t.Fatal("missing plan")
	}

	var extra bytes.Buffer
	if code := Run([]string{"-m", "box", "-u", "plan"}, &extra, &bytes.Buffer{}); code == 0 || !strings.Contains(extra.String(), "Usage: crusade -m box -u") {
		t.Fatalf("extra args code=%d output=%q", code, extra.String())
	}

	writeBoxEditor(t, script, "surprise: true\n")
	var bad bytes.Buffer
	if code := Run([]string{"-m", "box", "-c"}, &bad, &bytes.Buffer{}); code == 0 || !strings.Contains(bad.String(), "surprise") {
		t.Fatalf("invalid yaml code=%d output=%q", code, bad.String())
	}
	outline, err = store.LoadOutline(store.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outline.ItemByID("plan"); !ok || len(outline.Flatten()) != 1 {
		t.Fatal("invalid yaml changed the outline")
	}
}

func TestOpenResumesLastMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := store.DatabasePath()
	if _, err := store.Open(db); err != nil {
		t.Fatal(err)
	}
	var opened string
	previous := runTUI
	runTUI = func(dataPath, mode string, stdout, stderr io.Writer) int {
		opened = mode
		if dataPath != db {
			t.Fatalf("data path = %s", dataPath)
		}
		return 0
	}
	t.Cleanup(func() { runTUI = previous })

	var out bytes.Buffer
	if code := Run([]string{"open"}, &out, &bytes.Buffer{}); code != 0 || opened != model.ModeDo {
		t.Fatalf("default open code=%d mode=%q out=%q", code, opened, out.String())
	}
	if err := store.SaveLastMode(db, model.ModeBox); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"open"}, &out, &bytes.Buffer{}); code != 0 || opened != model.ModeBox {
		t.Fatalf("resume open code=%d mode=%q out=%q", code, opened, out.String())
	}
}

func writeBoxEditor(t *testing.T, path, document string) {
	t.Helper()
	body := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n" + document + "EOF\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
