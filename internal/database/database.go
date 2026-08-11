// Package database provides SQLite persistence for PicoPost.
package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// DB wraps the SQLite connection.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path and runs
// pending migrations. The parent directory is created if missing.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies embedded migrations in filename order, tracking applied
// versions in schema_migrations.
func (db *DB) migrate() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}
		sqlBytes, err := embeddedMigrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().Unix()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func migrationVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("migration %s: bad version prefix", name)
	}
	return v, nil
}

// SchemaVersion returns the highest applied migration version.
func (db *DB) SchemaVersion() (int, error) {
	var v sql.NullInt64
	err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

// CountPosts returns the total number of posts.
func (db *DB) CountPosts() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n)
	return n, err
}

// CountIdentities returns the total number of identities.
func (db *DB) CountIdentities() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM identities`).Scan(&n)
	return n, err
}

// OldestPostTime returns the created_at of the oldest post, or 0 if none.
func (db *DB) OldestPostTime() (int64, error) {
	var t sql.NullInt64
	err := db.QueryRow(`SELECT MIN(created_at) FROM posts`).Scan(&t)
	if err != nil {
		return 0, err
	}
	return t.Int64, nil
}

// NewestPostTime returns the created_at of the newest post, or 0 if none.
func (db *DB) NewestPostTime() (int64, error) {
	var t sql.NullInt64
	err := db.QueryRow(`SELECT MAX(created_at) FROM posts`).Scan(&t)
	if err != nil {
		return 0, err
	}
	return t.Int64, nil
}
