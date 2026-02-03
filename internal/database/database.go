// Package database provides SQLite storage for Airbridge configuration.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection.
type DB struct {
	conn *sql.DB
}

// Renderer represents a configured DLNA renderer.
type Renderer struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	AirPlayDeviceID string    `json:"airplay_device_id"`
	AirPlayName     string    `json:"airplay_name"`
	Port            int       `json:"port"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

// Open opens or creates the database at the given path.
func Open(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate runs database migrations.
func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS renderers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			airplay_device_id TEXT NOT NULL,
			airplay_name TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL,
			enabled INTEGER DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("execute migration: %w", err)
		}
	}

	log.Println("Database migrations complete")
	return nil
}

// ListRenderers returns all configured renderers.
func (db *DB) ListRenderers() ([]Renderer, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, airplay_device_id, airplay_name, port, enabled, created_at 
		FROM renderers 
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var renderers []Renderer
	for rows.Next() {
		var r Renderer
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.AirPlayDeviceID, &r.AirPlayName, &r.Port, &enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		renderers = append(renderers, r)
	}
	return renderers, rows.Err()
}

// GetRenderer returns a renderer by ID.
func (db *DB) GetRenderer(id string) (*Renderer, error) {
	var r Renderer
	var enabled int
	err := db.conn.QueryRow(`
		SELECT id, name, airplay_device_id, airplay_name, port, enabled, created_at 
		FROM renderers WHERE id = ?
	`, id).Scan(&r.ID, &r.Name, &r.AirPlayDeviceID, &r.AirPlayName, &r.Port, &enabled, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled == 1
	return &r, nil
}

// CreateRenderer creates a new renderer.
func (db *DB) CreateRenderer(r *Renderer) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	_, err := db.conn.Exec(`
		INSERT INTO renderers (id, name, airplay_device_id, airplay_name, port, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.Name, r.AirPlayDeviceID, r.AirPlayName, r.Port, enabled, r.CreatedAt)
	return err
}

// UpdateRenderer updates an existing renderer.
func (db *DB) UpdateRenderer(r *Renderer) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	_, err := db.conn.Exec(`
		UPDATE renderers SET name = ?, airplay_device_id = ?, airplay_name = ?, port = ?, enabled = ?
		WHERE id = ?
	`, r.Name, r.AirPlayDeviceID, r.AirPlayName, r.Port, enabled, r.ID)
	return err
}

// DeleteRenderer removes a renderer.
func (db *DB) DeleteRenderer(id string) error {
	_, err := db.conn.Exec(`DELETE FROM renderers WHERE id = ?`, id)
	return err
}

// RenameRenderer updates just the name of a renderer.
func (db *DB) RenameRenderer(id, name string) error {
	_, err := db.conn.Exec(`UPDATE renderers SET name = ? WHERE id = ?`, name, id)
	return err
}

// ToggleRenderer toggles the enabled state of a renderer.
func (db *DB) ToggleRenderer(id string) error {
	_, err := db.conn.Exec(`UPDATE renderers SET enabled = NOT enabled WHERE id = ?`, id)
	return err
}

// GetNextPort returns the next available port starting from basePort.
func (db *DB) GetNextPort(basePort int) (int, error) {
	var maxPort sql.NullInt64
	err := db.conn.QueryRow(`SELECT MAX(port) FROM renderers`).Scan(&maxPort)
	if err != nil {
		return basePort, err
	}
	if !maxPort.Valid {
		return basePort, nil
	}
	return int(maxPort.Int64) + 1, nil
}

// GetSetting retrieves a setting value.
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting stores a setting value.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}
