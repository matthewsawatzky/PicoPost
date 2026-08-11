// Package posts defines the post model, channel validation, and the
// post store.
package posts

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxChannelLength is the maximum length of a channel name.
const MaxChannelLength = 128

// ErrChannel is returned when a channel name is invalid.
var ErrChannel = errors.New("invalid channel")

// Post is a single text record.
type Post struct {
	ID          string         `json:"id"`
	Channel     string         `json:"channel"`
	Text        string         `json:"text"`
	Meta        map[string]any `json:"meta,omitempty"`
	IdentityID  *string        `json:"identity_id,omitempty"`
	DisplayName *string        `json:"display_name,omitempty"`
	CreatedAt   int64          `json:"created_at"`
}

// NewPost is the client-supplied portion of a post.
type NewPost struct {
	Channel string         `json:"channel"`
	Text    string         `json:"text"`
	Meta    map[string]any `json:"meta,omitempty"`
	// DisplayName is only honored for anonymous posts, and is treated
	// as untrusted display data. Authenticated posts always use the
	// server-side identity name.
	DisplayName *string `json:"display_name,omitempty"`
}

// Limits are the configurable validation limits for posts.
type Limits struct {
	MaxTextBytes     int
	MaxMetadataBytes int
	MaxMetadataKeys  int
	MaxKeyLength     int
	MaxURLsPerPost   int
}

// ValidateChannel checks a channel name against the allowed character
// set and length. All errors wrap ErrChannel.
func ValidateChannel(channel string) error {
	if channel == "" {
		return ErrChannel
	}
	if len(channel) > MaxChannelLength {
		return fmt.Errorf("%w: channel too long (max %d bytes)", ErrChannel, MaxChannelLength)
	}
	for _, r := range channel {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '/':
		default:
			return fmt.Errorf("%w: channel %q contains invalid characters (allowed: a-z A-Z 0-9 - _ /)", ErrChannel, channel)
		}
	}
	return nil
}

// Validate checks a NewPost against the configured limits.
// It returns a descriptive error; callers map it to public error codes.
func (l Limits) Validate(p *NewPost) error {
	if err := ValidateChannel(p.Channel); err != nil {
		return err
	}
	if p.Text == "" {
		return errors.New("text is required")
	}
	if len(p.Text) > l.MaxTextBytes {
		return fmt.Errorf("text too long (max %d bytes)", l.MaxTextBytes)
	}
	if !utf8.ValidString(p.Text) {
		return errors.New("text is not valid UTF-8")
	}
	if p.Meta != nil {
		if l.MaxMetadataKeys > 0 && len(p.Meta) > l.MaxMetadataKeys {
			return fmt.Errorf("too many metadata keys (max %d)", l.MaxMetadataKeys)
		}
		for k := range p.Meta {
			if len(k) > l.MaxKeyLength {
				return fmt.Errorf("metadata key too long (max %d bytes)", l.MaxKeyLength)
			}
			if k == "" {
				return errors.New("metadata key must not be empty")
			}
		}
		raw, err := json.Marshal(p.Meta)
		if err != nil {
			return fmt.Errorf("invalid metadata: %w", err)
		}
		if l.MaxMetadataBytes > 0 && len(raw) > l.MaxMetadataBytes {
			return fmt.Errorf("metadata too large (max %d bytes)", l.MaxMetadataBytes)
		}
	}
	return nil
}

// Store persists posts.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a post and returns the stored record. The post's
// identity and display name are determined by the server, never by the
// client.
func (s *Store) Create(channel, text string, meta map[string]any, identityID, displayName *string) (*Post, error) {
	metaJSON := "{}"
	if meta != nil {
		raw, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = string(raw)
	}
	now := time.Now().Unix()
	id, err := newID()
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO posts (id, channel, text, metadata_json, identity_id, display_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, channel, text, metaJSON, identityID, displayName, now)
	if err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}
	return &Post{
		ID:          id,
		Channel:     channel,
		Text:        text,
		Meta:        meta,
		IdentityID:  identityID,
		DisplayName: displayName,
		CreatedAt:   now,
	}, nil
}

// ListOptions controls post listing.
type ListOptions struct {
	Channel  string
	Limit    int
	Before   int64  // cursor: only posts created before this timestamp
	BeforeID string // cursor: tie-breaker for posts sharing the timestamp
}

// List returns posts matching the options, newest first.
func (s *Store) List(opts ListOptions) ([]*Post, error) {
	query := `SELECT id, channel, text, metadata_json, identity_id, display_name, created_at FROM posts`
	var args []any
	var conds []string
	if opts.Channel != "" {
		conds = append(conds, `channel = ?`)
		args = append(args, opts.Channel)
	}
	if opts.Before > 0 {
		if opts.BeforeID != "" {
			conds = append(conds, `(created_at < ? OR (created_at = ? AND id < ?))`)
			args = append(args, opts.Before, opts.Before, opts.BeforeID)
		} else {
			conds = append(conds, `created_at < ?`)
			args = append(args, opts.Before)
		}
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, opts.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	var out []*Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns a single post by ID.
func (s *Store) Get(id string) (*Post, error) {
	row := s.db.QueryRow(
		`SELECT id, channel, text, metadata_json, identity_id, display_name, created_at FROM posts WHERE id = ?`, id)
	p, err := scanPost(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return p, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPost(row scanner) (*Post, error) {
	var p Post
	var metaJSON string
	var identityID, displayName sql.NullString
	err := row.Scan(&p.ID, &p.Channel, &p.Text, &metaJSON, &identityID, &displayName, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if identityID.Valid {
		p.IdentityID = &identityID.String
	}
	if displayName.Valid {
		p.DisplayName = &displayName.String
	}
	if metaJSON != "" && metaJSON != "{}" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
			return nil, fmt.Errorf("corrupt metadata for post %s: %w", p.ID, err)
		}
		p.Meta = meta
	}
	return &p, nil
}

func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "p_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
