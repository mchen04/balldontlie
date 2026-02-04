package positions

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Position represents a Kalshi position
type Position struct {
	ID         int64
	GameID     string
	HomeTeam   string
	AwayTeam   string
	MarketType string // "moneyline", "spread", "total"
	Side       string // "home", "away", "over", "under"
	EntryPrice float64
	Contracts  int
	CreatedAt  time.Time
}

// DB handles position storage
type DB struct {
	db *sql.DB
}

// NewDB creates a new position database
func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS positions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		game_id TEXT NOT NULL,
		home_team TEXT NOT NULL,
		away_team TEXT NOT NULL,
		market_type TEXT NOT NULL,
		side TEXT NOT NULL,
		entry_price REAL NOT NULL,
		contracts INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_positions_game ON positions(game_id);
	CREATE INDEX IF NOT EXISTS idx_positions_active ON positions(market_type, side);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	return nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// AddPosition adds a new position
func (d *DB) AddPosition(pos Position) (int64, error) {
	result, err := d.db.Exec(`
		INSERT INTO positions (game_id, home_team, away_team, market_type, side, entry_price, contracts)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, pos.GameID, pos.HomeTeam, pos.AwayTeam, pos.MarketType, pos.Side, pos.EntryPrice, pos.Contracts)
	if err != nil {
		return 0, fmt.Errorf("inserting position: %w", err)
	}

	return result.LastInsertId()
}

// GetPosition retrieves a position by ID
func (d *DB) GetPosition(id int64) (*Position, error) {
	row := d.db.QueryRow(`
		SELECT id, game_id, home_team, away_team, market_type, side, entry_price, contracts, created_at
		FROM positions WHERE id = ?
	`, id)

	var pos Position
	err := row.Scan(&pos.ID, &pos.GameID, &pos.HomeTeam, &pos.AwayTeam,
		&pos.MarketType, &pos.Side, &pos.EntryPrice, &pos.Contracts, &pos.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning position: %w", err)
	}

	return &pos, nil
}

// GetAllPositions retrieves all positions
func (d *DB) GetAllPositions() ([]Position, error) {
	rows, err := d.db.Query(`
		SELECT id, game_id, home_team, away_team, market_type, side, entry_price, contracts, created_at
		FROM positions
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying positions: %w", err)
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var pos Position
		if err := rows.Scan(&pos.ID, &pos.GameID, &pos.HomeTeam, &pos.AwayTeam,
			&pos.MarketType, &pos.Side, &pos.EntryPrice, &pos.Contracts, &pos.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning position row: %w", err)
		}
		positions = append(positions, pos)
	}

	return positions, rows.Err()
}

// GetPositionsByGame retrieves positions for a specific game
func (d *DB) GetPositionsByGame(gameID string) ([]Position, error) {
	rows, err := d.db.Query(`
		SELECT id, game_id, home_team, away_team, market_type, side, entry_price, contracts, created_at
		FROM positions
		WHERE game_id = ?
		ORDER BY created_at DESC
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("querying positions by game: %w", err)
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var pos Position
		if err := rows.Scan(&pos.ID, &pos.GameID, &pos.HomeTeam, &pos.AwayTeam,
			&pos.MarketType, &pos.Side, &pos.EntryPrice, &pos.Contracts, &pos.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning position row: %w", err)
		}
		positions = append(positions, pos)
	}

	return positions, rows.Err()
}

// DeletePosition removes a position
func (d *DB) DeletePosition(id int64) error {
	_, err := d.db.Exec("DELETE FROM positions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting position: %w", err)
	}
	return nil
}

// UpdateContracts updates the contract count for a position
func (d *DB) UpdateContracts(id int64, contracts int) error {
	_, err := d.db.Exec("UPDATE positions SET contracts = ? WHERE id = ?", contracts, id)
	if err != nil {
		return fmt.Errorf("updating contracts: %w", err)
	}
	return nil
}
