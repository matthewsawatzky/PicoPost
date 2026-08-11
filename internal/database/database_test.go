package database

import (
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	v, err := db.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Errorf("schema version = %d, want 1", v)
	}

	// Tables exist.
	for _, table := range []string{"posts", "identities", "schema_migrations"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestOpenIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	v, err := db2.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Errorf("schema version = %d after reopen, want 1", v)
	}
}

func TestCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if n, err := db.CountPosts(); err != nil || n != 0 {
		t.Errorf("CountPosts = %d, %v", n, err)
	}
	if n, err := db.CountIdentities(); err != nil || n != 0 {
		t.Errorf("CountIdentities = %d, %v", n, err)
	}

	if _, err := db.Exec(`INSERT INTO posts (id, channel, text, metadata_json, created_at) VALUES ('p_1', 'chat/general', 'hi', '{}', 100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO identities (id, key_hash, created_at, updated_at) VALUES ('id_1', 'hash', 100, 100)`); err != nil {
		t.Fatal(err)
	}

	if n, err := db.CountPosts(); err != nil || n != 1 {
		t.Errorf("CountPosts = %d, %v", n, err)
	}
	if n, err := db.CountIdentities(); err != nil || n != 1 {
		t.Errorf("CountIdentities = %d, %v", n, err)
	}

	oldest, err := db.OldestPostTime()
	if err != nil || oldest != 100 {
		t.Errorf("OldestPostTime = %d, %v", oldest, err)
	}
	newest, err := db.NewestPostTime()
	if err != nil || newest != 100 {
		t.Errorf("NewestPostTime = %d, %v", newest, err)
	}
}
