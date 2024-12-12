package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"walross/nixtea/internal/config"

	"github.com/charmbracelet/log"

	_ "github.com/mattn/go-sqlite3"
)

// DB handles our SQLite connection and operations
type DB struct {
	*sql.DB
}

// New creates a new database manager
func New(cfg *config.Config) (*DB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(cfg.DBDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	dbPath := filepath.Join(cfg.DBDir, cfg.DBName)

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
	// Initialize state table
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

	// Create state table trigger
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

	// Initialize repositories table
	if err := m.initReposSchema(); err != nil {
		return fmt.Errorf("failed to initialize repositories schema: %w", err)
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
	log.Infof("Saved repo URL: %s", url)
	return nil
}

func (m *DB) GetRepoURL() (string, error) {
	log.Info("Attempting to retrieve repo URL")

	var url string
	err := m.QueryRow(`
        SELECT value FROM state
        WHERE key = 'repo_url'
    `).Scan(&url)

	if err == sql.ErrNoRows {
		log.Info("No repo URL found in database")
		return "", nil
	}
	if err != nil {
		log.Error("Failed to get repo URL", "error", err)
		return "", fmt.Errorf("failed to get repo URL: %w", err)
	}

	log.Info("Successfully retrieved repo URL", "url", url)
	return url, nil
}

// Close closes the database connection
func (m *DB) Close() error {
	return m.DB.Close()
}

// GetState retrieves a value by key from the state table
