package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const (
	dbDir  = "/tmp"
	dbName = "tinygit.db"
)

// DB handles our SQLite connection and operations
type DB struct {
	*sql.DB
}

// New creates a new database manager
func New() (*DB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, dbName)

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize manager
	m := &DB{db}

	// Initialize database schema
	if err := m.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return m, nil
}

// initSchema creates the necessary tables if they don't exist
func (m *DB) initSchema() error {
	_, err := m.Exec(`
		CREATE TABLE IF NOT EXISTS state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create state table: %w", err)
	}

	// Create trigger to update updated_at timestamp
	_, err = m.Exec(`
		CREATE TRIGGER IF NOT EXISTS update_state_timestamp
		AFTER UPDATE ON state
		BEGIN
			UPDATE state SET updated_at = CURRENT_TIMESTAMP
			WHERE key = NEW.key;
		END;
	`)
	if err != nil {
		return fmt.Errorf("failed to create timestamp trigger: %w", err)
	}

	return nil
}

// SaveRepoURL saves or updates the repository URL
func (m *DB) SaveRepoURL(url string) error {
	_, err := m.Exec(`
		INSERT INTO state (key, value)
		VALUES ('repo_url', ?)
		ON CONFLICT(key) DO UPDATE SET
		value = excluded.value
	`, url)
	if err != nil {
		return fmt.Errorf("failed to save repo URL: %w", err)
	}
	return nil
}

// GetRepoURL retrieves the stored repository URL
func (m *DB) GetRepoURL() (string, error) {
	var url string
	err := m.QueryRow(`
		SELECT value FROM state
		WHERE key = 'repo_url'
	`).Scan(&url)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get repo URL: %w", err)
	}
	return url, nil
}

// Close closes the database connection
func (m *DB) Close() error {
	return m.DB.Close()
}

// GetState retrieves a value by key from the state table
