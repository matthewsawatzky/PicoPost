package identity

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE identities (
		id TEXT PRIMARY KEY,
		key_hash TEXT NOT NULL UNIQUE,
		display_name TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCreateAndGetByKey(t *testing.T) {
	s := NewStore(openTestDB(t))
	ident, key, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ident.ID, "id_") {
		t.Errorf("unexpected id %q", ident.ID)
	}
	if !strings.HasPrefix(key, "pi_") {
		t.Errorf("unexpected key %q", key)
	}
	if ident.KeyHash == key {
		t.Error("stored hash must not equal the plaintext key")
	}

	got, err := s.GetByKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != ident.ID {
		t.Errorf("GetByKey returned %q, want %q", got.ID, ident.ID)
	}

	if _, err := s.GetByKey("pi_wrongkey"); err != sql.ErrNoRows {
		t.Errorf("GetByKey with wrong key: got %v, want sql.ErrNoRows", err)
	}
}

func TestSetName(t *testing.T) {
	s := NewStore(openTestDB(t))
	ident, key, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetName(ident.ID, "Mat"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Mat" {
		t.Errorf("display name = %q, want %q", got.DisplayName, "Mat")
	}
	if got.UpdatedAt < got.CreatedAt {
		t.Error("updated_at should not be before created_at")
	}
}

func TestKeyMatches(t *testing.T) {
	key := "pi_secret"
	hash := HashKey(key)
	if !KeyMatches(key, hash) {
		t.Error("KeyMatches should accept the correct key")
	}
	if KeyMatches("pi_other", hash) {
		t.Error("KeyMatches should reject a wrong key")
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"", "Mat", "Mat O'Brien", "名前", "a", strings.Repeat("x", 32)}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{strings.Repeat("x", 33), "a\x00b", "a\nb", strings.Repeat("x", 65)}
	for _, n := range invalid {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", n)
		}
	}
}
