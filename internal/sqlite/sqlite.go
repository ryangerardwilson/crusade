// Package sqlite runs SQL against an app's sqlite file through the sqlite3
// command and brings that file to the app's schema version. The crusade store
// and the nlp state both use it, so a change to sqlite access is one file.
//
// The application owns each sqlite file. Nebula sql is the Postgres interface
// and is not called. Nebula keep owns the backup at s3://backups/<name>.sqlite:
// the app says the file changed after a commit and asks for a missing file at
// startup; the upload, the snapshot, and the retries live in nebula.
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Schema is an app's tables at one version. Tables creates them in an empty
// file. Migrate returns the statements that move an older file to Version; they
// run in one transaction with the version bump, so a failed statement leaves
// the file as it was.
type Schema struct {
	Name    string
	Version int
	Tables  string
	Migrate func(path string) (string, error)
}

// File is where the app keeps its sqlite file.
func File(name string) string {
	if root := os.Getenv("SQLX_ROOT"); root != "" {
		return filepath.Join(root, "data", name+".sqlite")
	}
	if v := os.Getenv("SQLX_RUNTIME"); v != "" {
		return filepath.Join(v, name+".sqlite")
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join("/dev/shm", fmt.Sprintf("%s-%d", name, os.Getuid()))
	} else {
		base = filepath.Join(base, name)
	}
	return filepath.Join(base, name+".sqlite")
}

// Prepare creates the file when it is missing, restoring a backup first, and
// brings it to schema.Version. A backup that cannot be reached fails Prepare
// and leaves no file, so an empty file never stands in for the backup.
func Prepare(path string, schema Schema) error {
	if err := prepareDirs(path); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = copyLegacyRuntime(path, schema.Name)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := restore(path, schema.Name); err != nil {
			return err
		}
	}
	if err := migrate(path, schema); err != nil {
		return err
	}
	if err := writeSchemaFile(schema); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func prepareDirs(path string) error {
	if root := os.Getenv("SQLX_ROOT"); root != "" {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(root, 0o700); err != nil {
			return err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// migrate runs the schema's tables on an empty file and its migration on an
// older one. user_version is the schema version the file holds.
func migrate(path string, schema Schema) error {
	version, err := UserVersion(path)
	if err != nil {
		return err
	}
	if version == schema.Version {
		return nil
	}
	if version > schema.Version {
		return fmt.Errorf("%s is schema version %d and this crusade reads version %d", path, version, schema.Version)
	}
	empty, err := isEmpty(path)
	if err != nil {
		return err
	}
	body := schema.Tables
	if !empty {
		if body, err = schema.Migrate(path); err != nil {
			return err
		}
	}
	bump := fmt.Sprintf("PRAGMA user_version = %d;\n", schema.Version)
	err = Exec(path, "BEGIN IMMEDIATE;\nPRAGMA defer_foreign_keys = ON;\n"+body+bump+"COMMIT;\n")
	if err == nil {
		return nil
	}
	// Another process may have migrated the file first.
	if now, checkErr := UserVersion(path); checkErr == nil && now == schema.Version {
		return nil
	}
	return fmt.Errorf("migrate %s to schema version %d: %w", path, schema.Version, err)
}

// UserVersion is the schema version stored in the file header.
func UserVersion(path string) (int, error) {
	var rows []struct {
		N int `json:"user_version"`
	}
	if err := Query(path, "PRAGMA user_version", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

func isEmpty(path string) (bool, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := Query(path, "SELECT COUNT(*) AS n FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'", &rows); err != nil {
		return false, err
	}
	return len(rows) == 0 || rows[0].N == 0, nil
}

// Columns names the columns of table. A missing table has none.
func Columns(path, table string) (map[string]bool, error) {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := Query(path, "PRAGMA table_info("+Ident(table)+")", &rows); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range rows {
		out[row.Name] = true
	}
	return out, nil
}

func writeSchemaFile(schema Schema) error {
	root := os.Getenv("SQLX_ROOT")
	if root == "" {
		return nil
	}
	dir := filepath.Join(root, "orm")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, schema.Name), []byte(schema.Tables), 0o600)
}

func copyLegacyRuntime(path, name string) error {
	if os.Getenv("SQLX_ROOT") != "" {
		return nil
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return nil
	}
	src := filepath.Join(base, "sqlx", name+".sqlite")
	if src == path {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil
	}
	return os.WriteFile(path, data, 0o600)
}

const (
	markWait    = 5 * time.Second  // local only: a record and a lock
	restoreWait = 90 * time.Second // one download over SSH
)

// restore makes the missing file. nebula keep restores the backup, or makes
// an empty file when there is none, and fails without a file when the backup
// cannot be reached. The SQLX_ROOT tree has no backup and starts empty.
func restore(path, name string) error {
	if !kept() {
		return os.WriteFile(path, nil, 0o600)
	}
	if err := runKeep(restoreWait, "restore", name, path); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("nebula keep restore %s: no file", name)
	}
	return nil
}

// Mark tells nebula keep the file changed, after a commit. It returns once
// keep has recorded the change; keep uploads a snapshot later in its own
// process. The caller decides that the file holds data worth keeping. A failed
// mark leaves the commit in place; keep's sweep and status show the copy
// lagging.
func Mark(path, name string) {
	if !kept() || path == "" {
		return
	}
	_ = runKeep(markWait, "mark", name, path)
}

// kept reports that the file has a backup. The SQLX_ROOT tree is local. A
// test binary reaches only the nebula NEBULA_BIN names, never the operator's.
func kept() bool {
	if os.Getenv("SQLX_ROOT") != "" {
		return false
	}
	return !testing.Testing() || os.Getenv("NEBULA_BIN") != ""
}

func runKeep(wait time.Duration, verb, name, path string) error {
	bin, err := nebulaBin()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "keep", verb, name, path).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("nebula keep %s %s: %s", verb, name, msg)
}

func nebulaBin() (string, error) {
	if bin := os.Getenv("NEBULA_BIN"); bin != "" {
		return bin, nil
	}
	bin, err := exec.LookPath("nebula")
	if err != nil {
		return "", errors.New("nebula is not on PATH")
	}
	return bin, nil
}

// Adopt renames the legacy file in dir to destPath when destPath is missing.
func Adopt(dir, legacy, destPath string) error {
	oldPath := filepath.Join(dir, legacy+".sqlite")
	if _, err := os.Stat(oldPath); err != nil {
		return nil
	}
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Rename(oldPath, destPath)
}

// Query runs sql and decodes its JSON rows into dest.
func Query(path, sql string, dest any) error {
	raw, err := run(path, sql, true)
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

// Exec runs sql and discards its output.
func Exec(path, sql string) error {
	_, err := run(path, sql, false)
	return err
}

// Changed runs sql and reports whether its last statement changed a row.
func Changed(path, sql string) (bool, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := Query(path, sql+"SELECT changes() AS n;", &rows); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, fmt.Errorf("%s: sqlite returned no changes count", path)
	}
	return rows[0].N > 0, nil
}

// Tx wraps statements in one transaction. run bails at the first failed
// statement, so nothing before it commits.
func Tx(statements ...string) string {
	return "BEGIN IMMEDIATE;\n" + strings.Join(statements, "") + "COMMIT;\n"
}

func run(path, sql string, asJSON bool) (string, error) {
	bin := os.Getenv("SQLX_SQLITE")
	if bin == "" {
		bin = "/usr/bin/sqlite3"
	}
	// -bail stops at the first failed statement, so a script wrapped in
	// BEGIN and COMMIT never commits the statements before the failure.
	args := []string{"-batch", "-bail", "-cmd", ".timeout 5000"}
	if asJSON {
		args = append(args, "-json")
	}
	args = append(args, path)
	cmd := exec.Command(bin, args...)
	// Every connection enforces the schema's foreign keys. The pragma runs
	// first because it is ignored inside a transaction.
	cmd.Stdin = strings.NewReader("PRAGMA foreign_keys = ON;\n" + sql)
	var stdout, stderr strings.Builder
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

// Ident quotes a table or column name.
func Ident(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Quote is value as a sql string literal.
func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// Text is value as a sql string literal, or NULL when it is empty.
func Text(value string) string {
	if value == "" {
		return "NULL"
	}
	return Quote(value)
}
