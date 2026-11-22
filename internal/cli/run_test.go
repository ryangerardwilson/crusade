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

	"github.com/ryangerardwilson/crusade/internal/box"
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
	if !strings.Contains(help.String(), "The order is do, cal, box, nlp, align.") {
		t.Fatalf("help output missing the radial order:\n%s", help.String())
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
	testHome(t)
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
	if err != nil || len(records) != 1 || records[0].Align.Y != "wrote the note" || records[0].Align.X.Format("2006-01-02") != "2026-04-02" {
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

func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	return home
}

func runOut(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := Run(args, &out, &bytes.Buffer{})
	return code, out.String()
}

func TestAlignListAndEditKeepRecords(t *testing.T) {
	testHome(t)
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local) }
	defer func() { nowFunc = time.Now }()

	path := store.DatabasePath()
	if _, err := store.Open(path); err != nil {
		t.Fatal(err)
	}
	think, err := model.NewAlign("2026-02-01 00:00:00", "ship the patch", "reduce drift")
	if err != nil {
		t.Fatal(err)
	}
	past, err := model.NewAlign("2025-12-01", "old outcome", "gone by")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []model.Align{think, past} {
		if err := store.Add(path, event); err != nil {
			t.Fatal(err)
		}
	}
	if code, list := runOut(t, "-m", "align", "-r"); code != 0 || !strings.Contains(list, "ship the patch") || strings.Contains(list, "old outcome") || !strings.Contains(list, "crusade align items (1)") {
		t.Fatalf("list code=%d:\n%s", code, list)
	}
	if code, out := runOut(t, "-m", "align", "-u", "9", "y", "changed"); code == 0 || !strings.Contains(out, "ID 9 not found") {
		t.Fatalf("missing id code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "align", "-u", "1", "z", ""); code == 0 || !strings.Contains(out, "impact") {
		t.Fatalf("empty z edit code=%d output=%q", code, out)
	}
	if code, again := runOut(t, "-m", "align", "-r"); code != 0 || !strings.Contains(again, "reduce drift") {
		t.Fatalf("align impact was cleared:\n%s", again)
	}
	if code, out := runOut(t, "list", "outcomes"); code == 0 || !strings.Contains(out, "Unknown command") {
		t.Fatalf("old list command code=%d output=%q", code, out)
	}
}

func seedDoList(t *testing.T, text string) string {
	t.Helper()
	db := store.DatabasePath()
	if _, err := store.Open(db); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceDoList(db, model.ParseDoList(text)); err != nil {
		t.Fatal(err)
	}
	return db
}

func loadDoText(t *testing.T, db string) ([]model.DoItem, string) {
	t.Helper()
	rows, err := store.LoadDoList(db)
	if err != nil {
		t.Fatal(err)
	}
	return rows, model.RenderDoList(rows)
}

func TestDoCreateAppendsAnItem(t *testing.T) {
	testHome(t)
	db := seedDoList(t, "# Work\n\n- mail the letter\n")
	if code, out := runOut(t, "-m", "do", "-c", "write the note"); code != 0 || out != "" {
		t.Fatalf("create code=%d output=%q", code, out)
	}
	rows, text := loadDoText(t, db)
	if text != "# Work\n\n- mail the letter\n- write the note\n" || rows[3].Kind != model.DoKindItem || rows[3].ID == 0 {
		t.Fatalf("rows = %#v", rows)
	}
	if code, out := runOut(t, "-m", "do", "-c", " "); code == 0 || !strings.Contains(out, "outcome cannot be empty") {
		t.Fatalf("empty create code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-c", "a", "b"); code == 0 || !strings.Contains(out, "Usage: crusade -m do -c") {
		t.Fatalf("two args code=%d output=%q", code, out)
	}
}

func TestDoReadPrintsEveryRowWithItsID(t *testing.T) {
	testHome(t)
	db := seedDoList(t, "# Work\n\n- mail the letter\n")
	if code, _ := runOut(t, "-m", "do", "-c", "x"); code != 0 {
		t.Fatal(code)
	}
	rows, _ := loadDoText(t, db)
	code, out := runOut(t, "-m", "do", "-r")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != 0 || len(lines) != 6 || lines[0] != "crusade do items (2)" || lines[1] != strings.Repeat("-", len(lines[0])) {
		t.Fatalf("read code=%d:\n%s", code, out)
	}
	for index, row := range rows {
		want := strconv.Itoa(row.ID) + "  " + row.Line()
		if got := strings.TrimLeft(lines[index+2], " "); got != want {
			t.Fatalf("line %d = %q want %q", index, got, want)
		}
	}
	if !strings.HasSuffix(lines[5], strconv.Itoa(rows[3].ID)+"  - x") {
		t.Fatalf("appended line = %q", lines[5])
	}
	if code, limited := runOut(t, "-m", "do", "-r", "limit", "1"); code != 0 || strings.Contains(limited, "mail the letter") || !strings.Contains(limited, "# Work") {
		t.Fatalf("limited code=%d:\n%s", code, limited)
	}
}

func TestDoReadEmptyList(t *testing.T) {
	testHome(t)
	if code, out := runOut(t, "-m", "do", "-r"); code != 0 || !strings.Contains(out, "crusade do items (0)") || !strings.Contains(out, "No items.") {
		t.Fatalf("empty code=%d:\n%s", code, out)
	}
}

func TestDoUpdateKeepsIDAndPlace(t *testing.T) {
	testHome(t)
	db := seedDoList(t, "# Work\n\n- mail the letter\n- call mum\n")
	before, _ := loadDoText(t, db)
	item := strconv.Itoa(before[2].ID)
	if code, out := runOut(t, "-m", "do", "-u", item, "sent the letter"); code != 0 {
		t.Fatalf("update code=%d output=%q", code, out)
	}
	heading := strconv.Itoa(before[0].ID)
	if code, out := runOut(t, "-m", "do", "-u", heading, "## Plan"); code != 0 {
		t.Fatalf("heading code=%d output=%q", code, out)
	}
	after, text := loadDoText(t, db)
	if text != "## Plan\n\n- sent the letter\n- call mum\n" || after[2].ID != before[2].ID {
		t.Fatalf("after = %#v", after)
	}
	if code, out := runOut(t, "-m", "do", "-u", heading, "Plan"); code == 0 || !strings.Contains(out, "starts with #") {
		t.Fatalf("bad heading code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-u", strconv.Itoa(before[1].ID), "text"); code == 0 || !strings.Contains(out, "blank line") {
		t.Fatalf("blank code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-u", "99", "x"); code == 0 || !strings.Contains(out, "ID 99 not found") {
		t.Fatalf("missing code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-u", item); code == 0 || !strings.Contains(out, "Usage: crusade -m do -u <id> <text>") {
		t.Fatalf("usage code=%d output=%q", code, out)
	}
}

func TestDoDeleteCountsTheWeek(t *testing.T) {
	testHome(t)
	db := seedDoList(t, "# Work\n- mail the letter\n- call mum\n")
	before, _ := loadDoText(t, db)
	if code, out := runOut(t, "-m", "do", "-d", strconv.Itoa(before[1].ID)); code != 0 {
		t.Fatalf("delete code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-d", strconv.Itoa(before[0].ID)); code != 0 {
		t.Fatalf("heading delete code=%d output=%q", code, out)
	}
	if _, text := loadDoText(t, db); text != "- call mum\n" {
		t.Fatalf("after = %q", text)
	}
	count, err := store.DeletedThisWeek(db, time.Now())
	if err != nil || count != 1 {
		t.Fatalf("deleted this week = %d err=%v", count, err)
	}
	if code, out := runOut(t, "-m", "do", "-d", "99"); code == 0 || !strings.Contains(out, "ID 99 not found") {
		t.Fatalf("missing code=%d output=%q", code, out)
	}
	if code, out := runOut(t, "-m", "do", "-d", "x"); code == 0 || !strings.Contains(out, "ID must be an integer") {
		t.Fatalf("bad id code=%d output=%q", code, out)
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
	email, ok := boxAt(outline, "work/email")
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
	if _, ok := boxAt(outline, "work/email"); ok {
		t.Fatal("replaced outline kept the old box")
	}
	if _, ok := boxAt(outline, "plan"); !ok {
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
	if _, ok := boxAt(outline, "plan"); !ok || len(outline.Flatten()) != 1 {
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
	runTUI = func(dataPath, mode string, stdout, stderr io.Writer) tui.Result {
		if dataPath != db {
			t.Fatalf("data path = %s", dataPath)
		}
		if err := store.SaveLastMode(dataPath, mode); err != nil {
			t.Fatal(err)
		}
		opened = append(opened, mode)
		return tui.Result{}
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
	runTUI = func(string, string, io.Writer, io.Writer) tui.Result {
		t.Fatal("tui")
		return tui.Result{Code: 1}
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
	})
	calls := 0
	pickMode = func(string) (string, error) {
		calls++
		return model.ModeCal, nil
	}
	runTUI = func(string, string, io.Writer, io.Writer) tui.Result {
		return tui.Result{Closed: true}
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

// stubVim makes dofile.VimArgs a script that logs the path it opens, copies
// that file to opened, and writes the file next over it when next exists.
func stubVim(t *testing.T, home string) (logPath, opened, next string) {
	t.Helper()
	logPath = filepath.Join(home, "editor.log")
	opened = filepath.Join(home, "opened")
	next = filepath.Join(home, "next")
	editor := filepath.Join(home, "editor")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + logPath + "'\ncp \"$1\" '" + opened + "'\n" +
		"if [ -f '" + next + "' ]; then cp '" + next + "' \"$1\"; fi\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := dofile.VimArgs
	dofile.VimArgs = []string{editor}
	t.Cleanup(func() { dofile.VimArgs = previous })
	return logPath, opened, next
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDoModeOpensTheListFile(t *testing.T) {
	home := testHome(t)
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "nvim")
	logPath, opened, next := stubVim(t, home)
	db := seedDoList(t, "# Work\n\n- mail the letter\n")
	before, _ := loadDoText(t, db)
	saved := "# Work\n\n- sent the letter\n\n## Home\n- call mum\n"
	if err := os.WriteFile(next, []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runOut(t, "-m", "do"); code != 0 {
		t.Fatalf("open code=%d out=%s", code, out)
	}
	listPath := config.DoPath()
	if listPath != filepath.Join(home, "config", "crusade", "do") {
		t.Fatalf("do path = %s", listPath)
	}
	if got := readText(t, opened); got != "# Work\n\n- mail the letter\n" {
		t.Fatalf("vim opened %q", got)
	}
	rows, text := loadDoText(t, db)
	if text != saved || rows[0].ID != before[0].ID || rows[2].ID != before[2].ID {
		t.Fatalf("stored rows = %#v", rows)
	}
	if mode, err := store.LastMode(db); err != nil || mode != model.ModeDo {
		t.Fatalf("last mode = %q err=%v", mode, err)
	}
	if strings.Count(readText(t, logPath), listPath) != 1 {
		t.Fatalf("editor log = %q", readText(t, logPath))
	}
}

func TestDoModeRewritesTheFileFromRows(t *testing.T) {
	home := testHome(t)
	logPath, opened, _ := stubVim(t, home)
	db := seedDoList(t, "# Work\n- mail the letter\n")
	listPath := config.DoPath()
	if err := os.MkdirAll(filepath.Dir(listPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(listPath, []byte("stale text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := runOut(t, "-m", "do", "-c", "second item"); code != 0 {
		t.Fatal(code)
	}
	if code, out := runOut(t, "-m", "do"); code != 0 {
		t.Fatalf("open code=%d out=%s", code, out)
	}
	want := "# Work\n- mail the letter\n- second item\n"
	if got := readText(t, opened); got != want {
		t.Fatalf("vim opened %q", got)
	}
	if _, text := loadDoText(t, db); text != want {
		t.Fatalf("rows changed: %q", text)
	}
	if strings.Count(readText(t, logPath), listPath) != 1 {
		t.Fatalf("editor log = %q", readText(t, logPath))
	}
}

func TestDoModeFailedVimKeepsRows(t *testing.T) {
	home := testHome(t)
	editor := filepath.Join(home, "fail")
	script := "#!/bin/sh\nprintf 'junk\\n' > \"$1\"\nexit 3\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := dofile.VimArgs
	dofile.VimArgs = []string{editor}
	t.Cleanup(func() { dofile.VimArgs = previous })
	db := seedDoList(t, "# Work\n- mail the letter\n")
	if code, _ := runOut(t, "-m", "do"); code != 3 {
		t.Fatalf("failed vim code = %d", code)
	}
	if _, text := loadDoText(t, db); text != "# Work\n- mail the letter\n" {
		t.Fatalf("rows changed: %q", text)
	}
}

func boxAt(outline box.Outline, path string) (box.FlatItem, bool) {
	flat := outline.Flatten()
	for index, p := range outline.Paths() {
		if p == path {
			return flat[index], true
		}
	}
	return box.FlatItem{}, false
}

func writeBoxEditor(t *testing.T, path, document string) {
	t.Helper()
	body := "#!/bin/sh\ncat > \"$1\" <<'EOF'\n" + document + "EOF\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
