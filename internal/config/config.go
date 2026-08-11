// Package config loads and validates PicoPost configuration from a TOML file.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the full PicoPost configuration.
type Config struct {
	Server    Server    `toml:"server"`
	Storage   Storage   `toml:"storage"`
	CORS      CORS      `toml:"cors"`
	Posts     Posts     `toml:"posts"`
	Identity  Identity  `toml:"identity"`
	RateLimit RateLimit `toml:"rate_limit"`
	Filters   Filters   `toml:"filters"`
}

type Server struct {
	Listen string `toml:"listen"`
}

type Storage struct {
	Database string `toml:"database"`
}

type CORS struct {
	Origins []string `toml:"origins"`
}

type Posts struct {
	MaxBodyBytes     int `toml:"max_body_bytes"`
	MaxTextBytes     int `toml:"max_text_bytes"`
	MaxMetadataBytes int `toml:"max_metadata_bytes"`
	MaxMetadataKeys  int `toml:"max_metadata_keys"`
	MaxKeyLength     int `toml:"max_key_length"`
	MaxURLsPerPost   int `toml:"max_urls_per_post"`
	PageSize         int `toml:"page_size"`
}

type Identity struct {
	Anonymous bool `toml:"anonymous"`
	Browser   bool `toml:"browser"`
}

type RateLimit struct {
	PostsPerMinute int  `toml:"posts_per_minute"`
	TrustForwarded bool `toml:"trust_forwarded"`
}

type Filters struct {
	Username FilterList `toml:"username"`
	Text     FilterList `toml:"text"`
}

type FilterList struct {
	Deny []string `toml:"deny"`
}

// Default returns a Config populated with sensible defaults.
func Default() Config {
	return Config{
		Server: Server{Listen: "127.0.0.1:8080"},
		Storage: Storage{
			Database: "./data/picopost.db",
		},
		CORS: CORS{},
		Posts: Posts{
			MaxBodyBytes:     1024,
			MaxTextBytes:     768,
			MaxMetadataBytes: 256,
			MaxMetadataKeys:  16,
			MaxKeyLength:     32,
			MaxURLsPerPost:   5,
			PageSize:         50,
		},
		Identity: Identity{
			Anonymous: true,
			Browser:   true,
		},
		RateLimit: RateLimit{
			PostsPerMinute: 20,
			TrustForwarded: false,
		},
		Filters: Filters{
			Username: FilterList{Deny: []string{"admin", "administrator", "moderator", "system"}},
			Text:     FilterList{Deny: []string{}},
		},
	}
}

// ErrMissingFile is returned by Load when the config file does not exist.
var ErrMissingFile = errors.New("config file not found")

// Load reads the config file at path, applies defaults for unset values,
// and validates the result. A missing file is an error.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("%w: %s", ErrMissingFile, path)
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("config %s: unknown keys: %s", path, undecoded)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that the configuration is internally consistent.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return fmt.Errorf("config: [server] listen must not be empty")
	}
	if strings.TrimSpace(c.Storage.Database) == "" {
		return fmt.Errorf("config: [storage] database must not be empty")
	}
	if c.Posts.MaxBodyBytes < 1 {
		return fmt.Errorf("config: [posts] max_body_bytes must be >= 1")
	}
	if c.Posts.MaxTextBytes < 1 {
		return fmt.Errorf("config: [posts] max_text_bytes must be >= 1")
	}
	if c.Posts.MaxTextBytes > c.Posts.MaxBodyBytes {
		return fmt.Errorf("config: [posts] max_text_bytes (%d) must not exceed max_body_bytes (%d)", c.Posts.MaxTextBytes, c.Posts.MaxBodyBytes)
	}
	if c.Posts.MaxMetadataBytes < 0 {
		return fmt.Errorf("config: [posts] max_metadata_bytes must be >= 0")
	}
	if c.Posts.MaxMetadataKeys < 0 {
		return fmt.Errorf("config: [posts] max_metadata_keys must be >= 0")
	}
	if c.Posts.MaxKeyLength < 1 {
		return fmt.Errorf("config: [posts] max_key_length must be >= 1")
	}
	if c.Posts.MaxURLsPerPost < 0 {
		return fmt.Errorf("config: [posts] max_urls_per_post must be >= 0")
	}
	if c.Posts.PageSize < 1 {
		return fmt.Errorf("config: [posts] page_size must be >= 1")
	}
	if c.RateLimit.PostsPerMinute < 0 {
		return fmt.Errorf("config: [rate_limit] posts_per_minute must be >= 0")
	}
	for _, o := range c.CORS.Origins {
		if o == "*" {
			continue
		}
		if !strings.HasPrefix(o, "http://") && !strings.HasPrefix(o, "https://") {
			return fmt.Errorf("config: [cors] origin %q must be a full origin (http:// or https://) or \"*\"", o)
		}
		if strings.Contains(o, "://") && strings.ContainsAny(strings.SplitN(o, "://", 2)[1], "/") {
			return fmt.Errorf("config: [cors] origin %q must not contain a path", o)
		}
	}
	return nil
}
