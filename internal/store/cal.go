package store

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/model"
)

const blockModel = "model block\n" +
	"  id integer primary key\n" +
	"  title text not null\n" +
	"  start_at text not null\n" +
	"  end_at text not null\n" +
	"  url text\n"

const createBlock = `CREATE TABLE IF NOT EXISTS block (
  "id" INTEGER PRIMARY KEY,
  "title" TEXT NOT NULL,
  "start_at" TEXT NOT NULL,
  "end_at" TEXT NOT NULL,
  "url" TEXT
)`

// BlockRecord is one span of clock time. It is not a do task.
type BlockRecord struct {
	ID    int
	Block model.Block
}

type blockRow struct {
	ID      int     `json:"id"`
	Title   string  `json:"title"`
	StartAt string  `json:"start_at"`
	EndAt   string  `json:"end_at"`
	URL     *string `json:"url"`
}

func LoadBlocks(dbPath string) ([]BlockRecord, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []blockRow
	if err := queryJSON(dbPath, "SELECT id, title, start_at, end_at, url FROM block", &rows); err != nil {
		return nil, fmt.Errorf("failed to read blocks from %s: %w", dbPath, err)
	}
	blocks := make([]BlockRecord, 0, len(rows))
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			return nil, fmt.Errorf("failed to read blocks from %s: block has invalid id", dbPath)
		}
		if seen[row.ID] {
			return nil, fmt.Errorf("failed to read blocks from %s: duplicate block id %d", dbPath, row.ID)
		}
		seen[row.ID] = true
		block, err := model.NewBlock(row.Title, row.StartAt, row.EndAt)
		if err != nil {
			return nil, fmt.Errorf("failed to read blocks from %s: block id %d: %w", dbPath, row.ID, err)
		}
		block.URL = plainURL(row.URL)
		blocks = append(blocks, BlockRecord{ID: row.ID, Block: block})
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Block.Start.Equal(blocks[j].Block.Start) {
			return blocks[i].ID < blocks[j].ID
		}
		return blocks[i].Block.Start.Before(blocks[j].Block.Start)
	})
	return blocks, nil
}

func AddBlock(dbPath string, block model.Block) (int, error) {
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	block, err := normalizeBlock(block)
	if err != nil {
		return 0, err
	}
	if err := blockConflicts(dbPath, 0, block); err != nil {
		return 0, err
	}
	id, err := nextBlockID(dbPath)
	if err != nil {
		return 0, err
	}
	if err := execSQL(dbPath, blockInsertSQL(id, block)); err != nil {
		return 0, err
	}
	return id, nil
}

func UpdateBlock(dbPath string, id int, block model.Block) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	block, err := normalizeBlock(block)
	if err != nil {
		return err
	}
	if err := blockConflicts(dbPath, id, block); err != nil {
		return err
	}
	changed, err := rowsChanged(dbPath, blockUpdateSQL(id, block))
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func DeleteBlock(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	changed, err := rowsChanged(dbPath, "DELETE FROM block WHERE id = "+fmt.Sprintf("%d", id)+";\n")
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func normalizeBlock(block model.Block) (model.Block, error) {
	next, err := model.NewBlock(block.Title, block.Start.Format(model.DateTimeFormat), block.End.Format(model.DateTimeFormat))
	if err != nil {
		return model.Block{}, err
	}
	if err := next.OnTheHour(); err != nil {
		return model.Block{}, err
	}
	next.URL = strings.TrimSpace(block.URL)
	return next, nil
}

func nextBlockID(dbPath string) (int, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(dbPath, "SELECT COALESCE(MAX(id), 0) AS n FROM block", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return rows[0].N + 1, nil
}

func blockInsertSQL(id int, block model.Block) string {
	return fmt.Sprintf("INSERT INTO block(id, title, start_at, end_at, url) VALUES (%d, %s, %s, %s, %s);\n",
		id,
		sqlString(block.Title),
		sqlString(block.Start.Format(model.DateTimeFormat)),
		sqlString(block.End.Format(model.DateTimeFormat)),
		storedURL(block.URL),
	)
}

func blockUpdateSQL(id int, block model.Block) string {
	return fmt.Sprintf("UPDATE block SET title = %s, start_at = %s, end_at = %s, url = %s WHERE id = %d;\n",
		sqlString(block.Title),
		sqlString(block.Start.Format(model.DateTimeFormat)),
		sqlString(block.End.Format(model.DateTimeFormat)),
		storedURL(block.URL),
		id,
	)
}
