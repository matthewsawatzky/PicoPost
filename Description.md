# PicoPost

**PicoPost is a tiny, self-hosted text service for websites and lightweight applications.**

It provides a simple way for a website to send, store, retrieve, and optionally stream small text-based records without needing a full backend, user system, or large application stack.

PicoPost is designed around one simple idea:

> Small pieces of text should not require a large server.

A PicoPost instance should be easy to install, easy to understand, and easy to remove. Ideally, someone should be able to download a single binary, create a small configuration file, point a domain or reverse proxy at it, add one small JavaScript client to their website, and have a working text backend.

## What PicoPost can power

PicoPost is not specifically a chat server.

The same underlying service should be useful for things such as:

* comments
* reviews and star ratings
* small chatrooms
* guestbooks
* community boards
* feedback forms
* suggestion boxes
* Q&A
* simple polls and reactions
* status feeds
* changelogs
* activity feeds
* agent-to-human messages
* human-to-agent messages
* temporary message rooms
* simple support or request queues

These should not become separate large subsystems.

They are different ways of using the same small set of primitives: **channels, text records, metadata, identities, and streams.**

## Small records

PicoPost is intended for small text payloads rather than files, media, or large structured documents.

A record might contain:

```json
{
  "text": "Really useful project.",
  "meta": {
    "name": "Mat",
    "stars": 5
  }
}
```

Metadata should remain small and flexible enough for applications to attach information such as:

```text
name
stars
reply target
reaction
type
status
category
```

The server should enforce configurable limits, with deployments commonly using maximum payloads around 1 KB.

PicoPost should deliberately avoid growing into file storage, image hosting, or a general-purpose database API.

## Channels

Records can belong to channels.

For example:

```text
reviews/homepage
comments/article-42
chat/general
feedback/site
agent/build-status
```

Channels allow one PicoPost instance to support several independent uses without requiring separate applications.

Channels may eventually have their own limits, permissions, retention policies, or validation rules.

## Lightweight identity

PicoPost should not require a traditional user-account system.

For public applications, three levels of identity are enough:

**Anonymous**

No persistent identity is required. The supplied display name or metadata is untrusted.

**Browser identity**

PicoPost can automatically issue a random visitor identity and store its key in the browser. This provides continuity across visits without requiring an email address, password, signup page, or account database.

A browser identity could support things such as:

```text
persistent nickname
editing your own posts
deleting your own posts
one vote per visitor
one review per visitor
basic blocking or rate limiting
```

The browser identity is intentionally lightweight. Losing browser storage may mean losing the identity.

**Trusted integration**

Applications, agents, or existing websites may eventually provide a trusted signed identity or server-issued credential when stronger authentication is actually needed.

PicoPost itself should avoid becoming a complete identity provider.

## Easy browser integration

The browser client should hide most of the networking and identity work.

A site owner should eventually be able to add something close to:

```html
<script
  src="https://posts.example.com/picopost.js"
  data-channel="comments">
</script>
```

The PicoPost client can then automatically:

```text
discover the PicoPost server
create or restore a browser identity
attach identity information to requests
send posts
retrieve posts
subscribe to new posts
handle errors and limits
```

Developers should also be able to use the JavaScript API directly and build completely custom interfaces.

PicoPost should provide infrastructure, not dictate the appearance of the website using it.

## CORS should be easy

Because PicoPost will commonly run on a different domain or subdomain from the website using it, cross-origin browser access is a core part of the product rather than an afterthought.

A deployment should be able to specify allowed websites:

```toml
allowed_origins = [
  "https://example.com"
]
```

PicoPost should handle CORS and browser preflight requests automatically.

The website owner should not need to understand the details of CORS just to embed PicoPost.

## Simple moderation

Public text endpoints inevitably attract spam and abuse, so PicoPost should include basic moderation without becoming a moderation platform.

Useful built-in controls include:

```text
maximum message size
maximum metadata size
rate limits
duplicate detection
maximum URL count
blocked words or strings
reserved usernames
field-specific filters
basic identity bans
```

Rules should be configurable independently for fields such as usernames and message text.

Filtering should primarily happen when data is submitted so rejected content is never stored.

The goal is predictable, inexpensive moderation rather than AI moderation or complex policy engines.

## Administration should stay separate

The public PicoPost server should not need a large administration dashboard or privileged web account system.

Administrative work can be handled by separate tooling.

For example, an optional utility could inspect the PicoPost database and provide a local-only interface for:

```text
viewing records
deleting records
blocking identities
editing filters
viewing channels
checking server activity
```

Keeping this separate prevents administration features from increasing the complexity and attack surface of the public server.

## Disposable by design

PicoPost should work well both as persistent infrastructure and as something temporary.

Storage modes could include persistent SQLite-backed storage as well as temporary in-memory storage.

Retention could be constrained by:

```text
maximum records
maximum age
channel-specific TTL
```

A temporary chatroom, build log, event feed, or agent message channel should be able to disappear automatically when it is no longer useful.

## Deployment philosophy

A normal PicoPost deployment should remain extremely small:

```text
one binary
one configuration file
one small database
one optional JavaScript client
```

A reverse proxy such as Caddy can handle domains, HTTPS, and certificates.

PicoPost itself should concentrate on:

```text
HTTP API
small records
channels
identity
validation
moderation
storage
streaming
CORS
```

It should not attempt to become a reverse proxy, certificate manager, CMS, social network, authentication platform, or general application framework.

## Development principle

The main test for adding a feature should be:

> Does this make small text interactions easier without making PicoPost substantially less small?

Features that naturally compose with the existing primitives are good candidates.

Features that require entire new subsystems should usually live outside PicoPost or be implemented as optional companion tools.

PicoPost should remain something a developer can understand quickly, deploy in minutes, and trust to quietly perform one small job.

**PicoPost is tiny text infrastructure for the web.**
