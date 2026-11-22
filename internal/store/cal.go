package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"fmt"
	"sort"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/model"
)

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
	if err := sqlite.Query(dbPath, "SELECT id, title, start_at, end_at, url FROM block", &rows); err != nil {
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

// AddBlock stores a new block on the hour.
func AddBlock(dbPath string, block model.Block) (int, error) {
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	block, err := normalizeBlock(block)
	if err != nil {
		return 0, err
	}
	if err := block.OnTheHour(); err != nil {
		return 0, err
	}
	if err := blockConflicts(dbPath, 0, block); err != nil {
		return 0, err
	}
	var rows []struct {
		ID int `json:"id"`
	}
	if err := sqlite.Query(dbPath, blockInsertSQL(block), &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("%s: sqlite returned no id", dbPath)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// updateBlock writes the block a form save decided on. model.EditBlock has
// already applied the on-the-hour rule.
func updateBlock(dbPath string, id int, block model.Block) error {
	block, err := normalizeBlock(block)
	if err != nil {
		return err
	}
	if err := blockConflicts(dbPath, id, block); err != nil {
		return err
	}
	return changeOne(dbPath, blockUpdateSQL(id, block), id)
}

func DeleteBlock(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return changeOne(dbPath, fmt.Sprintf("DELETE FROM block WHERE id = %d;\n", id), id)
}

func requireBlock(dbPath string, id int) (model.Block, error) {
	blocks, err := LoadBlocks(dbPath)
	if err != nil {
		return model.Block{}, err
	}
	for _, block := range blocks {
		if block.ID == id {
			return block.Block, nil
		}
	}
	return model.Block{}, fmt.Errorf("ID %d not found", id)
}

func normalizeBlock(block model.Block) (model.Block, error) {
	next, err := model.NewBlock(block.Title, block.Start.Format(model.DateTimeFormat), block.End.Format(model.DateTimeFormat))
	if err != nil {
		return model.Block{}, err
	}
	next.URL = strings.TrimSpace(block.URL)
	return next, nil
}

func blockConflicts(dbPath string, id int, block model.Block) error {
	probe := span{start: block.Start, end: block.End}
	return spanConflicts(dbPath, []span{probe}, func(existing span) bool {
		return id != 0 && existing.blockID == id
	})
}

func blockInsertSQL(block model.Block) string {
	return fmt.Sprintf("INSERT INTO block(title, start_at, end_at, url) VALUES (%s, %s, %s, %s) RETURNING id;\n",
		sqlite.Quote(block.Title),
		sqlite.Quote(block.Start.Format(model.DateTimeFormat)),
		sqlite.Quote(block.End.Format(model.DateTimeFormat)),
		storedURL(block.URL),
	)
}

func blockUpdateSQL(id int, block model.Block) string {
	return fmt.Sprintf("UPDATE block SET title = %s, start_at = %s, end_at = %s, url = %s WHERE id = %d;\n",
		sqlite.Quote(block.Title),
		sqlite.Quote(block.Start.Format(model.DateTimeFormat)),
		sqlite.Quote(block.End.Format(model.DateTimeFormat)),
		storedURL(block.URL),
		id,
	)
}
