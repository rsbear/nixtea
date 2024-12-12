// file: internal/db/repos.go

package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// initReposSchema creates the repositories table if it doesn't exist
func (m *DB) initReposSchema() error {
	_, err := m.Exec(`
		CREATE TABLE IF NOT EXISTS repositories (
			id TEXT PRIMARY KEY,
			url TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create repositories table: %w", err)
	}

	// Create trigger to update updated_at timestamp
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

// SaveRepo saves a new repository to the database
func (m *DB) SaveRepo(url string) (*Repository, error) {
	// Generate a unique ID (using timestamp + first 8 chars of URL as a simple method)
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), url[:min(8, len(url))])

	result, err := m.Exec(`
		INSERT INTO repositories (id, url)
		VALUES (?, ?)
	`, id, url)
	if err != nil {
		return nil, fmt.Errorf("failed to save repository: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get affected rows: %w", err)
	}
	if rows != 1 {
		return nil, fmt.Errorf("expected 1 row to be affected, got %d", rows)
	}

	return m.GetRepoByID(id)
}

// GetRepos returns all repositories
func (m *DB) GetRepos() ([]Repository, error) {
	rows, err := m.Query(`
		SELECT id, url, created_at, updated_at
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
		err := rows.Scan(&repo.ID, &repo.URL, &repo.CreatedAt, &repo.UpdatedAt)
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
		SELECT id, url, created_at, updated_at
		FROM repositories
		WHERE id = ?
	`, id).Scan(&repo.ID, &repo.URL, &repo.CreatedAt, &repo.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &repo, nil
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
