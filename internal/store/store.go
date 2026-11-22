package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/sqlite"
)

const DatabaseName = "crusade"

const legacyDatabaseName = "xecute"

// AlignRecord is one stored align record.
type AlignRecord struct {
	ID    int
	Align model.Align
}

type alignRow struct {
	ID     int     `json:"id"`
	X      string  `json:"x"`
	Y      string  `json:"y"`
	Z      string  `json:"z"`
	DoneAt *string `json:"done_at"`
}

func DatabasePath() string {
	return sqlite.File(DatabaseName)
}

// Open creates the crusade database when it is missing, migrates an older
// one, and runs the one-time boxes and do list imports. It reports whether the
// database was created.
func Open(dbPath string) (bool, error) {
	if err := sqlite.Adopt(filepath.Dir(dbPath), legacyDatabaseName, dbPath); err != nil {
		return false, err
	}
	existed, err := databaseListed(dbPath)
	if err != nil {
		return false, err
	}
	steps := []func(string) error{ensure, importBoxesOnce, importDoOnce}
	for _, step := range steps {
		if err := step(dbPath); err != nil {
			if !existed {
				_ = os.Remove(dbPath)
			}
			return false, err
		}
	}
	return !existed, nil
}

func ensure(dbPath string) error {
	return sqlite.Prepare(dbPath, crusadeSchema())
}

// databaseListed reports that dbPath already holds crusade tables.
func databaseListed(dbPath string) (bool, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(dbPath, "SELECT COUNT(*) AS n FROM sqlite_master WHERE type = 'table' AND name IN ('install', 'task')", &rows); err != nil {
		return false, err
	}
	return len(rows) > 0 && rows[0].N > 0, nil
}

// Load reads the align records in day order.
func Load(dbPath string) ([]AlignRecord, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []alignRow
	if err := sqlite.Query(dbPath, "SELECT id, x, y, z, done_at FROM align", &rows); err != nil {
		return nil, fmt.Errorf("failed to read align records from %s: %w", dbPath, err)
	}
	records := make([]AlignRecord, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("failed to read align records from %s: id %d: %w", dbPath, row.ID, err)
		}
		records = append(records, AlignRecord{ID: row.ID, Align: event})
	}
	sortRecords(records)
	return records, nil
}

func Add(dbPath string, event model.Align) error {
	_, err := AddRecord(dbPath, event)
	return err
}

// AddRecord stores a new align record and returns its id.
func AddRecord(dbPath string, event model.Align) (int, error) {
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	event, err := stored(event)
	if err != nil {
		return 0, err
	}
	var rows []struct {
		ID int `json:"id"`
	}
	sql := fmt.Sprintf("INSERT INTO align(x, y, z, done_at) VALUES (%s) RETURNING id;\n", alignValuesSQL(event))
	if err := sqlite.Query(dbPath, sql, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("%s: sqlite returned no id", dbPath)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// Update replaces the align record id.
func Update(dbPath string, id int, event model.Align) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	event, err := stored(event)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("UPDATE align SET x = %s, y = %s, z = %s, done_at = %s WHERE id = %d;\n",
		sqlite.Quote(event.X.Format(time.DateOnly)), sqlite.Quote(event.Y), sqlite.Quote(event.Z), doneAtSQL(event), id)
	return changeOne(dbPath, sql, id)
}

// Delete removes the align record id.
func Delete(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return changeOne(dbPath, "DELETE FROM align WHERE id = "+strconv.Itoa(id)+";\n", id)
}

func changeOne(dbPath, sql string, id int) error {
	changed, err := rowsChanged(dbPath, sql)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

// DeletedThisWeek counts do items deleted during the Monday-Sunday week that contains now.
func DeletedThisWeek(dbPath string, now time.Time) (int, error) {
	if strings.TrimSpace(dbPath) == "" {
		return 0, nil
	}
	exists, err := databaseListed(dbPath)
	if err != nil || !exists {
		return 0, err
	}
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	var rows []struct {
		DeletedAt string `json:"deleted_at"`
	}
	if err := sqlite.Query(dbPath, "SELECT deleted_at FROM deletion", &rows); err != nil {
		return 0, err
	}
	stamps := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		stamp, err := parseInstant(row.DeletedAt)
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

// LastMode is the mode the next launch opens.
func LastMode(dbPath string) (string, error) {
	if err := ensure(dbPath); err != nil {
		return "", err
	}
	var rows []struct {
		Mode string `json:"last_mode"`
	}
	if err := sqlite.Query(dbPath, "SELECT last_mode FROM install WHERE id = 1", &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return model.ModeDo, nil
	}
	return rows[0].Mode, nil
}

// SaveLastMode stores the mode the next launch opens.
func SaveLastMode(dbPath, mode string) error {
	if !knownMode(mode) {
		return fmt.Errorf("unknown mode %s", mode)
	}
	if err := ensure(dbPath); err != nil {
		return err
	}
	return execSQL(dbPath, "UPDATE install SET last_mode = "+sqlite.Quote(mode)+" WHERE id = 1;\n")
}

func knownMode(mode string) bool {
	switch mode {
	case model.ModeDo, model.ModeAlign, model.ModeBox, model.ModeCal, model.ModeNLP:
		return true
	default:
		return false
	}
}

func stored(event model.Align) (model.Align, error) {
	next, err := model.NewAlign(event.X.Format(time.DateOnly), event.Y, event.Z)
	if err != nil {
		return model.Align{}, err
	}
	next.DoneAt = event.DoneAt
	return next, nil
}

func sortRecords(records []AlignRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if model.Less(records[i].Align, records[j].Align) {
			return true
		}
		if model.Less(records[j].Align, records[i].Align) {
			return false
		}
		return records[i].ID < records[j].ID
	})
}

func eventFromRow(row alignRow) (model.Align, error) {
	event, err := model.NewAlign(row.X, row.Y, row.Z)
	if err != nil {
		return model.Align{}, err
	}
	if row.DoneAt != nil {
		stamp, err := parseInstant(*row.DoneAt)
		if err != nil {
			return model.Align{}, err
		}
		event.DoneAt = stamp
	}
	return event, nil
}

func alignValuesSQL(event model.Align) string {
	return strings.Join([]string{sqlite.Quote(event.X.Format(time.DateOnly)), sqlite.Quote(event.Y), sqlite.Quote(event.Z), doneAtSQL(event)}, ", ")
}

func doneAtSQL(event model.Align) string {
	if !event.Done() {
		return "NULL"
	}
	return instantSQL(event.DoneAt)
}

// instantSQL stores a moment in UTC. Calendar clocks are not instants: they
// stay local wall time.
func instantSQL(at time.Time) string {
	return sqlite.Quote(at.UTC().Format(model.DateTimeFormat))
}

func parseInstant(raw string) (time.Time, error) {
	at, err := time.ParseInLocation(model.DateTimeFormat, raw, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return at.Local(), nil
}

func plainURL(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func storedURL(value string) string {
	return sqlite.Text(strings.TrimSpace(value))
}

func rowsChanged(dbPath, sql string) (bool, error) {
	changed, err := sqlite.Changed(dbPath, sql)
	if err == nil && changed {
		published(dbPath)
	}
	return changed, err
}

// execSQL writes the app sqlite and backs it up.
func execSQL(dbPath, sql string) error {
	if err := sqlite.Exec(dbPath, sql); err != nil {
		return err
	}
	published(dbPath)
	return nil
}

// published backs the file up unless it holds no records. An empty file is
// the damaged local copy, so it is not published.
func published(dbPath string) {
	if os.Getenv("SQLX_ROOT") != "" {
		return
	}
	var rows []struct {
		N int `json:"n"`
	}
	sql := "SELECT (SELECT COUNT(*) FROM align) + (SELECT COUNT(*) FROM do_item) + (SELECT COUNT(*) FROM box) + (SELECT COUNT(*) FROM block) + (SELECT COUNT(*) FROM repeat) AS n"
	if err := sqlite.Query(dbPath, sql, &rows); err != nil || len(rows) == 0 || rows[0].N == 0 {
		return
	}
	sqlite.Publish(dbPath, DatabaseName)
}
