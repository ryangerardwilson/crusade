package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/dofile"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/radial"
	"github.com/ryangerardwilson/crusade/internal/store"
	"github.com/ryangerardwilson/crusade/internal/tui"
	"github.com/ryangerardwilson/crusade/internal/version"
)

const installScriptURL = "https://raw.githubusercontent.com/ryangerardwilson/crusade/main/install.sh"

func Run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0 || sameArgs(args, "help"):
		fmt.Fprint(stdout, HelpText)
		return 0
	case sameArgs(args, "version"):
		fmt.Fprintln(stdout, version.Version)
		return 0
	case sameArgs(args, "upgrade"):
		return upgrade(stdout, stderr)
	case len(args) > 0 && isGlobalAction(args[0]):
		fmt.Fprintln(stdout, "Use help, version, or upgrade by itself.")
		return 1
	default:
		return dispatch(args, stdout, stderr)
	}
}

var runTUI = tui.Run
var pickMode = radial.Pick
var openDoFile = dofile.Open

func openRadial(dbPath string, stdout, stderr io.Writer) int {
	for {
		concealCursor(stdout)
		code, done := runRadialOnce(dbPath, stdout, stderr)
		if done {
			return code
		}
	}
}

func openSession(stdout, stderr io.Writer) int {
	concealCursor(stdout)
	defer revealCursor(stdout)
	if _, err := config.Load(); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	dbPath := store.DatabasePath()
	if _, err := store.Open(dbPath); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return openRadial(dbPath, stdout, stderr)
}

// The radial keeps the keyboard, so Alacritty draws its unfocused block
// in the first cell. Hide that cursor on a terminal and leave other writers alone.
func concealCursor(w io.Writer) {
	writeCursor(w, "\x1b[?25l")
}

func revealCursor(w io.Writer) {
	writeCursor(w, "\x1b[?25h")
}

func writeCursor(w io.Writer, sequence string) {
	file, ok := w.(*os.File)
	if !ok {
		return
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	_, _ = io.WriteString(file, sequence)
}

func runRadialOnce(dbPath string, stdout, stderr io.Writer) (int, bool) {
	preferred, err := store.LastMode(dbPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1, true
	}
	chosen, err := pickMode(preferred)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1, true
	}
	if chosen == "" {
		return 0, true
	}
	result := launchChosen(dbPath, chosen, stdout, stderr)
	if result.Code != 0 || result.Closed {
		return result.Code, true
	}
	return 0, false
}

func launchChosen(dbPath, mode string, stdout, stderr io.Writer) tui.Result {
	switch mode {
	case model.ModeDo:
		return tui.Result{Code: openDo(dbPath, stdout)}
	case model.ModeAlign, model.ModeBox, model.ModeCal, model.ModeNLP:
		return runTUI(dbPath, mode, stdout, stderr)
	default:
		fmt.Fprintln(stdout, "Unknown mode.")
		return tui.Result{Code: 1}
	}
}

func openDo(dataPath string, stdout io.Writer) int {
	if err := store.SaveLastMode(dataPath, model.ModeDo); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := openDoFile(dataPath); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	if sameArgs(args, "open") {
		return openSession(stdout, stderr)
	}
	if _, err := config.Load(); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if len(args) >= 1 && args[0] == "config" {
		if len(args) != 1 {
			fmt.Fprintln(stdout, "Usage: crusade config")
			return 1
		}
		return openConfig(stdout)
	}
	dbPath := store.DatabasePath()
	if _, err := store.Open(dbPath); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}

	switch {
	case len(args) > 0 && args[0] == "open":
		fmt.Fprintln(stdout, "Usage: crusade open")
		return 1
	case sameArgs(args, "explain", "fields"):
		printFieldMeanings(stdout)
		return 0
	case len(args) >= 2 && args[0] == "explain" && args[1] == "fields":
		fmt.Fprintln(stdout, "Usage: crusade explain fields")
		return 1
	default:
		return runCRUD(dbPath, args, stdout, stderr)
	}
}

func runCRUD(dataPath string, args []string, stdout, stderr io.Writer) int {
	mode, op, rest, err := parseModeOp(args)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if op == "" {
		if mode == model.ModeDo {
			return openDo(dataPath, stdout)
		}
		return runTUI(dataPath, mode, stdout, stderr).Code
	}
	switch mode {
	case model.ModeDo:
		switch op {
		case "c":
			return createDo(dataPath, rest, stdout)
		case "r":
			return readDo(dataPath, rest, stdout)
		case "u":
			return updateDo(dataPath, rest, stdout)
		case "d":
			return deleteDo(dataPath, rest, stdout)
		}
	case model.ModeAlign:
		switch op {
		case "c":
			return createAlign(dataPath, rest, stdout)
		case "r":
			return readAlign(dataPath, rest, stdout)
		case "u":
			return updateAlign(dataPath, rest, stdout)
		case "d":
			return deleteAlign(dataPath, rest, stdout)
		}
	case model.ModeBox:
		return runBox(dataPath, op, rest, stdout)
	case model.ModeCal:
		fmt.Fprintln(stdout, "Usage: crusade -m cal opens the calendar. It blocks time and does not take -c, -r, -u, or -d.")
		return 1
	case model.ModeNLP:
		fmt.Fprintln(stdout, "Usage: crusade -m nlp")
		return 1
	}
	fmt.Fprintln(stdout, "Unknown command. Run: crusade help")
	return 1
}

func parseModeOp(args []string) (string, string, []string, error) {
	var mode, op string
	rest := []string{}
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == "--" {
			rest = append(rest, args[idx+1:]...)
			break
		}
		switch arg {
		case "-m":
			if mode != "" {
				return "", "", nil, fmt.Errorf("Use -m once. Choose do, align, box, cal, or nlp.")
			}
			if idx+1 >= len(args) {
				return "", "", nil, fmt.Errorf("Usage: crusade -m do|align -c|-r|-u|-d [args]")
			}
			idx++
			switch args[idx] {
			case model.ModeDo, model.ModeAlign, model.ModeBox, model.ModeCal, model.ModeNLP:
				mode = args[idx]
			default:
				return "", "", nil, fmt.Errorf("Use -m do, -m align, -m box, -m cal, or -m nlp.")
			}
		case "-c", "-r", "-u", "-d":
			if op != "" {
				return "", "", nil, fmt.Errorf("Choose one of -c, -r, -u, or -d.")
			}
			op = strings.TrimPrefix(arg, "-")
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", nil, fmt.Errorf("Unknown flag '%s'. Run: crusade help", arg)
			}
			rest = append(rest, arg)
		}
	}
	if mode == "" && op == "" {
		return "", "", nil, fmt.Errorf("Unknown command. Run: crusade help")
	}
	if mode == "" {
		return "", "", nil, fmt.Errorf("Usage: crusade -m do|align|box|cal|nlp [-c|-r|-u|-d] [args]")
	}
	if op == "" {
		if len(rest) != 0 {
			return "", "", nil, fmt.Errorf("Usage: crusade -m %s opens that mode. Add -c, -r, -u, or -d to change records.", mode)
		}
		return mode, "", nil, nil
	}
	return mode, op, rest, nil
}

func createDo(dataPath string, args []string, stdout io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stdout, "Usage: crusade -m do -c <outcome>")
		return 1
	}
	if _, err := store.AddDoItem(dataPath, 0, args[0]); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func updateDo(dataPath string, args []string, stdout io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stdout, "Usage: crusade -m do -u <id> <text>")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.UpdateDoItem(dataPath, id, args[1]); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func deleteDo(dataPath string, args []string, stdout io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stdout, "Usage: crusade -m do -d <id>")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.DeleteDoItem(dataPath, id); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func readDo(dataPath string, args []string, stdout io.Writer) int {
	limit, ok := parseLimit(model.ModeDo, args, stdout)
	if !ok {
		return 1
	}
	rows, err := store.LoadDoList(dataPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	printDoList(rows, stdout)
	return 0
}

func createAlign(dataPath string, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		event, ok, err := editAlign(model.DefaultAlign(nowFunc()))
		if err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
		if !ok {
			return 0
		}
		if err := store.Add(dataPath, event); err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
		return 0
	}
	fields, err := parseLabeledValues(args, map[string]bool{"x": true, "y": true, "z": true})
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	missing := missingFields(fields, []string{"x", "y", "z"})
	if len(missing) > 0 {
		fmt.Fprintf(stdout, "Missing required field(s): %s for direct add\n", strings.Join(missing, ", "))
		return 1
	}
	event, err := model.NewAlign(fields["x"], fields["y"], fields["z"])
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.Add(dataPath, event); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func updateAlign(dataPath string, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "Usage: crusade -m align -u <id> [x <YYYY-MM-DD>] [y <value>] [z <value>]")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	original, code := loadAlignRecord(dataPath, id, stdout)
	if code != 0 {
		return code
	}
	if len(args) == 1 {
		event, ok, err := editAlign(original)
		if err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
		if !ok {
			return 0
		}
		if err := store.Update(dataPath, id, event); err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
		return 0
	}
	fields, err := parseLabeledValues(args[1:], map[string]bool{"x": true, "y": true, "z": true})
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	event, err := original.WithUpdates(fields)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.Update(dataPath, id, event); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func readAlign(dataPath string, args []string, stdout io.Writer) int {
	limit, ok := parseLimit(model.ModeAlign, args, stdout)
	if !ok {
		return 1
	}
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	filtered := make([]store.AlignRecord, 0, len(records))
	now := nowFunc()
	for _, record := range records {
		if !record.Align.X.Before(now) {
			filtered = append(filtered, record)
		}
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	printAlignList(filtered, stdout)
	return 0
}

func parseLimit(mode string, args []string, stdout io.Writer) (int, bool) {
	if len(args) == 0 {
		return 0, true
	}
	if len(args) != 2 || args[0] != "limit" {
		fmt.Fprintf(stdout, "Usage: crusade -m %s -r [limit <count>]\n", mode)
		return 0, false
	}
	parsed, err := parsePositiveInt(args[1], "List count")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 0, false
	}
	return parsed, true
}

func deleteAlign(dataPath string, args []string, stdout io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stdout, "Usage: crusade -m align -d <id>")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.Delete(dataPath, id); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func loadAlignRecord(dataPath string, id int, stdout io.Writer) (model.Align, int) {
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return model.Align{}, 1
	}
	original, found := eventByID(records, id)
	if !found {
		fmt.Fprintf(stdout, "ID %d not found\n", id)
		return model.Align{}, 1
	}
	return original, 0
}

func eventByID(records []store.AlignRecord, id int) (model.Align, bool) {
	for _, record := range records {
		if record.ID == id {
			return record.Align, true
		}
	}
	return model.Align{}, false
}

func openConfig(stdout io.Writer) int {
	if err := config.Ensure(); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vim"}
	}
	cmd := exec.Command(parts[0], append(parts[1:], config.Path())...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func editAlign(seed model.Align) (model.Align, bool, error) {
	tmp, err := os.CreateTemp("", "crusade-*.yaml")
	if err != nil {
		return model.Align{}, false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	payload, err := model.ToYAML(seed)
	if err != nil {
		_ = tmp.Close()
		return model.Align{}, false, err
	}
	if _, err := tmp.Write(append(payload, '\n')); err != nil {
		_ = tmp.Close()
		return model.Align{}, false, err
	}
	if err := tmp.Close(); err != nil {
		return model.Align{}, false, err
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vim"}
	}
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return model.Align{}, false, nil
		}
		return model.Align{}, false, err
	}
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return model.Align{}, false, err
	}
	event, err := model.FromYAML(edited)
	if err != nil {
		if errors.Is(err, model.ErrBlankAlign) {
			return model.Align{}, false, nil
		}
		return model.Align{}, false, err
	}
	event.DoneAt = seed.DoneAt
	return event, true, nil
}

func upgrade(stdout, stderr io.Writer) int {
	if override := os.Getenv("XYZ_INSTALL_SCRIPT"); override != "" {
		cmd := exec.Command("bash", override, "upgrade")
		cmd.Stdin = os.Stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return runCmd(cmd, stderr)
	}
	cmd := exec.Command("bash", "-c", "curl -fsSL "+installScriptURL+" | bash -s -- upgrade")
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return runCmd(cmd, stderr)
}

func runCmd(cmd *exec.Cmd, stderr io.Writer) int {
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func printFieldMeanings(stdout io.Writer) {
	fmt.Fprint(stdout, "Idea:\n")
	fmt.Fprint(stdout, "  \"When X happens, I want Y outcome, so that I can drive Z impact\"\n\n")
	fmt.Fprint(stdout, "XYZ Fields:\n")
	fmt.Fprint(stdout, "  x: the day this happens, as YYYY-MM-DD\n")
	fmt.Fprint(stdout, "  y: desired progress/outcome\n")
	fmt.Fprint(stdout, "  z: why it matters / impact\n")
}

func printAlignList(records []store.AlignRecord, stdout io.Writer) {
	title := fmt.Sprintf("crusade align items (%d)", len(records))
	fmt.Fprintln(stdout, title)
	fmt.Fprintln(stdout, strings.Repeat("-", len(title)))
	if len(records) == 0 {
		fmt.Fprintln(stdout, "No upcoming items.")
		return
	}
	for idx, record := range records {
		event := record.Align
		fmt.Fprintf(stdout, "   id: %d\n", record.ID)
		fmt.Fprintf(stdout, "%d. x: %s\n", idx+1, event.X.Format("2006-01-02"))
		wrapLine(stdout, "   y: ", event.Y)
		wrapLine(stdout, "   z: ", event.Z)
		if idx < len(records)-1 {
			fmt.Fprintln(stdout, strings.Repeat("-", 36))
		}
	}
}

// printDoList shows the do list as Vim shows it, each row with its id.
func printDoList(rows []model.DoItem, stdout io.Writer) {
	items := 0
	for _, row := range rows {
		if row.Kind == model.DoKindItem {
			items++
		}
	}
	title := fmt.Sprintf("crusade do items (%d)", items)
	fmt.Fprintln(stdout, title)
	fmt.Fprintln(stdout, strings.Repeat("-", len(title)))
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "No items.")
		return
	}
	for _, row := range rows {
		fmt.Fprintf(stdout, "%4d  %s\n", row.ID, row.Line())
	}
}

func wrapLine(stdout io.Writer, prefix, text string) {
	wrapped := wrapWords(text, 100-len(prefix))
	if len(wrapped) == 0 {
		fmt.Fprintln(stdout, prefix)
		return
	}
	for idx, line := range wrapped {
		if idx == 0 {
			fmt.Fprintln(stdout, prefix+line)
		} else {
			fmt.Fprintln(stdout, "      "+line)
		}
	}
}

func wrapWords(text string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []string{text}
	}
	lines := []string{}
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	lines = append(lines, current)
	return lines
}

func parseLabeledValues(args []string, allowed map[string]bool) (map[string]string, error) {
	fields := map[string]string{}
	for idx := 0; idx < len(args); idx += 2 {
		key := args[idx]
		if !allowed[key] {
			return nil, fmt.Errorf("Unknown field '%s'", key)
		}
		if idx+1 >= len(args) {
			return nil, fmt.Errorf("%s requires a value", key)
		}
		fields[key] = args[idx+1]
	}
	return fields, nil
}

func parsePositiveInt(raw string, label string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", label)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be >= 1", label)
	}
	return value, nil
}

func missingFields(fields map[string]string, required []string) []string {
	missing := []string{}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func sameArgs(args []string, want ...string) bool {
	if len(args) != len(want) {
		return false
	}
	for idx := range args {
		if args[idx] != want[idx] {
			return false
		}
	}
	return true
}

func isGlobalAction(value string) bool {
	return value == "help" || value == "version" || value == "upgrade"
}

var nowFunc = func() time.Time { return time.Now() }
