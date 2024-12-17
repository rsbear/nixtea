package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"walross/nixtea/internal/config"

	"github.com/charmbracelet/log"
	_ "github.com/mattn/go-sqlite3"
)

type Repository struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DB handles our SQLite connection and operations
type DB struct {
	*sql.DB
}

// New creates a new database manager
func New(cfg *config.Config) (*DB, error) {
	if err := os.MkdirAll(cfg.DBDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	dbPath := filepath.Join(cfg.DBDir, cfg.DBName)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	m := &DB{db}

	if err := m.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return m, nil
}

func (m *DB) initSchema() error {
	_, err := m.Exec(`
		CREATE TABLE IF NOT EXISTS repositories (
			id TEXT PRIMARY KEY,
			url TEXT NOT NULL UNIQUE,
			active BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create repositories table: %w", err)
	}

	_, err = m.Exec(`
		CREATE TRIGGER IF NOT EXISTS update_repos_timestamp
		AFTER UPDATE ON repositories
		BEGIN
			UPDATE repositories SET updated_at = CURRENT_TIMESTAMP
			WHERE id = NEW.id;
		END;
	`)
	if err != nil {
		return fmt.Errorf("failed to create timestamp trigger: %w", err)
	}

	return nil
}

// SaveRepo saves a new repository or updates an existing one
func (m *DB) SaveRepo(url string) (*Repository, error) {
	tx, err := m.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if repo exists
	var existingRepo Repository
	err = tx.QueryRow(`
		SELECT id, url, active, created_at, updated_at
		FROM repositories
		WHERE url = ?
	`, url).Scan(&existingRepo.ID, &existingRepo.URL, &existingRepo.Active, &existingRepo.CreatedAt, &existingRepo.UpdatedAt)

	if err == nil {
		// Repo exists, make it active
		_, err = tx.Exec(`
			UPDATE repositories SET active = CASE 
				WHEN url = ? THEN 1 
				ELSE 0 
			END
		`, url)
		if err != nil {
			return nil, fmt.Errorf("failed to update repository status: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		existingRepo.Active = true
		return &existingRepo, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing repository: %w", err)
	}

	// Deactivate all repos and insert new one
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), url[:min(8, len(url))])
	_, err = tx.Exec(`UPDATE repositories SET active = 0`)
	if err != nil {
		return nil, fmt.Errorf("failed to deactivate repositories: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO repositories (id, url, active)
		VALUES (?, ?, 1)
	`, id, url)
	if err != nil {
		return nil, fmt.Errorf("failed to save repository: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return m.GetRepoByID(id)
}

// GetRepos returns all repositories
func (m *DB) GetRepos() ([]Repository, error) {
	rows, err := m.Query(`
		SELECT id, url, active, created_at, updated_at
		FROM repositories
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query repositories: %w", err)
	}
	defer rows.Close()

	var repos []Repository
	for rows.Next() {
		var repo Repository
		err := rows.Scan(&repo.ID, &repo.URL, &repo.Active, &repo.CreatedAt, &repo.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan repository row: %w", err)
		}
		repos = append(repos, repo)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating repository rows: %w", err)
	}

	return repos, nil
}

// GetRepoByID returns a single repository by ID
func (m *DB) GetRepoByID(id string) (*Repository, error) {
	var repo Repository
	err := m.QueryRow(`
		SELECT id, url, active, created_at, updated_at
		FROM repositories
		WHERE id = ?
	`, id).Scan(&repo.ID, &repo.URL, &repo.Active, &repo.CreatedAt, &repo.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &repo, nil
}

// GetRepoURL returns the currently active repository URL
func (m *DB) GetRepoURL() (string, error) {
	log.Info("Attempting to retrieve active repo URL")

	var url string
	err := m.QueryRow(`
		SELECT url FROM repositories
		WHERE active = 1
		LIMIT 1
	`).Scan(&url)

	if err == sql.ErrNoRows {
		log.Info("No active repo URL found")
		return "", nil
	}
	if err != nil {
		log.Error("Failed to get repo URL", "error", err)
		return "", fmt.Errorf("failed to get repo URL: %w", err)
	}

	log.Info("Successfully retrieved repo URL", "url", url)
	return url, nil
}

// DeleteRepo deletes a repository by ID
func (m *DB) DeleteRepo(id string) error {
	result, err := m.Exec(`
		DELETE FROM repositories
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("expected 1 row to be affected, got %d", rows)
	}

	return nil
}

// Helper function for string length
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Close closes the database connection
func (m *DB) Close() error {
	return m.DB.Close()
}
