package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
	"  done integer not null default 0\n" +
	"  done_at text\n"

const deletionModel = "model deletion\n" +
	"  id integer primary key\n" +
	"  deleted_at text not null\n"

const dropTaskScoresSQL = `
BEGIN IMMEDIATE;
CREATE TABLE task_scoreless (
  "id" INTEGER PRIMARY KEY,
  "mode" TEXT NOT NULL,
  "x" TEXT NOT NULL,
  "y" TEXT NOT NULL,
  "z" TEXT,
  "done" INTEGER NOT NULL,
  "done_at" TEXT
);
INSERT INTO task_scoreless(id, mode, x, y, z, done, done_at)
SELECT
  id,
  CASE
    WHEN TRIM(COALESCE(mode, '')) = '' AND z = 'done' AND COALESCE(p, 0) = 0 AND COALESCE(q, 0) = 0 AND COALESCE(r, 0) = 0 THEN 'do'
    WHEN LOWER(TRIM(mode)) = 'think' THEN 'align'
    WHEN TRIM(COALESCE(mode, '')) = '' THEN 'align'
    ELSE mode
  END,
  CASE
    WHEN NOT (TRIM(COALESCE(mode, '')) = '' AND z = 'done' AND COALESCE(p, 0) = 0 AND COALESCE(q, 0) = 0 AND COALESCE(r, 0) = 0)
      AND LOWER(TRIM(COALESCE(mode, ''))) IN ('', 'align', 'think')
    THEN substr(x, 1, 10)
    ELSE x
  END,
  y,
  CASE
    WHEN TRIM(COALESCE(mode, '')) = '' AND z = 'done' AND COALESCE(p, 0) = 0 AND COALESCE(q, 0) = 0 AND COALESCE(r, 0) = 0 THEN NULL
    ELSE z
  END,
  COALESCE(done, 0),
  done_at
FROM task;
DROP TABLE task;
ALTER TABLE task_scoreless RENAME TO task;
COMMIT;
`

type Record struct {
	ID    int
	Event model.Event
}

type taskRow struct {
	ID     int     `json:"id"`
	Mode   string  `json:"mode"`
	X      string  `json:"x"`
	Y      string  `json:"y"`
	Z      *string `json:"z"`
	Done   int     `json:"done"`
	DoneAt *string `json:"done_at"`
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
	existed, err := databaseListed(DatabaseName)
	if err != nil {
		return false, err
	}
	if err := ensure(dbPath); err != nil {
		if !existed {
			_, _ = sqlxCall("drop", "database", DatabaseName)
		}
		return false, err
	}
	if err := importBoxesOnce(dbPath); err != nil {
		if !existed {
			_, _ = sqlxCall("drop", "database", DatabaseName)
		}
		return false, err
	}
	return !existed, nil
}

func Load(dbPath string) ([]Record, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []taskRow
	if err := queryJSON(dbPath, "SELECT id, mode, x, y, z, done, done_at FROM task", &rows); err != nil {
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
		b.WriteString("INSERT INTO task(id, mode, x, y, z, done, done_at) VALUES\n")
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
	if err := execSQL(dbPath, "INSERT INTO task(id, mode, x, y, z, done, done_at) VALUES "+rowSQL(record)+";\n"); err != nil {
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
	record, found, err := taskByID(dbPath, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("ID %d not found", id)
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\n")
	if record.Event.IsDo() {
		fmt.Fprintf(&b, "INSERT INTO deletion(id, deleted_at) SELECT COALESCE((SELECT MAX(id) FROM deletion), 0) + 1, %s WHERE EXISTS (SELECT 1 FROM task WHERE id = %d);\n", sqlString(time.Now().Format(model.DateTimeFormat)), id)
	}
	fmt.Fprintf(&b, "DELETE FROM task WHERE id = %d;\nCOMMIT;\n", id)
	changed, err := rowsChanged(dbPath, b.String())
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func taskByID(dbPath string, id int) (Record, bool, error) {
	var rows []taskRow
	query := "SELECT id, mode, x, y, z, done, done_at FROM task WHERE id = " + strconv.Itoa(id)
	if err := queryJSON(dbPath, query, &rows); err != nil {
		return Record{}, false, err
	}
	if len(rows) == 0 {
		return Record{}, false, nil
	}
	event, err := eventFromRow(rows[0])
	if err != nil {
		return Record{}, false, err
	}
	return Record{ID: rows[0].ID, Event: event}, true, nil
}

// DeletedThisWeek counts do tasks deleted during the Monday-Sunday week that contains now.
func DeletedThisWeek(dbPath string, now time.Time) (int, error) {
	if strings.TrimSpace(dbPath) == "" {
		return 0, nil
	}
	exists, err := databaseListed(DatabaseName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	var rows []struct {
		DeletedAt string `json:"deleted_at"`
	}
	if err := queryJSON(dbPath, "SELECT deleted_at FROM deletion", &rows); err != nil {
		return 0, err
	}
	stamps := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		stamp, err := model.ParseDateTime(row.DeletedAt)
		if err != nil {
			continue
		}
		stamps = append(stamps, stamp)
	}
	return deletionsDuringWeek(stamps, now), nil
}

func deletionsDuringWeek(stamps []time.Time, now time.Time) int {
	start := weekStart(now)
	end := start.AddDate(0, 0, 7)
	count := 0
	for _, stamp := range stamps {
		if stamp.IsZero() {
			continue
		}
		day := time.Date(stamp.Year(), stamp.Month(), stamp.Day(), 0, 0, 0, 0, stamp.Location())
		if !day.Before(start) && day.Before(end) {
			count++
		}
	}
	return count
}

func weekStart(now time.Time) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func ensure(dbPath string) error {
	_ = dbPath
	if _, err := sqlxCall("create", "database", DatabaseName); err != nil {
		return err
	}
	if err := migrateLegacyTask(); err != nil {
		return err
	}
	return sqlxMigrate(ormText())
}

func migrateLegacyTask() error {
	raw, err := sqlxCall(DatabaseName, "columns", "task")
	if err != nil {
		return err
	}
	cols := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cols[line] = true
		}
	}
	if !cols["p"] {
		return nil
	}
	if !cols["done"] {
		if _, err := sqlxCall(DatabaseName, "once", "task_done", "ALTER TABLE task ADD COLUMN done INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if !cols["done_at"] {
		if _, err := sqlxCall(DatabaseName, "once", "task_done_at", "ALTER TABLE task ADD COLUMN done_at TEXT"); err != nil {
			return err
		}
	}
	_, err = sqlxCall(DatabaseName, "once", "task_drop_scores", dropTaskScoresSQL)
	return err
}

func databaseListed(name string) (bool, error) {
	// `sql list databases` asks Postgres over SSH even when this
	// database's memory copy is already on disk. Column lookup reads
	// that copy, including a file that still has no orm.
	_, err := sqlxCall(name, "columns", "task")
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "does not exist") {
		return false, nil
	}
	return false, err
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
	doneAt := event.DoneAt
	if event.IsDo() {
		event, err = model.NewDoEvent(event.X.Format(model.DateTimeFormat), event.Y)
	} else if event.Mode == "" || event.Mode == model.ModeAlign {
		event, err = model.NewEvent(event.X.Format("2006-01-02"), event.Y, event.Z)
	} else {
		return Record{}, fmt.Errorf("unknown mode %q", event.Mode)
	}
	if err != nil {
		return Record{}, err
	}
	event.Done = done
	if done {
		event.DoneAt = doneAt
	}
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
		if strings.TrimSpace(row.Mode) == "" && z == "done" {
			event, err = model.NewDoEvent(row.X, row.Y)
			break
		}
		event, err = model.NewEvent(row.X, row.Y, z)
	default:
		return model.Event{}, fmt.Errorf("unknown mode %q", row.Mode)
	}
	if err != nil {
		return model.Event{}, err
	}
	event.Done = row.Done != 0
	if event.Done && row.DoneAt != nil && strings.TrimSpace(*row.DoneAt) != "" {
		stamp, stampErr := model.ParseDateTime(*row.DoneAt)
		if stampErr != nil {
			return model.Event{}, stampErr
		}
		event.DoneAt = stamp
	}
	return event, nil
}

func rowSQL(record Record) string {
	event := record.Event
	outcome := sqlString(event.Y)
	if event.IsDo() {
		when := sqlString(event.X.Format(model.DateTimeFormat))
		return fmt.Sprintf("(%d, %s, %s, %s, NULL, %d, %s)", record.ID, sqlString(model.ModeDo), when, outcome, doneBit(event.Done), doneAtSQL(event))
	}
	when := sqlString(event.X.Format("2006-01-02"))
	return fmt.Sprintf("(%d, %s, %s, %s, %s, %d, %s)",
		record.ID,
		sqlString(model.ModeAlign),
		when,
		outcome,
		sqlString(event.Z),
		doneBit(event.Done),
		doneAtSQL(event),
	)
}

func rowAssignments(record Record) string {
	event := record.Event
	outcome := sqlString(event.Y)
	if event.IsDo() {
		when := sqlString(event.X.Format(model.DateTimeFormat))
		return "mode = " + sqlString(model.ModeDo) + ", x = " + when + ", y = " + outcome + ", z = NULL, done = " + doneSQL(event.Done) + ", done_at = " + doneAtSQL(event)
	}
	when := sqlString(event.X.Format("2006-01-02"))
	return "mode = " + sqlString(model.ModeAlign) +
		", x = " + when +
		", y = " + outcome +
		", z = " + sqlString(event.Z) +
		", done = " + doneSQL(event.Done) +
		", done_at = " + doneAtSQL(event)
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

func doneAtSQL(event model.Event) string {
	if !event.Done || event.DoneAt.IsZero() {
		return "NULL"
	}
	return sqlString(event.DoneAt.Format(model.DateTimeFormat))
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func plainURL(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func storedURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "NULL"
	}
	return sqlString(value)
}

func storedChangeURL(change RepeatChange) string {
	if !change.HasURL {
		return "NULL"
	}
	return sqlString(strings.TrimSpace(change.URL))
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
	_ = dbPath
	raw, err := sqlxCall(DatabaseName, "query", "--json", sql)
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
	_ = dbPath
	file, err := os.CreateTemp("", DatabaseName+"-sql-*")
	if err != nil {
		return err
	}
	sqlPath := file.Name()
	defer os.Remove(sqlPath)
	if _, err := file.WriteString(sql); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = sqlxCall(DatabaseName, "exec", "--file", sqlPath)
	return err
}

func sqlxBin() string {
	if bin := os.Getenv("SQLX_BIN"); bin != "" {
		return bin
	}
	path, err := exec.LookPath("sql")
	if err != nil {
		return "sql"
	}
	return path
}

func sqlxCall(args ...string) (string, error) {
	cmd := exec.Command(sqlxBin(), args...)
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

func sqlxMigrate(body string) error {
	file, err := os.CreateTemp("", DatabaseName+"-orm-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = sqlxCall(DatabaseName, "migrate", name)
	return err
}
