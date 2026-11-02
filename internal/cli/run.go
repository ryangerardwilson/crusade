package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/model"
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

func dispatch(args []string, stdout, stderr io.Writer) int {
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
	case sameArgs(args, "open"):
		mode, err := store.LastMode(dbPath)
		if err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
		return runTUI(dbPath, mode, stdout, stderr)
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
		return tui.Run(dataPath, mode, stdout, stderr)
	}
	switch mode {
	case model.ModeDo:
		switch op {
		case "c":
			return createDo(dataPath, rest, stdout)
		case "r":
			return readRecords(dataPath, model.ModeDo, rest, stdout)
		case "u":
			return updateDo(dataPath, rest, stdout)
		case "d":
			return deleteRecord(dataPath, model.ModeDo, rest, stdout)
		}
	case model.ModeAlign:
		switch op {
		case "c":
			return createAlign(dataPath, rest, stdout)
		case "r":
			return readRecords(dataPath, model.ModeAlign, rest, stdout)
		case "u":
			return updateAlign(dataPath, rest, stdout)
		case "d":
			return deleteRecord(dataPath, model.ModeAlign, rest, stdout)
		}
	case model.ModeBox:
		return runBox(dataPath, op, rest, stdout)
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
				return "", "", nil, fmt.Errorf("Use -m once. Choose do, align, box, or nlp.")
			}
			if idx+1 >= len(args) {
				return "", "", nil, fmt.Errorf("Usage: crusade -m do|align|box|nlp -c|-r|-u|-d [args]")
			}
			idx++
			switch args[idx] {
			case model.ModeDo, model.ModeAlign, model.ModeBox, model.ModeNLP:
				mode = args[idx]
			default:
				return "", "", nil, fmt.Errorf("Use -m do, -m align, -m box, or -m nlp.")
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
		return "", "", nil, fmt.Errorf("Usage: crusade -m do|align|box|nlp [-c|-r|-u|-d] [args]")
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
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(stdout, "Usage: crusade -m do -c <outcome> [when]")
		return 1
	}
	when, err := doWhen(args, 1)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	event, err := model.NewDoEvent(when, args[0])
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

func updateDo(dataPath string, args []string, stdout io.Writer) int {
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintln(stdout, "Usage: crusade -m do -u <id> <outcome> [when]")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	original, code := loadModeRecord(dataPath, id, model.ModeDo, stdout)
	if code != 0 {
		return code
	}
	when := original.X.Format(model.DateTimeFormat)
	if len(args) == 3 {
		when, err = normalizeWhen(args[2])
		if err != nil {
			fmt.Fprintln(stdout, err)
			return 1
		}
	}
	event, err := model.NewDoEvent(when, args[1])
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	event.Done = original.Done
	if err := store.Update(dataPath, id, event); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func doWhen(args []string, index int) (string, error) {
	if index >= len(args) {
		day := nowFunc()
		stamp := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())
		return stamp.Format(model.DateTimeFormat), nil
	}
	return normalizeWhen(args[index])
}

func normalizeWhen(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) == 10 {
		if day, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
			stamp := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.Local)
			return stamp.Format(model.DateTimeFormat), nil
		}
	}
	parsed, err := model.ParseDateTime(value)
	if err != nil {
		return "", err
	}
	return parsed.Format(model.DateTimeFormat), nil
}

func createAlign(dataPath string, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		event, ok, err := editEvent(model.DefaultEvent(nowFunc()))
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
	fields, err := parseLabeledValues(args, map[string]bool{"x": true, "y": true, "z": true, "p": true, "q": true, "r": true})
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	missing := missingFields(fields, []string{"x", "y", "z", "p", "q", "r"})
	if len(missing) > 0 {
		fmt.Fprintf(stdout, "Missing required field(s): %s for direct add\n", strings.Join(missing, ", "))
		return 1
	}
	event, err := model.NewEvent(fields["x"], fields["y"], fields["z"], fields["p"], fields["q"], fields["r"])
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
		fmt.Fprintln(stdout, "Usage: crusade -m align -u <id> [x <value>] [y <value>] [z <value>] [p <score>] [q <score>] [r <score>]")
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	original, code := loadModeRecord(dataPath, id, model.ModeAlign, stdout)
	if code != 0 {
		return code
	}
	if len(args) == 1 {
		event, ok, err := editEvent(original)
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
	fields, err := parseLabeledValues(args[1:], map[string]bool{"x": true, "y": true, "z": true, "p": true, "q": true, "r": true})
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

func readRecords(dataPath string, mode string, args []string, stdout io.Writer) int {
	limit, ok := parseLimit(mode, args, stdout)
	if !ok {
		return 1
	}
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	filtered := make([]store.Record, 0, len(records))
	now := nowFunc()
	for _, record := range records {
		if mode == model.ModeDo {
			if !record.Event.IsDo() {
				continue
			}
		} else if record.Event.IsDo() || record.Event.X.Before(now) {
			continue
		}
		filtered = append(filtered, record)
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	if mode == model.ModeDo {
		printDoList(filtered, stdout)
	} else {
		printAlignList(filtered, stdout)
	}
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

func deleteRecord(dataPath string, mode string, args []string, stdout io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintf(stdout, "Usage: crusade -m %s -d <id>\n", mode)
		return 1
	}
	id, err := parsePositiveInt(args[0], "ID")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if _, code := loadModeRecord(dataPath, id, mode, stdout); code != 0 {
		return code
	}
	if err := store.Delete(dataPath, id); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	return 0
}

func loadModeRecord(dataPath string, id int, mode string, stdout io.Writer) (model.Event, int) {
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return model.Event{}, 1
	}
	original, found := eventByID(records, id)
	if !found {
		fmt.Fprintf(stdout, "ID %d not found\n", id)
		return model.Event{}, 1
	}
	if mode == model.ModeDo && !original.IsDo() {
		fmt.Fprintf(stdout, "ID %d is an align record.\n", id)
		return model.Event{}, 1
	}
	if mode == model.ModeAlign && original.IsDo() {
		fmt.Fprintf(stdout, "ID %d is a do record.\n", id)
		return model.Event{}, 1
	}
	return original, 0
}
func eventByID(records []store.Record, id int) (model.Event, bool) {
	for _, record := range records {
		if record.ID == id {
			return record.Event, true
		}
	}
	return model.Event{}, false
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

func editEvent(seed model.Event) (model.Event, bool, error) {
	tmp, err := os.CreateTemp("", "xyz-*.json")
	if err != nil {
		return model.Event{}, false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	payload, err := json.MarshalIndent(model.ToJSONEvent(seed), "", "  ")
	if err != nil {
		_ = tmp.Close()
		return model.Event{}, false, err
	}
	if _, err := tmp.Write(append(payload, '\n')); err != nil {
		_ = tmp.Close()
		return model.Event{}, false, err
	}
	if err := tmp.Close(); err != nil {
		return model.Event{}, false, err
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
			return model.Event{}, false, nil
		}
		return model.Event{}, false, err
	}
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return model.Event{}, false, err
	}
	event, err := model.FromJSON(edited)
	if err != nil {
		return model.Event{}, false, err
	}
	event.Done = seed.Done
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
	fmt.Fprint(stdout, "  x: target trigger datetime (when outcome should be checked)\n")
	fmt.Fprint(stdout, "  y: desired progress/outcome\n")
	fmt.Fprint(stdout, "  z: why it matters / impact\n\n")
	fmt.Fprint(stdout, "PQR Scores (0-10):\n")
	fmt.Fprint(stdout, "  p: Jesus' will alignment\n")
	fmt.Fprint(stdout, "  q: outward impact\n")
	fmt.Fprint(stdout, "  r: embodied practice\n")
}

func printAlignList(records []store.Record, stdout io.Writer) {
	title := fmt.Sprintf("crusade align items (%d)", len(records))
	fmt.Fprintln(stdout, title)
	fmt.Fprintln(stdout, strings.Repeat("-", len(title)))
	if len(records) == 0 {
		fmt.Fprintln(stdout, "No upcoming items.")
		return
	}
	for idx, record := range records {
		event := record.Event
		avg := (event.P + event.Q + event.R) / 3
		fmt.Fprintf(stdout, "   id: %d\n", record.ID)
		fmt.Fprintf(stdout, "%d. x: %s\n", idx+1, event.X.Format(model.DateTimeFormat))
		wrapLine(stdout, "   y: ", event.Y)
		wrapLine(stdout, "   z: ", event.Z)
		fmt.Fprintf(stdout, "   p: %s  q: %s  r: %s  avg: %s\n", model.FormatMetric(event.P), model.FormatMetric(event.Q), model.FormatMetric(event.R), model.FormatMetric(avg))
		if idx < len(records)-1 {
			fmt.Fprintln(stdout, strings.Repeat("-", 36))
		}
	}
}

func printDoList(records []store.Record, stdout io.Writer) {
	title := fmt.Sprintf("crusade do items (%d)", len(records))
	fmt.Fprintln(stdout, title)
	fmt.Fprintln(stdout, strings.Repeat("-", len(title)))
	if len(records) == 0 {
		fmt.Fprintln(stdout, "No items.")
		return
	}
	for idx, record := range records {
		event := record.Event
		fmt.Fprintf(stdout, "%d. id: %d\n", idx+1, record.ID)
		fmt.Fprintf(stdout, "   when: %s\n", event.X.Format(model.DateTimeFormat))
		wrapLine(stdout, "   outcome: ", event.Y)
		if idx < len(records)-1 {
			fmt.Fprintln(stdout, strings.Repeat("-", 36))
		}
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
