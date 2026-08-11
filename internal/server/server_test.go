package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matthewsawatzky/PicoPost/internal/config"
	"github.com/matthewsawatzky/PicoPost/internal/database"
)

func newTestServer(t *testing.T, mutate func(*config.Config)) (*Server, *httptest.Server) {
	t.Helper()
	cfg := config.Default()
	cfg.Storage.Database = filepath.Join(t.TempDir(), "test.db")
	cfg.CORS.Origins = []string{"https://example.com"}
	cfg.RateLimit.PostsPerMinute = 1000
	if mutate != nil {
		mutate(&cfg)
	}
	db, err := database.Open(cfg.Storage.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, db, log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func doJSON(t *testing.T, method, url string, body any, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	if len(raw) > 0 {
		json.Unmarshal(raw, &out)
	}
	return res, out
}

func createIdentity(t *testing.T, ts *httptest.Server) (string, string) {
	t.Helper()
	res, out := doJSON(t, "POST", ts.URL+"/v1/identity", nil, nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create identity: status %d, body %v", res.StatusCode, out)
	}
	return out["id"].(string), out["key"].(string)
}

func authHeader(key string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + key}
}

func TestHealth(t *testing.T) {
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "GET", ts.URL+"/health", nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if out["status"] != "ok" {
		t.Errorf("body %v", out)
	}
}

func TestIdentityCreateGetPatch(t *testing.T) {
	_, ts := newTestServer(t, nil)
	id, key := createIdentity(t, ts)
	if !strings.HasPrefix(id, "id_") || !strings.HasPrefix(key, "pi_") {
		t.Fatalf("unexpected identity: id=%q key=%q", id, key)
	}

	// GET without auth -> unauthorized
	res, out := doJSON(t, "GET", ts.URL+"/v1/identity", nil, nil)
	if res.StatusCode != http.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("GET without auth: %d %v", res.StatusCode, out)
	}

	// GET with auth
	res, out = doJSON(t, "GET", ts.URL+"/v1/identity", nil, authHeader(key))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET with auth: %d %v", res.StatusCode, out)
	}
	if out["id"] != id {
		t.Errorf("id = %v, want %s", out["id"], id)
	}

	// PATCH name
	res, out = doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": "  Mat  "}, authHeader(key))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH: %d %v", res.StatusCode, out)
	}
	if out["display_name"] != "Mat" {
		t.Errorf("display_name = %v, want Mat", out["display_name"])
	}

	// PATCH without auth -> unauthorized
	res, _ = doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": "x"}, nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PATCH without auth: %d", res.StatusCode)
	}

	// PATCH with reserved name -> rejected
	res, out = doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": "Admin"}, authHeader(key))
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("PATCH reserved name: %d %v", res.StatusCode, out)
	}

	// PATCH with invalid name -> invalid_request
	res, out = doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": strings.Repeat("x", 100)}, authHeader(key))
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("PATCH long name: %d %v", res.StatusCode, out)
	}
}

func TestPostCreateAndGet(t *testing.T) {
	_, ts := newTestServer(t, nil)
	_, key := createIdentity(t, ts)

	// Set a name so the post carries it.
	doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": "Mat"}, authHeader(key))

	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "hello world",
		"meta":    map[string]any{"stars": 5},
	}, authHeader(key))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create post: %d %v", res.StatusCode, out)
	}
	if out["display_name"] != "Mat" {
		t.Errorf("display_name = %v, want Mat (server-determined)", out["display_name"])
	}
	if out["identity_id"] == nil {
		t.Error("identity_id missing")
	}
	if out["created_at"] == nil {
		t.Error("created_at missing")
	}
	meta, ok := out["meta"].(map[string]any)
	if !ok || meta["stars"] != float64(5) {
		t.Errorf("meta = %v", out["meta"])
	}
	id := out["id"].(string)

	// GET by id
	res, out = doJSON(t, "GET", ts.URL+"/v1/posts/"+id, nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get post: %d %v", res.StatusCode, out)
	}
	if out["text"] != "hello world" {
		t.Errorf("text = %v", out["text"])
	}

	// GET missing
	res, out = doJSON(t, "GET", ts.URL+"/v1/posts/p_nope", nil, nil)
	if res.StatusCode != http.StatusNotFound || out["error"] != "not_found" {
		t.Fatalf("get missing post: %d %v", res.StatusCode, out)
	}
}

func TestAnonymousPost(t *testing.T) {
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "anon hello",
	}, nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("anonymous post: %d %v", res.StatusCode, out)
	}
	if out["identity_id"] != nil {
		t.Error("anonymous post must not have identity_id")
	}
	if out["display_name"] != nil {
		t.Error("anonymous post must not have display_name")
	}
}

func TestAnonymousDisabled(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.Identity.Anonymous = false
	})
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "hello",
	}, nil)
	if res.StatusCode != http.StatusUnauthorized || out["error"] != "unauthorized" {
		t.Fatalf("anonymous post with anonymous disabled: %d %v", res.StatusCode, out)
	}
}

func TestClientSuppliedNameIgnored(t *testing.T) {
	_, ts := newTestServer(t, nil)
	_, key := createIdentity(t, ts)
	doJSON(t, "PATCH", ts.URL+"/v1/identity", map[string]any{"name": "Real"}, authHeader(key))

	// A client-supplied display_name must be ignored for authenticated
	// posts; the server uses the identity's own name.
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":      "chat/general",
		"text":         "hello",
		"display_name": "Admin",
	}, authHeader(key))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("post: %d %v", res.StatusCode, out)
	}
	if out["display_name"] != "Real" {
		t.Errorf("display_name = %v, want Real (client-supplied must be ignored)", out["display_name"])
	}

	// identity_id is not a client field at all: reject it.
	res, out = doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":     "chat/general",
		"text":        "hello",
		"identity_id": "id_fake",
	}, authHeader(key))
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("client-supplied identity_id: %d %v", res.StatusCode, out)
	}
}

func TestAnonymousDisplayName(t *testing.T) {
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":      "chat/general",
		"text":         "hello",
		"display_name": "  Guest  ",
	}, nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("anonymous post with name: %d %v", res.StatusCode, out)
	}
	if out["display_name"] != "Guest" {
		t.Errorf("display_name = %v, want Guest (trimmed)", out["display_name"])
	}
	if out["identity_id"] != nil {
		t.Error("anonymous post must not have identity_id")
	}

	// Reserved anonymous name -> rejected.
	res, out = doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":      "chat/general",
		"text":         "hello",
		"display_name": "Admin",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("anonymous reserved name: %d %v", res.StatusCode, out)
	}
}

func TestPostSizeLimits(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.Posts.MaxBodyBytes = 256
		c.Posts.MaxTextBytes = 100
	})

	// Text over the limit -> invalid_request
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    strings.Repeat("x", 101),
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("over-long text: %d %v", res.StatusCode, out)
	}

	// Body over the limit -> payload_too_large
	big := map[string]any{
		"channel": "chat/general",
		"text":    strings.Repeat("x", 300),
	}
	raw, _ := json.Marshal(big)
	req, _ := http.NewRequest("POST", ts.URL+"/v1/posts", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-large body: status %d, want 413", resp.StatusCode)
	}
	var out2 map[string]any
	json.NewDecoder(resp.Body).Decode(&out2)
	if out2["error"] != "payload_too_large" {
		t.Errorf("error = %v", out2["error"])
	}
}

func TestChannelValidation(t *testing.T) {
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "bad channel!",
		"text":    "hello",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_channel" {
		t.Fatalf("invalid channel: %d %v", res.StatusCode, out)
	}

	res, out = doJSON(t, "GET", ts.URL+"/v1/posts?channel=bad%20channel!", nil, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_channel" {
		t.Fatalf("list invalid channel: %d %v", res.StatusCode, out)
	}
}

func TestTextFilter(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.Filters.Text.Deny = []string{"blocked phrase", "spam.example"}
	})
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "this contains a BLOCKED PHRASE",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("filtered text: %d %v", res.StatusCode, out)
	}

	res, out = doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "visit spam.example now",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("filtered url text: %d %v", res.StatusCode, out)
	}

	// Clean text passes.
	res, out = doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "perfectly fine message",
	}, nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("clean text: %d %v", res.StatusCode, out)
	}
}

func TestUsernameFilterOnPost(t *testing.T) {
	// The username filter runs on every post. The anonymous path honors
	// display_name as untrusted data and runs the same username filter.
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":      "chat/general",
		"text":         "hello",
		"display_name": "Administrator",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("post with reserved username: %d %v", res.StatusCode, out)
	}
}

func TestURLCountLimit(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.Posts.MaxURLsPerPost = 1
	})
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "https://a.example https://b.example",
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "post_rejected" {
		t.Fatalf("too many urls: %d %v", res.StatusCode, out)
	}
}

func TestRateLimit(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.RateLimit.PostsPerMinute = 2
	})
	for i := 0; i < 2; i++ {
		res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
			"channel": "chat/general",
			"text":    fmt.Sprintf("post %d", i),
		}, nil)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("post %d: %d %v", i, res.StatusCode, out)
		}
	}
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "too many",
	}, nil)
	if res.StatusCode != http.StatusTooManyRequests || out["error"] != "rate_limited" {
		t.Fatalf("rate limited: %d %v", res.StatusCode, out)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Error("missing Retry-After header")
	}
}

func TestPagination(t *testing.T) {
	_, ts := newTestServer(t, nil)
	for i := 0; i < 5; i++ {
		res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
			"channel": "chat/general",
			"text":    fmt.Sprintf("post %d", i),
		}, nil)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("post %d: %d %v", i, res.StatusCode, out)
		}
	}

	res, out := doJSON(t, "GET", ts.URL+"/v1/posts?channel=chat/general&limit=2", nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", res.StatusCode)
	}
	page1 := out["posts"].([]any)
	if len(page1) != 2 {
		t.Fatalf("limit=2 returned %d posts", len(page1))
	}
	last := page1[1].(map[string]any)

	// Cursor pagination: fetch the next page using the last post of
	// page 1 as the cursor. Pages must not overlap.
	before := int64(last["created_at"].(float64))
	beforeID := last["id"].(string)
	res, out = doJSON(t, "GET", fmt.Sprintf("%s/v1/posts?channel=chat/general&limit=10&before=%d&before_id=%s", ts.URL, before, beforeID), nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list before: %d", res.StatusCode)
	}
	page2 := out["posts"].([]any)
	if len(page2) != 3 {
		t.Fatalf("before cursor returned %d posts, want 3", len(page2))
	}
	seen := map[string]bool{}
	for _, p := range append(page1, page2...) {
		id := p.(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatalf("post %s appears on both pages", id)
		}
		seen[id] = true
	}

	// Newest first: created_at must be non-increasing.
	var prev int64 = 1 << 62
	for _, p := range append(page1, page2...) {
		ts := int64(p.(map[string]any)["created_at"].(float64))
		if ts > prev {
			t.Fatalf("posts not newest-first: %d after %d", ts, prev)
		}
		prev = ts
	}

	// Channel filter.
	res, out = doJSON(t, "GET", ts.URL+"/v1/posts?channel=other/channel", nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list other channel: %d", res.StatusCode)
	}
	if len(out["posts"].([]any)) != 0 {
		t.Error("other channel should be empty")
	}

	// Bad limit.
	res, out = doJSON(t, "GET", ts.URL+"/v1/posts?limit=0", nil, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("bad limit: %d %v", res.StatusCode, out)
	}

	// before_id without before.
	res, out = doJSON(t, "GET", ts.URL+"/v1/posts?before_id=p_1", nil, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("before_id without before: %d %v", res.StatusCode, out)
	}
}

func TestCORSAllowedOrigin(t *testing.T) {
	_, ts := newTestServer(t, nil)
	req, _ := http.NewRequest("GET", ts.URL+"/health", nil)
	req.Header.Set("Origin", "https://example.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("ACAO = %q, want https://example.com", got)
	}
	if !strings.Contains(res.Header.Get("Vary"), "Origin") {
		t.Errorf("Vary = %q, want it to include Origin", res.Header.Get("Vary"))
	}
}

func TestCORSRejectedOrigin(t *testing.T) {
	_, ts := newTestServer(t, nil)
	req, _ := http.NewRequest("GET", ts.URL+"/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty for disallowed origin", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	_, ts := newTestServer(t, nil)
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/v1/posts", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type, authorization")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("ACAO = %q", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("ACAM = %q", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Errorf("ACAH = %q", got)
	}
}

func TestCORSWildcard(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.CORS.Origins = []string{"*"}
	})
	req, _ := http.NewRequest("GET", ts.URL+"/health", nil)
	req.Header.Set("Origin", "https://anything.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
}

func TestSSEDelivery(t *testing.T) {
	_, ts := newTestServer(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/v1/stream?channel=chat/general", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}

	// Give the connection a moment to register.
	time.Sleep(100 * time.Millisecond)

	doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "live message",
	}, nil)

	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		n, err := res.Body.Read(buf)
		if err != nil && n == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		chunk := string(buf[:n])
		if strings.Contains(chunk, "event: post") && strings.Contains(chunk, "live message") {
			return
		}
	}
	t.Fatal("SSE event not received")
}

func TestSSEChannelFiltering(t *testing.T) {
	_, ts := newTestServer(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/v1/stream?channel=chat/other", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	time.Sleep(100 * time.Millisecond)

	doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "should not arrive",
	}, nil)

	// Read in a goroutine so we can time out cleanly.
	chunkCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := res.Body.Read(buf)
			if n > 0 {
				chunkCh <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case chunk := <-chunkCh:
		if strings.Contains(chunk, "should not arrive") {
			t.Fatal("post from other channel leaked into stream")
		}
	case <-time.After(1 * time.Second):
		// No data for the other channel: correct.
	}
}

func TestSQLitePersistence(t *testing.T) {
	cfg := config.Default()
	dbPath := filepath.Join(t.TempDir(), "persist.db")
	cfg.Storage.Database = dbPath
	cfg.CORS.Origins = []string{"https://example.com"}
	cfg.RateLimit.PostsPerMinute = 1000

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, db, log)
	ts := httptest.NewServer(srv.Handler())

	doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel": "chat/general",
		"text":    "persisted",
	}, nil)
	ts.Close()
	db.Close()

	// Reopen the same file: the post must still be there.
	db2, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	srv2 := New(cfg, db2, log)
	ts2 := httptest.NewServer(srv2.Handler())
	defer ts2.Close()

	res, out := doJSON(t, "GET", ts2.URL+"/v1/posts?channel=chat/general", nil, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", res.StatusCode)
	}
	posts := out["posts"].([]any)
	if len(posts) != 1 || posts[0].(map[string]any)["text"] != "persisted" {
		t.Fatalf("post not persisted: %v", out)
	}
}

func TestIdentityDisabled(t *testing.T) {
	_, ts := newTestServer(t, func(c *config.Config) {
		c.Identity.Browser = false
	})
	res, out := doJSON(t, "POST", ts.URL+"/v1/identity", nil, nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("identity creation with browser disabled: %d %v", res.StatusCode, out)
	}
}

func TestUnknownFieldsRejected(t *testing.T) {
	_, ts := newTestServer(t, nil)
	res, out := doJSON(t, "POST", ts.URL+"/v1/posts", map[string]any{
		"channel":   "chat/general",
		"text":      "hello",
		"bogus_key": true,
	}, nil)
	if res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_request" {
		t.Fatalf("unknown field: %d %v", res.StatusCode, out)
	}
}
