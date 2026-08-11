# PicoPost

**Tiny text infrastructure for the web.**

PicoPost is a small, self-hosted text backend for websites. It provides a
small HTTP API that websites can use for comments, reviews, small chatrooms,
guestbooks, feedback, simple community boards, agent messages, and other
lightweight text-based interactions.

A deployment is three files:

```text
picopost
picopost.toml
picopost.db
```

## What PicoPost is

- A small HTTP API for posts (text + optional JSON metadata) organized by
  channel.
- Lightweight, persistent, browser-scoped identities (deliberately not user
  accounts).
- Live updates over Server-Sent Events (SSE).
- First-class CORS support for sites hosted on other domains.
- Simple server-side filters, size limits, and per-IP rate limiting.
- One process, one SQLite database, no Docker, no Redis, no Postgres.

## What PicoPost is not

Not a CMS, not a social network, not a complete forum, not an
authentication service, not a database platform, not a moderation platform,
not a messaging platform, and not a frontend framework. There are no user
accounts, no OAuth, no email, no file uploads, and no images.

---

## Building

Requires Go 1.22+ (tested with Go 1.26). No C compiler needed — SQLite is
pure Go via `modernc.org/sqlite`.

```bash
go build -o bin/picopost ./cmd/picopost
```

or:

```bash
./dev build
```

The only external Go dependencies are `github.com/BurntSushi/toml`
(configuration) and `modernc.org/sqlite` (SQLite).

## Installing

One command (downloads the latest release, or builds from source if no
prebuilt binary matches your platform):

```bash
curl -fsSL https://raw.githubusercontent.com/matthewsawatzky/PicoPost/main/install.sh | sh
```

The binary lands in `~/.local/bin` (or `~/bin` if it exists).

## Running

```bash
cp config/picopost.example.toml picopost.toml
# edit picopost.toml
./bin/picopost serve
```

Or point at a specific config:

```bash
./bin/picopost serve --config /etc/picopost/picopost.toml
```

Other CLI commands:

```bash
./bin/picopost version   # print version
./bin/picopost check     # validate config + database, then exit
```

## Configuration

Everything is in a single `picopost.toml`. See
[`config/picopost.example.toml`](config/picopost.example.toml) for a fully
commented example. All values shown there are the defaults.

Invalid configurations fail loudly at startup; unknown keys are rejected.

Key sections:

| Section        | What it controls                                              |
| -------------- | ------------------------------------------------------------- |
| `[server]`     | listen address                                                |
| `[storage]`    | SQLite database path                                          |
| `[cors]`       | allowed origins (`["*"]` for deliberately public mode)        |
| `[posts]`      | body/text/metadata size limits, metadata key limits, URL count |
| `[identity]`   | enable browser identities / anonymous posting                 |
| `[rate_limit]` | posts per minute per IP, forwarded-IP trust                   |
| `[filters]`    | deny lists for usernames and text                             |

## Using `./dev`

The `./dev` script starts the API plus a local demo site. No Docker needed.

```bash
./dev start       # build, create dev config if missing, start API + demo
./dev stop        # stop exactly the processes ./dev start created
./dev restart     # stop, then start
./dev status      # what is running, health, database state
./dev logs        # follow all logs (or: ./dev logs api / ./dev logs demo)
./dev db clean    # reset the dev database (confirms first; --yes skips)
./dev db info     # path, size, schema version, post/identity counts
./dev test        # run the Go test suite
./dev build       # build ./bin/picopost
```

After `./dev start`:

- API: <http://127.0.0.1:8080>
- Demo: <http://127.0.0.1:8081> — open the demo page in two windows to see
  SSE live updates.

`./dev db clean` only ever touches `./data/dev.db`, never an arbitrary
configured database.

## API

All JSON. Errors use a small consistent format:

```json
{ "error": "post_rejected" }
```

Error codes: `invalid_request`, `payload_too_large`, `post_rejected`,
`rate_limited`, `unauthorized`, `not_found`, `invalid_channel`.

| Endpoint            | Description                                        |
| ------------------- | -------------------------------------------------- |
| `GET /health`       | liveness check                                     |
| `POST /v1/identity` | create a browser identity → `{id, key}` (key shown once) |
| `GET /v1/identity`  | fetch own identity (Bearer auth)                   |
| `PATCH /v1/identity`| set display name: `{"name": "Mat"}` (Bearer auth)  |
| `POST /v1/posts`    | create a post                                      |
| `GET /v1/posts`     | list posts (see below)                             |
| `GET /v1/posts/{id}`| fetch one post                                     |
| `GET /v1/stream`    | SSE live stream                                    |

Authenticate with `Authorization: Bearer pi_...`.

### Creating a post

```json
{
  "channel": "reviews/homepage",
  "text": "Really nice.",
  "meta": { "stars": 5 }
}
```

The server adds `id`, `created_at`, and (for authenticated identities) the
`identity_id` and `display_name`. **Timestamps and identity fields from the
client are never trusted.** Anonymous posts may include a `display_name`,
which is treated as untrusted display data. For authenticated posts, the
client-supplied `display_name` is ignored.

### Listing posts

```text
GET /v1/posts?channel=chat/general&limit=50&before=1786460000&before_id=p_...
```

- `channel` — required filtering per channel (validate the format: `a-z`,
  `A-Z`, `0-9`, `-`, `_`, `/`).
- `limit` — page size (default from config, max 200).
- `before` / `before_id` — cursor from the last post of the previous page
  (`created_at` plus `id` tie-breaker). Pages never overlap, even for posts
  created within the same second.

Results are newest-first.

### SSE

```text
GET /v1/stream?channel=chat/general
```

Delivers `event: post` messages with the same JSON shape as the create
response. Keep-alive comments are sent every 20 seconds. The stream is an
in-process broadcaster — PicoPost currently runs as one process.

### CORS

CORS is a first-class feature because the site and the API are usually on
different domains:

```toml
[cors]
origins = ["https://example.com", "http://localhost:5173"]
```

Preflight (`OPTIONS`), `Access-Control-Allow-Origin`, `-Methods`,
`-Headers`, and `Vary: Origin` are handled automatically. `*` is only
allowed if explicitly configured. CORS is not security — anyone can still
call the API directly.

### Browser identities

On first use, the JS client calls `POST /v1/identity`. The server returns a
public ID (`id_...`) and a secret key (`pi_...`). The key is shown exactly
once, stored in `localStorage` by the client, and only the SHA-256 hash of
the key is stored server-side. Identity keys are not user accounts.

### Filters and abuse protection

`[filters.username]` and `[filters.text]` deny lists run **before a post is
stored**, using case-insensitive substring matching. Usernames are checked
with capitalization and whitespace normalized. Matched content is rejected
with a non-sensitive `post_rejected` error — clients never learn which rule
triggered.

Configurable protections: maximum request body (413 when exceeded), maximum
text size, maximum metadata size/keys, maximum URLs per post, and a
per-IP rate limit (`[rate_limit] posts_per_minute`). Rate-limit state is
in-memory and resets on restart. Forwarded-IP headers are only trusted when
`trust_forwarded = true` is explicitly set.

## Browser client

`web/picopost.js` is a tiny dependency-free client. Use it via script tag:

```html
<script src="https://posts.example.com/picopost.js" data-channel="comments"></script>
```

or explicitly:

```javascript
PicoPost.init({ server: "http://localhost:8080", channel: "chat/general" });

PicoPost.send("Hello!", { stars: 5 });
PicoPost.list().then(function (posts) { ... });
PicoPost.subscribe(function (post) { ... });   // SSE live updates

PicoPost.identity.get();
PicoPost.identity.setName("Mat");
PicoPost.identity.reset();
```

The client creates an identity when needed, persists the key in
`localStorage`, restores it on later loads, sends the `Authorization`
header automatically, and lets the browser handle SSE reconnection.

## Development

See [ARCHITECTURE.md](ARCHITECTURE.md) for the component layout, request
flow, and where future features should live.

Workflow:

```bash
./dev start      # API + demo site
# edit code, hit save
./dev logs       # watch the API logs
go test ./...    # or: ./dev test
./dev restart
./dev db clean   # start over from an empty database
./dev stop
```

## Public demo site

The `web/site/` folder is a static site (no build step) ready for Cloudflare
Pages or any static host. It contains:

- `index.html` — a live demo with five widgets on one page: chatroom,
  comments, reviews, an agent-style help desk, and a guestbook, all built
  on the same post API with SSE live updates.
- `usage.html` — how to integrate PicoPost into your own site.
- `setup.html` — how to install and configure the server, including the
  install script and Cloudflare Pages deployment.

To deploy to Cloudflare Pages: build command `none`, output directory
`web/site`. Or: `npx wrangler pages deploy web/site`.

## Tests

```bash
go test ./...
```

The suite covers post creation, size limits, channel and metadata
validation, identity lifecycle, filtering, CORS (allowed, rejected,
preflight, wildcard), rate limiting, pagination, SQLite persistence, and
SSE delivery — mostly against real HTTP handlers.

## License

[MIT](LICENSE)
