package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/dofile"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
	"github.com/ryangerardwilson/crusade/internal/tui"
	"github.com/ryangerardwilson/crusade/internal/version"
	"golang.org/x/sys/unix"
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
	if !strings.Contains(help.String(), "open a mode:") || !strings.Contains(help.String(), "crusade -m box") || !strings.Contains(help.String(), "crusade -m align -r limit 10") || !strings.Contains(help.String(), "crusade -m do -c") || !strings.Contains(help.String(), "radial menu") && strings.Contains(help.String(), "do list") {
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
	data, err := os.ReadFile(filepath.Join(home, "config", "crusade", "config.json"))
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
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	defer func() { nowFunc = time.Now }()

	addArgs := []string{
		"-m", "align", "-c",
		"x", "2026-01-26",
		"y", "ship the patch",
		"z", "reduce drift",
	}
	if code := Run(addArgs, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add code = %d", code)
	}
	var list bytes.Buffer
	if code := Run([]string{"-m", "align", "-r", "limit", "5"}, &list, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list code = %d output=%q", code, list.String())
	}
	if !strings.Contains(list.String(), "id: 1") || !strings.Contains(list.String(), "ship the patch") || !strings.Contains(list.String(), "x: 2026-01-26") || strings.Contains(list.String(), "00:00") || strings.Contains(list.String(), "p:") {
		t.Fatalf("list output missing item:\n%s", list.String())
	}
	if code := Run([]string{"-m", "align", "-u", "1", "y", "updated outcome"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("edit code = %d", code)
	}
	var edited bytes.Buffer
	if code := Run([]string{"-m", "align", "-r"}, &edited, &bytes.Buffer{}); code != 0 {
		t.Fatalf("edited list code = %d", code)
	}
	if !strings.Contains(edited.String(), "updated outcome") || strings.Contains(edited.String(), "p:") {
		t.Fatalf("edited output missing update:\n%s", edited.String())
	}
	var rejected bytes.Buffer
	if code := Run([]string{"-m", "align", "-u", "1", "p", "8"}, &rejected, &bytes.Buffer{}); code == 0 || !strings.Contains(rejected.String(), "Unknown field 'p'") {
		t.Fatalf("score edit code=%d output=%q", code, rejected.String())
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

func TestAlignEditorUsesYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing.txt"))
	t.Setenv("VISUAL", "")
	nowFunc = func() time.Time { return time.Date(2026, 1, 26, 15, 4, 5, 0, time.Local) }
	defer func() { nowFunc = time.Now }()
	script := filepath.Join(home, "edit-align")
	copyPath := filepath.Join(home, "opened.yaml")
	body := "#!/bin/sh\ncp \"$1\" \"" + copyPath + "\"\ncat > \"$1\" <<'EOF'\nx: 2026-04-02\ny: wrote the note\nz: keep the trail\nEOF\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)
	if code := Run([]string{"-m", "align", "-c"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("create code = %d", code)
	}
	opened, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(opened)
	if !strings.Contains(text, "x: 2026-01-26") || strings.Contains(text, "nsm") || strings.Contains(text, "{") || strings.Contains(text, `"`) || strings.Contains(text, "'") {
		t.Fatalf("opened yaml:\n%s", text)
	}
	records, err := store.Load(store.DatabasePath())
	if err != nil || len(records) != 1 || records[0].Event.Y != "wrote the note" || records[0].Event.X.Format("2006-01-02") != "2026-04-02" {
		t.Fatalf("saved %#v err=%v", records, err)
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
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	defer func() { nowFunc = time.Now }()

	think, err := model.NewEvent("2026-02-01 00:00:00", "ship the patch", "reduce drift")
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

func TestOpenUsesTheRadial(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := store.DatabasePath()
	if _, err := store.Open(db); err != nil {
		t.Fatal(err)
	}
	previousPick := pickMode
	previousTUI := runTUI
	previousDo := openDoFile
	t.Cleanup(func() {
		pickMode = previousPick
		runTUI = previousTUI
		openDoFile = previousDo
	})
	var prefs []string
	var opened []string
	pickMode = func(preferred string) (string, error) {
		prefs = append(prefs, preferred)
		if len(opened) == 0 {
			return model.ModeBox, nil
		}
		return "", nil
	}
	runTUI = func(dataPath, mode string, stdout, stderr io.Writer) int {
		if dataPath != db {
			t.Fatalf("data path = %s", dataPath)
		}
		if err := store.SaveLastMode(dataPath, mode); err != nil {
			t.Fatal(err)
		}
		opened = append(opened, mode)
		return 0
	}
	openDoFile = func(string) error {
		t.Fatal("do file")
		return nil
	}
	var out bytes.Buffer
	if code := Run([]string{"open"}, &out, &bytes.Buffer{}); code != 0 || strings.Join(prefs, ",") != model.ModeDo+","+model.ModeBox || strings.Join(opened, ",") != model.ModeBox {
		t.Fatalf("code=%d prefs=%v opened=%v out=%q", code, prefs, opened, out.String())
	}
}

func TestOpenReturnsToTheRadialAfterDo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := store.DatabasePath()
	if _, err := store.Open(db); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLastMode(db, model.ModeCal); err != nil {
		t.Fatal(err)
	}
	previousPick := pickMode
	previousTUI := runTUI
	previousDo := openDoFile
	t.Cleanup(func() {
		pickMode = previousPick
		runTUI = previousTUI
		openDoFile = previousDo
	})
	var prefs []string
	doOpened := false
	pickMode = func(preferred string) (string, error) {
		prefs = append(prefs, preferred)
		if !doOpened {
			return model.ModeDo, nil
		}
		return "", nil
	}
	openDoFile = func(string) error {
		doOpened = true
		return nil
	}
	runTUI = func(string, string, io.Writer, io.Writer) int {
		t.Fatal("tui")
		return 1
	}
	if code := Run([]string{"open"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 || !doOpened || strings.Join(prefs, ",") != model.ModeCal+","+model.ModeDo {
		t.Fatalf("code=%d do=%v prefs=%v", code, doOpened, prefs)
	}
}

func TestOpenLinkClosesInsteadOfReturning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	if _, err := store.Open(store.DatabasePath()); err != nil {
		t.Fatal(err)
	}
	previousPick := pickMode
	previousTUI := runTUI
	t.Cleanup(func() {
		pickMode = previousPick
		runTUI = previousTUI
		tui.ExitAfterMode = false
	})
	calls := 0
	pickMode = func(string) (string, error) {
		calls++
		return model.ModeCal, nil
	}
	runTUI = func(string, string, io.Writer, io.Writer) int {
		tui.ExitAfterMode = true
		return 0
	}
	if code := Run([]string{"open"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 || calls != 1 {
		t.Fatalf("code=%d calls=%d", code, calls)
	}
}

func TestCursorSequencesSkipOrdinaryWriters(t *testing.T) {
	var buf bytes.Buffer
	concealCursor(&buf)
	revealCursor(&buf)
	if buf.Len() != 0 {
		t.Fatalf("buffer received %q", buf.String())
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	concealCursor(file)
	revealCursor(file)
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("file received %d bytes", info.Size())
	}
}

func TestCursorSequencesReachATerminal(t *testing.T) {
	master, slave := openTestPTY(t)
	t.Cleanup(func() {
		master.Close()
		slave.Close()
	})
	concealCursor(slave)
	revealCursor(slave)
	if err := master.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, err := master.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "\x1b[?25l\x1b[?25h" {
		t.Fatalf("terminal received %q", got)
	}
}

func openTestPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("pty numbers come from linux ioctls")
	}
	master, err := os.OpenFile("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	return master, slave
}

func TestDoModeOpensTheListFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "nvim")
	logPath := filepath.Join(home, "editor.log")
	editor := filepath.Join(home, "editor")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + logPath + "'\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := dofile.VimArgs
	dofile.VimArgs = []string{editor}
	t.Cleanup(func() { dofile.VimArgs = previous })
	if code := Run([]string{"-m", "do", "-c", "mail the letter"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal(code)
	}
	var out bytes.Buffer
	if code := Run([]string{"-m", "do"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("open code=%d out=%s", code, out.String())
	}
	listPath := filepath.Join(home, "config", "crusade", "do")
	body, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mail the letter\n" {
		t.Fatalf("list = %q", body)
	}
	if err := os.WriteFile(listPath, []byte("keep this\n- grouped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"-m", "do", "-c", "second item"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal(code)
	}
	if code := Run([]string{"-m", "do"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("second open code=%d out=%s", code, out.String())
	}
	body, err = os.ReadFile(listPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "keep this\n- grouped\n" {
		t.Fatalf("list rewritten:\n%s", body)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(logged), listPath) != 2 {
		t.Fatalf("editor log = %q", logged)
	}
}

func writeBoxEditor(t *testing.T, path, document string) {
	t.Helper()
	body := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n" + document + "EOF\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
