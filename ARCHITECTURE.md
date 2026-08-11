# PicoPost Architecture

This document explains how the prototype is put together so another
developer can continue the project without the original author present.

## Principles

- **One process, one SQLite file.** No Postgres, no Redis, no Docker
  runtime, no microservices.
- **Small composable primitives.** Chat, comments, reviews, guestbooks,
  agent messages are all just posts on channels. There is no per-feature
  code.
- **Go standard library first.** The only external dependencies are
  `BurntSushi/toml` (config parsing) and `modernc.org/sqlite` (pure-Go
  SQLite).
- **The server decides.** Timestamps, identity IDs, and display names for
  authenticated posts always come from the server, never from the client
  payload.

## Repository layout

```text
cmd/picopost/     CLI: serve, version, check
cmd/demo/         tiny static file server for the demo site
internal/config/  TOML loading, defaults, validation
internal/database/SQLite open/close + embedded migration runner + counts
internal/identity/browser identities (creation, hashing, name storage)
internal/posts/   post model, channel/metadata validation, post store
internal/filters/ compiled deny lists, URL counting
internal/ratelimit/in-memory fixed-window per-key limiter
internal/stream/  in-process SSE broadcaster (hub)
internal/server/  HTTP routing, middleware (CORS, logging, body limits),
                  and all handlers
migrations/       SQL migrations (embedded into the binary)
web/picopost.js   browser client
web/demo/         development demo site
dev               developer utility script
```

Each package corresponds to one real responsibility. Nothing exists just
for the sake of architecture.

## Data model

Two tables plus a migration bookkeeping table:

```sql
posts(id, channel, text, metadata_json, identity_id?, display_name?,
      created_at)
identities(id, key_hash, display_name, created_at, updated_at)
schema_migrations(version, applied_at)
```

- `metadata_json` stores arbitrary JSON; there are no columns for specific
  metadata keys. Limits are enforced in `internal/posts` before storage.
- `identities.key_hash` is the SHA-256 of the secret key. The plaintext key
  is returned to the client exactly once at creation and is never stored or
  logged.
- Posts are listed `ORDER BY created_at DESC, id DESC`; pagination uses
  `before` + `before_id` cursors so pages never overlap, even when several
  posts share a `created_at` second.

## Migrations

SQL files live in `migrations/` and are embedded into the binary via
`//go:embed` (`internal/database/migrations/` is the same file, mirrored
for embedding). `database.Open` creates the schema_migrations table if
needed, applies missing migrations in filename order inside a transaction,
and records each applied version. To add a schema change: create
`migrations/0002_*.sql`, copy it into `internal/database/migrations/`, and
it applies automatically on next startup.

## Request flow (post creation)

```
HTTP POST /v1/posts
  ├─ limitBody middleware: MaxBytesReader rejects > max_body_bytes with 413
  ├─ logRequests middleware: records method/path/status/duration/IP
  ├─ cors middleware: adds CORS headers, handles OPTIONS preflight
  ├─ rate limiter: per-IP, in-memory, 429 + Retry-After
  ├─ decode JSON (unknown fields rejected)
  ├─ validate channel, text size, metadata size/keys
  ├─ authenticate Bearer key → identity (else anonymous path)
  │     anonymous posts may carry an untrusted display_name;
  │     authenticated posts use the identity's stored name
  ├─ filters: username deny list, text deny list, URL count
  │     any match → 400 post_rejected (non-specific on purpose)
  ├─ store post (prepared statements only)
  └─ hub.Publish("post", post) → all SSE subscribers
```

The stream handler (`GET /v1/stream`) subscribes to the hub, filters
events by channel, and writes `event: post` frames. It flushes a keep-alive
comment every 20 seconds and ends when the client disconnects.

## Identity flow

```
POST /v1/identity   → generate id_ + pi_ keys (crypto/rand), store key hash
GET  /v1/identity   → Bearer pi_... → lookup by hash → return profile
PATCH /v1/identity  → Bearer → validate/normalize name → store
```

The JS client keeps `{id, key}` in `localStorage` and attaches
`Authorization: Bearer pi_...` to every request. Keys are high-entropy
random strings; hashing protects the database if it leaks. This is a
browser identity, not a user account: no passwords, no recovery, no admin.

## SSE flow

`internal/stream.Hub` is an in-process map of channels. `Publish` JSON
encodes the event and non-blockingly writes to each subscriber channel;
slow subscribers are dropped rather than blocking post creation. This is
deliberately single-process: scale-out would replace the hub with a
message broker, but the prototype does not need that.

## CORS

`internal/server` computes the allow list once at construction. Every
response with a matching `Origin` gets `Access-Control-Allow-Origin` plus
`Vary: Origin`. `OPTIONS` preflight returns 204 with the allowed methods
and headers. `["*"]` is honored only when explicitly configured. CORS is a
convenience for browsers, never treated as security: the API is public by
design, and rate limiting/filters are the actual abuse protections.

## Security basics

- Request bodies capped by `http.MaxBytesReader` before JSON parsing.
- `crypto/rand` identity keys; only SHA-256 hashes stored.
- Constant-time key comparison.
- Prepared/parameterized queries throughout.
- Strict JSON decoding (unknown fields rejected, no float timestamps).
- `X-Forwarded-For` only trusted when `trust_forwarded = true`.
- HTTP server timeouts set; graceful shutdown on SIGINT/SIGTERM.
- No secrets, Authorization headers, or request bodies in logs.

## Logging

`slog` text output to stderr. Request lines (method, path, status,
duration, IP), startup/shutdown, rejected posts, and rate-limit events.
Identity keys and request bodies are never logged.

## Where future functionality should live

- **New API endpoints** → `internal/server` (routing + handlers).
- **New validation rules** → `internal/posts` (limits) and
  `internal/filters` (content).
- **New storage** → new tables via `migrations/`, queries in the relevant
  store package.
- **Admin tooling** → deliberately separate from the public server; a
  future binary can reuse `internal/database` directly.
- **Scale-out SSE** → replace `internal/stream` with a broker-backed hub;
  the handler and client stay the same.
- **Moderation** → extend `internal/filters`; the post-rejection path
  already exists.
