// Package server wires the HTTP API: routing, CORS, auth, rate
// limiting, and the public endpoints.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/matthewsawatzky/PicoPost/internal/config"
	"github.com/matthewsawatzky/PicoPost/internal/database"
	"github.com/matthewsawatzky/PicoPost/internal/filters"
	"github.com/matthewsawatzky/PicoPost/internal/identity"
	"github.com/matthewsawatzky/PicoPost/internal/posts"
	"github.com/matthewsawatzky/PicoPost/internal/ratelimit"
	"github.com/matthewsawatzky/PicoPost/internal/stream"
)

// Server is the PicoPost HTTP server.
type Server struct {
	cfg      config.Config
	db       *database.DB
	log      *slog.Logger
	ident    *identity.Store
	posts    *posts.Store
	hub      *stream.Hub
	limiter  *ratelimit.Limiter
	username *filters.Filter
	text     *filters.Filter
	http     *http.Server
}

// New builds a Server from configuration and an open database.
func New(cfg config.Config, db *database.DB, log *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		db:       db,
		log:      log,
		ident:    identity.NewStore(db.DB),
		posts:    posts.NewStore(db.DB),
		hub:      stream.NewHub(),
		limiter:  ratelimit.New(cfg.RateLimit.PostsPerMinute, time.Minute),
		username: filters.New(cfg.Filters.Username.Deny),
		text:     filters.New(cfg.Filters.Text.Deny),
	}
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/identity", s.handleIdentityCreate)
	mux.HandleFunc("GET /v1/identity", s.handleIdentityGet)
	mux.HandleFunc("PATCH /v1/identity", s.handleIdentityPatch)
	mux.HandleFunc("POST /v1/posts", s.handlePostCreate)
	mux.HandleFunc("GET /v1/posts", s.handlePostList)
	mux.HandleFunc("GET /v1/posts/{id}", s.handlePostGet)
	mux.HandleFunc("GET /v1/stream", s.handleStream)

	return s.cors(s.logRequests(s.limitBody(mux)))
}

// Serve starts the HTTP server and blocks until it shuts down.
func (s *Server) Serve() error {
	s.http = &http.Server{
		Addr:              s.cfg.Server.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s.log.Info("picopost listening", "addr", s.cfg.Server.Listen)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// --- middleware ---

// limitBody rejects requests whose body exceeds the configured maximum
// before any JSON parsing happens.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.Posts.MaxBodyBytes))
		}
		next.ServeHTTP(w, r)
	})
}

// logRequests logs method, path, status, and duration.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		s.log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", s.clientIP(r),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush keeps SSE working through the logging middleware.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// cors implements first-class CORS support. Origins are matched
// exactly; "*" is only used when explicitly configured.
func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range s.cfg.CORS.Origins {
		allowed[o] = true
	}
	allowAll := allowed["*"]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}

		if r.Method == http.MethodOptions {
			if origin == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if allowAll || allowed[origin] {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the client's IP address. Forwarded headers are only
// trusted when explicitly configured.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.RateLimit.TrustForwarded {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- error responses ---

// Public error codes. Keep them stable; they are part of the API.
const (
	ErrInvalidRequest  = "invalid_request"
	ErrPayloadTooLarge = "payload_too_large"
	ErrPostRejected    = "post_rejected"
	ErrRateLimited     = "rate_limited"
	ErrUnauthorized    = "unauthorized"
	ErrNotFound        = "not_found"
	ErrInvalidChannel  = "invalid_channel"
)

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleIdentityCreate(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Identity.Browser {
		writeError(w, http.StatusForbidden, ErrInvalidRequest)
		return
	}
	ident, key, err := s.ident.Create()
	if err != nil {
		s.log.Error("create identity", "error", err)
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"id":  ident.ID,
		"key": key,
	})
}

func (s *Server) handleIdentityGet(w http.ResponseWriter, r *http.Request) {
	ident, ok := s.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	s.writeIdentity(w, ident)
}

func (s *Server) handleIdentityPatch(w http.ResponseWriter, r *http.Request) {
	ident, ok := s.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrUnauthorized)
		return
	}

	var req struct {
		Name *string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		return
	}

	name := identity.NormalizeName(*req.Name)
	if err := identity.ValidateName(name); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if s.username.Rejects(name) {
		writeError(w, http.StatusBadRequest, ErrPostRejected)
		return
	}

	if err := s.ident.SetName(ident.ID, name); err != nil {
		s.log.Error("set identity name", "error", err)
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}
	ident.DisplayName = name
	s.writeIdentity(w, ident)
}

func (s *Server) writeIdentity(w http.ResponseWriter, ident *identity.Identity) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":           ident.ID,
		"display_name": ident.DisplayName,
		"created_at":   ident.CreatedAt,
		"updated_at":   ident.UpdatedAt,
	})
}

func (s *Server) handlePostCreate(w http.ResponseWriter, r *http.Request) {
	// Rate limit by client IP before doing any work.
	if ok, retry := s.limiter.Allow("post:" + s.clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeError(w, http.StatusTooManyRequests, ErrRateLimited)
		return
	}

	var req posts.NewPost
	if err := decodeJSON(r, &req); err != nil {
		if errors.Is(err, errPayloadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrPayloadTooLarge)
		} else {
			writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		}
		return
	}

	limits := posts.Limits{
		MaxTextBytes:     s.cfg.Posts.MaxTextBytes,
		MaxMetadataBytes: s.cfg.Posts.MaxMetadataBytes,
		MaxMetadataKeys:  s.cfg.Posts.MaxMetadataKeys,
		MaxKeyLength:     s.cfg.Posts.MaxKeyLength,
		MaxURLsPerPost:   s.cfg.Posts.MaxURLsPerPost,
	}
	if err := limits.Validate(&req); err != nil {
		if errors.Is(err, posts.ErrChannel) {
			writeError(w, http.StatusBadRequest, ErrInvalidChannel)
		} else {
			writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		}
		return
	}

	// Determine identity server-side. The client never supplies the
	// identity or display name in the payload.
	var identityID, displayName *string
	if ident, ok := s.authenticate(r); ok {
		identityID = &ident.ID
		if ident.DisplayName != "" {
			displayName = &ident.DisplayName
		}
	} else if !s.cfg.Identity.Anonymous {
		writeError(w, http.StatusUnauthorized, ErrUnauthorized)
		return
	} else if req.DisplayName != nil {
		// Anonymous posts may carry an untrusted display name.
		name := identity.NormalizeName(*req.DisplayName)
		if err := identity.ValidateName(name); err != nil {
			writeError(w, http.StatusBadRequest, ErrInvalidRequest)
			return
		}
		if name != "" {
			displayName = &name
		}
	}

	// Filters run before storage.
	if s.username.Rejects(displayNameOrEmpty(displayName)) {
		s.log.Warn("post rejected by username filter", "ip", s.clientIP(r))
		writeError(w, http.StatusBadRequest, ErrPostRejected)
		return
	}
	if s.text.Rejects(req.Text) {
		s.log.Warn("post rejected by text filter", "ip", s.clientIP(r))
		writeError(w, http.StatusBadRequest, ErrPostRejected)
		return
	}
	if s.cfg.Posts.MaxURLsPerPost > 0 && filters.CountURLs(req.Text) > s.cfg.Posts.MaxURLsPerPost {
		s.log.Warn("post rejected: too many URLs", "ip", s.clientIP(r))
		writeError(w, http.StatusBadRequest, ErrPostRejected)
		return
	}

	post, err := s.posts.Create(req.Channel, req.Text, req.Meta, identityID, displayName)
	if err != nil {
		s.log.Error("create post", "error", err)
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}

	s.hub.Publish("post", post)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post)
}

func (s *Server) handlePostList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	channel := q.Get("channel")
	if channel != "" {
		if err := posts.ValidateChannel(channel); err != nil {
			writeError(w, http.StatusBadRequest, ErrInvalidChannel)
			return
		}
	}

	limit := s.cfg.Posts.PageSize
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, ErrInvalidRequest)
			return
		}
		limit = n
	}

	var before int64
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, ErrInvalidRequest)
			return
		}
		before = n
	}
	beforeID := q.Get("before_id")
	if beforeID != "" && before == 0 {
		writeError(w, http.StatusBadRequest, ErrInvalidRequest)
		return
	}

	list, err := s.posts.List(posts.ListOptions{Channel: channel, Limit: limit, Before: before, BeforeID: beforeID})
	if err != nil {
		s.log.Error("list posts", "error", err)
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}
	if list == nil {
		list = []*posts.Post{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"posts": list})
}

func (s *Server) handlePostGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	post, err := s.posts.Get(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		s.log.Error("get post", "error", err)
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	if channel != "" {
		if err := posts.ValidateChannel(channel); err != nil {
			writeError(w, http.StatusBadRequest, ErrInvalidChannel)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, ErrInvalidRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Initial comment so proxies and clients see the stream is open.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	sub := s.hub.Subscribe()
	defer s.hub.Unsubscribe(sub)

	ctx := r.Context()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sub:
			if channel == "" || strings.Contains(string(msg), `"channel":"`+channel+`"`) {
				w.Write(msg)
				flusher.Flush()
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// authenticate resolves the Bearer identity key from the request.
func (s *Server) authenticate(r *http.Request) (*identity.Identity, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, false
	}
	key := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if key == "" {
		return nil, false
	}
	ident, err := s.ident.GetByKey(key)
	if err != nil {
		return nil, false
	}
	return ident, true
}

func displayNameOrEmpty(name *string) string {
	if name == nil {
		return ""
	}
	return *name
}

// decodeJSON parses a JSON request body. It returns errPayloadTooLarge
// for oversized bodies and errInvalidJSON for parse/validation errors.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errPayloadTooLarge
		}
		return errInvalidJSON
	}
	return nil
}

var (
	errPayloadTooLarge = errors.New("payload too large")
	errInvalidJSON     = errors.New("invalid json")
)
