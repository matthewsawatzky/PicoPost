// Package identity implements lightweight browser identities.
//
// An identity is deliberately not a user account: it is a persistent
// browser-scoped handle with a display name. The server stores only a
// hash of the secret key.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxNameBytes is the maximum display name length.
	MaxNameBytes = 64
	// MaxNameRunes is the maximum display name length in runes.
	MaxNameRunes = 32
)

// Identity is a browser identity as stored in the database.
type Identity struct {
	ID          string
	KeyHash     string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// Store persists identities.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create generates a new identity with a fresh ID and secret key.
// It returns the identity and the plaintext secret key. The key is
// returned exactly once and must not be logged.
func (s *Store) Create() (*Identity, string, error) {
	id, err := randomID("id_", 16)
	if err != nil {
		return nil, "", err
	}
	key, err := randomID("pi_", 24)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().Unix()
	ident := &Identity{
		ID:        id,
		KeyHash:   HashKey(key),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.Exec(
		`INSERT INTO identities (id, key_hash, display_name, created_at, updated_at) VALUES (?, ?, NULL, ?, ?)`,
		ident.ID, ident.KeyHash, now, now,
	); err != nil {
		return nil, "", fmt.Errorf("insert identity: %w", err)
	}
	return ident, key, nil
}

// GetByID returns the identity with the given public ID.
func (s *Store) GetByID(id string) (*Identity, error) {
	row := s.db.QueryRow(
		`SELECT id, key_hash, display_name, created_at, updated_at FROM identities WHERE id = ?`, id)
	return scanIdentity(row)
}

// GetByKey returns the identity whose key hash matches the given key.
func (s *Store) GetByKey(key string) (*Identity, error) {
	row := s.db.QueryRow(
		`SELECT id, key_hash, display_name, created_at, updated_at FROM identities WHERE key_hash = ?`, HashKey(key))
	return scanIdentity(row)
}

// SetName updates the display name of the identity with the given ID.
// The name must already be validated by ValidateName.
func (s *Store) SetName(id, name string) error {
	res, err := s.db.Exec(
		`UPDATE identities SET display_name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update identity name: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanIdentity(row *sql.Row) (*Identity, error) {
	var i Identity
	var name sql.NullString
	err := row.Scan(&i.ID, &i.KeyHash, &name, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	i.DisplayName = name.String
	return &i, nil
}

// HashKey returns the hex SHA-256 hash of a secret key.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// KeyMatches reports whether key hashes to the stored hash, in
// constant time.
func KeyMatches(key, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashKey(key)), []byte(storedHash)) == 1
}

// ValidateName checks a display name. Names are optional; an empty
// string is valid. Names must be printable and within length limits.
func ValidateName(name string) error {
	if name == "" {
		return nil
	}
	if len(name) > MaxNameBytes {
		return fmt.Errorf("name too long (max %d bytes)", MaxNameBytes)
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return fmt.Errorf("name too long (max %d characters)", MaxNameRunes)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("name contains control characters")
		}
	}
	return nil
}

// NormalizeName trims surrounding whitespace for storage.
func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func randomID(prefix string, nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
