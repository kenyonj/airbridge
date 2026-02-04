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
	DeviceType      string    `json:"device_type"`       // "airplay" or "chromecast"
	AirPlayDeviceID string    `json:"airplay_device_id"` // Legacy: device ID for AirPlay
	DeviceID        string    `json:"device_id"`         // Generic device ID
	AirPlayName     string    `json:"airplay_name"`      // Legacy: AirPlay device name
	DeviceName      string    `json:"device_name"`       // Generic device name
	Port            int       `json:"port"`
	Enabled         bool      `json:"enabled"`
	CastEnabled     bool      `json:"cast_enabled"` // Chromecast receiver enabled
	CastPort        int       `json:"cast_port"`    // Chromecast receiver port (default 8009)
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

	// Add device_type column if it doesn't exist (defaults to 'airplay' for existing rows)
	db.addColumnIfNotExists("renderers", "device_type", "TEXT NOT NULL DEFAULT 'airplay'")
	// Add generic device_id column
	db.addColumnIfNotExists("renderers", "device_id", "TEXT NOT NULL DEFAULT ''")
	// Add generic device_name column
	db.addColumnIfNotExists("renderers", "device_name", "TEXT NOT NULL DEFAULT ''")
	// Add Chromecast receiver columns
	db.addColumnIfNotExists("renderers", "cast_enabled", "INTEGER DEFAULT 0")
	db.addColumnIfNotExists("renderers", "cast_port", "INTEGER DEFAULT 8009")

	log.Println("Database migrations complete")
	return nil
}

// addColumnIfNotExists adds a column to a table if it doesn't already exist.
func (db *DB) addColumnIfNotExists(table, column, colType string) {
	// Check if column exists
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&count)
	if err != nil || count > 0 {
		return // Column exists or error checking
	}

	// Add the column
	_, err = db.conn.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, colType))
	if err != nil {
		log.Printf("Failed to add column %s.%s: %v", table, column, err)
	}
}

// ListRenderers returns all configured renderers.
func (db *DB) ListRenderers() ([]Renderer, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, airplay_device_id, airplay_name, port, enabled, created_at,
		       COALESCE(device_type, 'airplay') as device_type,
		       COALESCE(device_id, '') as device_id,
		       COALESCE(device_name, '') as device_name,
		       COALESCE(cast_enabled, 0) as cast_enabled,
		       COALESCE(cast_port, 8009) as cast_port
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
		var enabled, castEnabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.AirPlayDeviceID, &r.AirPlayName, &r.Port, &enabled, &r.CreatedAt,
			&r.DeviceType, &r.DeviceID, &r.DeviceName, &castEnabled, &r.CastPort); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		r.CastEnabled = castEnabled == 1
		// Backfill generic fields from legacy fields if empty
		if r.DeviceID == "" {
			r.DeviceID = r.AirPlayDeviceID
		}
		if r.DeviceName == "" {
			r.DeviceName = r.AirPlayName
		}
		renderers = append(renderers, r)
	}
	return renderers, rows.Err()
}

// GetRenderer returns a renderer by ID.
func (db *DB) GetRenderer(id string) (*Renderer, error) {
	var r Renderer
	var enabled, castEnabled int
	err := db.conn.QueryRow(`
		SELECT id, name, airplay_device_id, airplay_name, port, enabled, created_at,
		       COALESCE(device_type, 'airplay') as device_type,
		       COALESCE(device_id, '') as device_id,
		       COALESCE(device_name, '') as device_name,
		       COALESCE(cast_enabled, 0) as cast_enabled,
		       COALESCE(cast_port, 8009) as cast_port
		FROM renderers WHERE id = ?
	`, id).Scan(&r.ID, &r.Name, &r.AirPlayDeviceID, &r.AirPlayName, &r.Port, &enabled, &r.CreatedAt,
		&r.DeviceType, &r.DeviceID, &r.DeviceName, &castEnabled, &r.CastPort)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled == 1
	r.CastEnabled = castEnabled == 1
	// Backfill generic fields from legacy fields if empty
	if r.DeviceID == "" {
		r.DeviceID = r.AirPlayDeviceID
	}
	if r.DeviceName == "" {
		r.DeviceName = r.AirPlayName
	}
	return &r, nil
}

// CreateRenderer creates a new renderer.
func (db *DB) CreateRenderer(r *Renderer) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	// Default device type to airplay if not set
	deviceType := r.DeviceType
	if deviceType == "" {
		deviceType = "airplay"
	}
	// Use generic fields, falling back to legacy fields
	deviceID := r.DeviceID
	if deviceID == "" {
		deviceID = r.AirPlayDeviceID
	}
	deviceName := r.DeviceName
	if deviceName == "" {
		deviceName = r.AirPlayName
	}
	_, err := db.conn.Exec(`
		INSERT INTO renderers (id, name, airplay_device_id, airplay_name, port, enabled, created_at, device_type, device_id, device_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.Name, r.AirPlayDeviceID, r.AirPlayName, r.Port, enabled, r.CreatedAt, deviceType, deviceID, deviceName)
	return err
}

// UpdateRenderer updates an existing renderer.
func (db *DB) UpdateRenderer(r *Renderer) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	deviceType := r.DeviceType
	if deviceType == "" {
		deviceType = "airplay"
	}
	deviceID := r.DeviceID
	if deviceID == "" {
		deviceID = r.AirPlayDeviceID
	}
	deviceName := r.DeviceName
	if deviceName == "" {
		deviceName = r.AirPlayName
	}
	_, err := db.conn.Exec(`
		UPDATE renderers SET name = ?, airplay_device_id = ?, airplay_name = ?, port = ?, enabled = ?,
		       device_type = ?, device_id = ?, device_name = ?
		WHERE id = ?
	`, r.Name, r.AirPlayDeviceID, r.AirPlayName, r.Port, enabled, deviceType, deviceID, deviceName, r.ID)
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

// ToggleCastReceiver toggles the cast_enabled state of a renderer.
func (db *DB) ToggleCastReceiver(id string) error {
	_, err := db.conn.Exec(`UPDATE renderers SET cast_enabled = NOT cast_enabled WHERE id = ?`, id)
	return err
}

// SetCastReceiver sets the cast_enabled state for a renderer.
func (db *DB) SetCastReceiver(id string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := db.conn.Exec(`UPDATE renderers SET cast_enabled = ? WHERE id = ?`, val, id)
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
