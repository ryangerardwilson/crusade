package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/model"
)

type doItemRow struct {
	ID   int    `json:"id"`
	Kind string `json:"kind"`
	Body string `json:"body"`
}

// LoadDoList reads the do list in order.
func LoadDoList(dbPath string) ([]model.DoItem, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []doItemRow
	if err := sqlite.Query(dbPath, "SELECT id, kind, body FROM do_item ORDER BY position, id", &rows); err != nil {
		return nil, fmt.Errorf("failed to read the do list from %s: %w", dbPath, err)
	}
	list := make([]model.DoItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, model.DoItem{ID: row.ID, Kind: row.Kind, Body: row.Body})
	}
	return list, nil
}

// AddDoItem puts a new item after the row afterID, or at the end of the list
// when afterID is zero, and returns its id.
func AddDoItem(dbPath string, afterID int, text string) (int, error) {
	item, err := model.NewDoItem(text)
	if err != nil {
		return 0, err
	}
	if afterID > 0 {
		if _, err := doItemByID(dbPath, afterID); err != nil {
			return 0, err
		}
	} else if err := ensure(dbPath); err != nil {
		return 0, err
	}
	position := "(SELECT COALESCE(MAX(position), -1) + 1 FROM do_item)"
	shift := ""
	if afterID > 0 {
		after := fmt.Sprintf("(SELECT position FROM do_item WHERE id = %d)", afterID)
		shift = fmt.Sprintf("UPDATE do_item SET position = position + 1 WHERE position > %s;\n", after)
		position = after + " + 1"
	}
	sql := "BEGIN IMMEDIATE;\n" + shift +
		fmt.Sprintf("INSERT INTO do_item(position, kind, body) SELECT %s, 'item', %s RETURNING id;\n", position, sqlite.Quote(item.Body)) +
		"COMMIT;\n"
	var rows []struct {
		ID int `json:"id"`
	}
	if err := sqlite.Query(dbPath, sql, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 || rows[0].ID == 0 {
		return 0, fmt.Errorf("ID %d not found", afterID)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// UpdateDoItem gives the row id new text and keeps its id and place.
func UpdateDoItem(dbPath string, id int, text string) error {
	current, err := doItemByID(dbPath, id)
	if err != nil {
		return err
	}
	next, err := current.WithBody(text)
	if err != nil {
		return err
	}
	return changeOne(dbPath, fmt.Sprintf("UPDATE do_item SET body = %s WHERE id = %d;\n", sqlite.Quote(next.Body), id), id)
}

// DeleteDoItem removes the row id. A deleted item counts toward the week.
func DeleteDoItem(dbPath string, id int) error {
	if _, err := doItemByID(dbPath, id); err != nil {
		return err
	}
	return changeOne(dbPath, "BEGIN IMMEDIATE;\n"+deleteDoItemsSQL([]int{id}, time.Now())+"COMMIT;\n", id)
}

// ReplaceDoList stores the list Vim saved in one transaction. A line left as
// it was, or edited in place, keeps its id.
func ReplaceDoList(dbPath string, rows []model.DoItem) error {
	for _, row := range rows {
		if err := row.Validate(); err != nil {
			return err
		}
	}
	current, err := LoadDoList(dbPath)
	if err != nil {
		return err
	}
	next := model.MatchDoItems(current, rows)
	kept := map[int]bool{}
	for _, row := range next {
		kept[row.ID] = true
	}
	var dropped []int
	for _, row := range current {
		if !kept[row.ID] {
			dropped = append(dropped, row.ID)
		}
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\n")
	b.WriteString(deleteDoItemsSQL(dropped, time.Now()))
	for position, row := range next {
		if row.ID == 0 {
			fmt.Fprintf(&b, "INSERT INTO do_item(position, kind, body) VALUES (%d, %s, %s);\n", position, sqlite.Quote(row.Kind), sqlite.Quote(row.Body))
			continue
		}
		fmt.Fprintf(&b, "UPDATE do_item SET position = %d, kind = %s, body = %s WHERE id = %d;\n", position, sqlite.Quote(row.Kind), sqlite.Quote(row.Body), row.ID)
	}
	b.WriteString("COMMIT;\n")
	return execSQL(dbPath, b.String())
}

// deleteDoItemsSQL removes rows and records each removed item as a deletion.
// Every removal of a do row goes through it.
func deleteDoItemsSQL(ids []int, now time.Time) string {
	if len(ids) == 0 {
		return ""
	}
	list := make([]string, len(ids))
	for index, id := range ids {
		list[index] = strconv.Itoa(id)
	}
	in := strings.Join(list, ", ")
	return fmt.Sprintf("INSERT INTO deletion(deleted_at) SELECT %s FROM do_item WHERE id IN (%s) AND kind = 'item';\n", instantSQL(now), in) +
		fmt.Sprintf("DELETE FROM do_item WHERE id IN (%s);\n", in)
}

func doItemByID(dbPath string, id int) (model.DoItem, error) {
	if err := ensure(dbPath); err != nil {
		return model.DoItem{}, err
	}
	var rows []doItemRow
	if err := sqlite.Query(dbPath, "SELECT id, kind, body FROM do_item WHERE id = "+strconv.Itoa(id), &rows); err != nil {
		return model.DoItem{}, err
	}
	if len(rows) == 0 {
		return model.DoItem{}, fmt.Errorf("ID %d not found", id)
	}
	return model.DoItem{ID: rows[0].ID, Kind: rows[0].Kind, Body: rows[0].Body}, nil
}

// importDoOnce reads the do file written before the list lived in do_item,
// once. Its lines replace the items the migration copied from the old task
// rows. With no file those items stay and no file is written.
func importDoOnce(dbPath string) error {
	if dbPath != DatabasePath() {
		return nil
	}
	var flags []struct {
		Done int `json:"do_imported"`
	}
	if err := sqlite.Query(dbPath, "SELECT do_imported FROM install WHERE id = 1", &flags); err != nil {
		return err
	}
	if len(flags) == 0 || flags[0].Done == 1 {
		return nil
	}
	data, err := os.ReadFile(config.DoPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\n")
	if err == nil {
		b.WriteString("DELETE FROM do_item;\n")
		for position, row := range model.ParseDoList(string(data)) {
			fmt.Fprintf(&b, "INSERT INTO do_item(position, kind, body) VALUES (%d, %s, %s);\n", position, sqlite.Quote(row.Kind), sqlite.Quote(row.Body))
		}
	}
	b.WriteString("UPDATE install SET do_imported = 1 WHERE id = 1;\nCOMMIT;\n")
	return execSQL(dbPath, b.String())
}
