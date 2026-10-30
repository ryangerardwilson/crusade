package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/model"
)

const DatabaseName = "crusade"

const legacyDatabaseName = "xecute"

const taskModel = "model task\n" +
	"  id integer primary key\n" +
	"  mode text not null\n" +
	"  x text not null\n" +
	"  y text not null\n" +
	"  z text\n" +
	"  p real\n" +
	"  q real\n" +
	"  r real\n" +
	"  done integer not null\n"

const createORM = `CREATE TABLE IF NOT EXISTS sqlx_orm (
  model TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  position INTEGER NOT NULL
)`

const createTask = `CREATE TABLE IF NOT EXISTS task (
  "id" INTEGER PRIMARY KEY,
  "mode" TEXT NOT NULL,
  "x" TEXT NOT NULL,
  "y" TEXT NOT NULL,
  "z" TEXT,
  "p" REAL,
  "q" REAL,
  "r" REAL,
  "done" INTEGER NOT NULL
)`

type Record struct {
	ID    int
	Event model.Event
}

type taskRow struct {
	ID   int      `json:"id"`
	Mode string   `json:"mode"`
	X    string   `json:"x"`
	Y    string   `json:"y"`
	Z    *string  `json:"z"`
	P    *float64 `json:"p"`
	Q    *float64 `json:"q"`
	R    *float64 `json:"r"`
	Done int      `json:"done"`
}

func DatabasePath() string {
	root := os.Getenv("SQLX_ROOT")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "/tmp"
		}
		root = filepath.Join(home, ".sqlx")
	}
	return filepath.Join(root, "data", DatabaseName+".sqlite")
}

// Open creates the crusade database when it is missing and imports the boxes
// outline once. A later call leaves existing rows in place.
func Open(dbPath string) (bool, error) {
	if err := adoptLegacyDatabase(dbPath); err != nil {
		return false, err
	}
	created := false
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		created = true
	} else if err != nil {
		return false, err
	}
	if err := ensure(dbPath); err != nil {
		if created {
			removeDB(dbPath)
		}
		return false, err
	}
	if err := importBoxesOnce(dbPath); err != nil {
		if created {
			removeDB(dbPath)
		}
		return false, err
	}
	return created, nil
}

func Load(dbPath string) ([]Record, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []taskRow
	if err := queryJSON(dbPath, "SELECT id, mode, x, y, z, p, q, r, done FROM task", &rows); err != nil {
		return nil, fmt.Errorf("failed to read tasks from %s: %w", dbPath, err)
	}
	records := make([]Record, 0, len(rows))
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			return nil, fmt.Errorf("failed to read tasks from %s: task has invalid id", dbPath)
		}
		if seen[row.ID] {
			return nil, fmt.Errorf("failed to read tasks from %s: duplicate task id %d", dbPath, row.ID)
		}
		seen[row.ID] = true
		event, err := eventFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("failed to read tasks from %s: task id %d: %w", dbPath, row.ID, err)
		}
		records = append(records, Record{ID: row.ID, Event: event})
	}
	sortRecords(records)
	return records, nil
}

func Save(dbPath string, records []Record) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	normalized, err := normalizeRecords(records)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\nDELETE FROM task;\n")
	if len(normalized) > 0 {
		b.WriteString("INSERT INTO task(id, mode, x, y, z, p, q, r, done) VALUES\n")
		for index, record := range normalized {
			if index > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(rowSQL(record))
		}
		b.WriteString(";\n")
	}
	b.WriteString("COMMIT;\n")
	return execSQL(dbPath, b.String())
}

func Add(dbPath string, event model.Event) error {
	_, err := AddRecord(dbPath, event)
	return err
}

func AddRecord(dbPath string, event model.Event) (int, error) {
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	id, err := nextID(dbPath)
	if err != nil {
		return 0, err
	}
	record, err := stored(Record{ID: id, Event: event})
	if err != nil {
		return 0, err
	}
	if err := execSQL(dbPath, "INSERT INTO task(id, mode, x, y, z, p, q, r, done) VALUES "+rowSQL(record)+";\n"); err != nil {
		return 0, err
	}
	return id, nil
}

func Update(dbPath string, id int, event model.Event) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	record, err := stored(Record{ID: id, Event: event})
	if err != nil {
		return err
	}
	changed, err := rowsChanged(dbPath, "UPDATE task SET "+rowAssignments(record)+" WHERE id = "+strconv.Itoa(id)+";\n")
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func Delete(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	changed, err := rowsChanged(dbPath, "DELETE FROM task WHERE id = "+strconv.Itoa(id)+";\n")
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func ensure(dbPath string) error {
	if err := prepareDirs(dbPath); err != nil {
		return err
	}
	sql := "PRAGMA journal_mode=WAL;\n" + createORM + ";\n" + createTask + ";\n" + createBox + ";\n" + createBoxCheck + ";\n" + createMeta + ";\n" + ormUpsertSQL()
	if err := execSQL(dbPath, sql); err != nil {
		return err
	}
	if err := ensureTaskDoneColumn(dbPath); err != nil {
		return err
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return err
	}
	return writeORMFile(dbPath)
}

func ensureTaskDoneColumn(dbPath string) error {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := queryJSON(dbPath, "SELECT name FROM pragma_table_info('task') WHERE name = 'done'", &rows); err != nil {
		return err
	}
	if len(rows) > 0 {
		return nil
	}
	return execSQL(dbPath, "ALTER TABLE task ADD COLUMN done INTEGER NOT NULL DEFAULT 0;")
}

func prepareDirs(dbPath string) error {
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return err
	}
	if filepath.Base(dbPath) != DatabaseName+".sqlite" || filepath.Base(dataDir) != "data" {
		return nil
	}
	root := filepath.Dir(dataDir)
	if err := os.Chmod(root, 0o700); err != nil {
		return err
	}
	ormDir := filepath.Join(root, "orm")
	if err := os.MkdirAll(ormDir, 0o700); err != nil {
		return err
	}
	return os.Chmod(ormDir, 0o700)
}

func writeORMFile(dbPath string) error {
	dataDir := filepath.Dir(dbPath)
	if filepath.Base(dbPath) != DatabaseName+".sqlite" || filepath.Base(dataDir) != "data" {
		return nil
	}
	ormDir := filepath.Join(filepath.Dir(dataDir), "orm")
	tmp, err := os.CreateTemp(ormDir, ".orm-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(ormText()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(ormDir, DatabaseName)); err != nil {
		return err
	}
	ok = true
	return nil
}

func removeDB(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}

func normalizeRecords(records []Record) ([]Record, error) {
	normalized := append([]Record(nil), records...)
	seen := make(map[int]bool, len(normalized))
	next := nextSliceID(normalized)
	for index := range normalized {
		record, err := stored(normalized[index])
		if err != nil {
			return nil, err
		}
		if record.ID <= 0 {
			record.ID = next
			next++
		}
		if seen[record.ID] {
			return nil, fmt.Errorf("duplicate task id %d", record.ID)
		}
		seen[record.ID] = true
		normalized[index] = record
	}
	sortRecords(normalized)
	return normalized, nil
}

func stored(record Record) (Record, error) {
	event := record.Event
	var err error
	done := event.Done
	if event.IsDo() {
		event, err = model.NewDoEvent(event.X.Format(model.DateTimeFormat), event.Y)
	} else if event.Mode == "" || event.Mode == model.ModeAlign {
		event, err = model.NewEvent(
			event.X.Format(model.DateTimeFormat),
			event.Y,
			event.Z,
			model.FormatMetric(event.P),
			model.FormatMetric(event.Q),
			model.FormatMetric(event.R),
		)
	} else {
		return Record{}, fmt.Errorf("unknown mode %q", event.Mode)
	}
	if err != nil {
		return Record{}, err
	}
	event.Done = done
	record.Event = event
	return record, nil
}

func nextID(dbPath string) (int, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(dbPath, "SELECT COALESCE(MAX(id), 0) AS n FROM task", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return rows[0].N + 1, nil
}

func nextSliceID(records []Record) int {
	maximum := 0
	for _, record := range records {
		if record.ID > maximum {
			maximum = record.ID
		}
	}
	return maximum + 1
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		if model.Less(records[i].Event, records[j].Event) {
			return true
		}
		if model.Less(records[j].Event, records[i].Event) {
			return false
		}
		return records[i].ID < records[j].ID
	})
}

func eventFromRow(row taskRow) (model.Event, error) {
	var event model.Event
	var err error
	switch strings.ToLower(strings.TrimSpace(row.Mode)) {
	case model.ModeDo:
		event, err = model.NewDoEvent(row.X, row.Y)
	case "", model.ModeAlign, "think":
		z := ""
		if row.Z != nil {
			z = *row.Z
		}
		if strings.TrimSpace(row.Mode) == "" && z == "done" && metricIsZero(row.P) && metricIsZero(row.Q) && metricIsZero(row.R) {
			event, err = model.NewDoEvent(row.X, row.Y)
			break
		}
		event, err = model.NewEvent(row.X, row.Y, z, metricField(row.P), metricField(row.Q), metricField(row.R))
	default:
		return model.Event{}, fmt.Errorf("unknown mode %q", row.Mode)
	}
	if err != nil {
		return model.Event{}, err
	}
	event.Done = row.Done != 0
	return event, nil
}

func metricIsZero(value *float64) bool {
	return value == nil || *value == 0
}

func metricField(value *float64) string {
	if value == nil {
		return ""
	}
	return model.FormatMetric(*value)
}

func rowSQL(record Record) string {
	event := record.Event
	when := sqlString(event.X.Format(model.DateTimeFormat))
	outcome := sqlString(event.Y)
	if event.IsDo() {
		return fmt.Sprintf("(%d, %s, %s, %s, NULL, NULL, NULL, NULL, %d)", record.ID, sqlString(model.ModeDo), when, outcome, doneBit(event.Done))
	}
	return fmt.Sprintf("(%d, %s, %s, %s, %s, %s, %s, %s, %d)",
		record.ID,
		sqlString(model.ModeAlign),
		when,
		outcome,
		sqlString(event.Z),
		strconv.FormatFloat(event.P, 'f', -1, 64),
		strconv.FormatFloat(event.Q, 'f', -1, 64),
		strconv.FormatFloat(event.R, 'f', -1, 64),
		doneBit(event.Done),
	)
}

func rowAssignments(record Record) string {
	event := record.Event
	when := sqlString(event.X.Format(model.DateTimeFormat))
	outcome := sqlString(event.Y)
	if event.IsDo() {
		return "mode = " + sqlString(model.ModeDo) + ", x = " + when + ", y = " + outcome + ", z = NULL, p = NULL, q = NULL, r = NULL, done = " + doneSQL(event.Done)
	}
	return "mode = " + sqlString(model.ModeAlign) +
		", x = " + when +
		", y = " + outcome +
		", z = " + sqlString(event.Z) +
		", p = " + strconv.FormatFloat(event.P, 'f', -1, 64) +
		", q = " + strconv.FormatFloat(event.Q, 'f', -1, 64) +
		", r = " + strconv.FormatFloat(event.R, 'f', -1, 64) +
		", done = " + doneSQL(event.Done)
}

func doneBit(done bool) int {
	if done {
		return 1
	}
	return 0
}

func doneSQL(done bool) string {
	if done {
		return "1"
	}
	return "0"
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func rowsChanged(dbPath, sql string) (bool, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(dbPath, sql+"SELECT changes() AS n;", &rows); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, fmt.Errorf("%s: sqlite returned no changes count", dbPath)
	}
	return rows[0].N > 0, nil
}

func queryJSON(dbPath, sql string, dest any) error {
	raw, err := run(dbPath, sql, "-json")
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("sqlite json: %w", err)
	}
	return nil
}

func execSQL(dbPath, sql string) error {
	_, err := run(dbPath, sql)
	return err
}

func run(dbPath, sql string, flags ...string) (string, error) {
	bin := os.Getenv("SQLX_SQLITE")
	if bin == "" {
		bin = "/usr/bin/sqlite3"
	}
	args := []string{"-batch", "-cmd", ".timeout 5000"}
	args = append(args, flags...)
	args = append(args, dbPath, sql)
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}
