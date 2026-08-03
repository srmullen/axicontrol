# Backend stack: Go, stdlib-first

The backend is Go, using the standard library wherever it covers the need rather than reaching for a framework: `net/http` (Go 1.22+ routing) for the API, `database/sql` with hand-written queries against the embedded SQLite datastore ([ADR-0009](./0009-embedded-sqlite-datastore.md)), `log/slog` for structured logging, and plain `os.Getenv` for the small amount of runtime config (device path, PV mount points, ports). This matches the app's actual shape — a single process, single writer, low request volume, small schema — where a fuller framework or ORM would be machinery bought for a problem that doesn't exist here.

Two deliberate departures from "add nothing": the SQLite driver is `modernc.org/sqlite`, a pure-Go implementation, chosen specifically so the backend compiles to a static binary with no cgo/C-toolchain requirement — this is what makes the [distroless container image](./0013-build-ci-and-publish.md) possible. And schema migrations use `golang-migrate` rather than an idempotent `CREATE TABLE IF NOT EXISTS` on boot, since the schema is expected to change shape over time and a versioned migration history is cheap to adopt now versus retrofitting once the first real migration is needed.

Tests use stdlib `testing` with `testify` for assertions — the one place we chose a small ergonomics win (clearer failure output, less boilerplate) over zero-dependency purity, since it's a near-universal pairing in Go and doesn't touch runtime code.

The upload-time SVG sanitization pass required by [ADR-0010](./0010-svg-sanitization-and-img-preview.md) uses `github.com/beevik/etree`, an XML tree library, rather than stdlib `encoding/xml` or the HTML sanitizer `bluemonday`. `encoding/xml` is token-stream based, which makes "walk the tree, drop disallowed nodes, keep the rest" meaningfully more code than a tree library gives for free. `bluemonday` was rejected outright despite being the most familiar name here: it's built on an HTML5 parser, not a strict XML parser, and risks mangling SVG's self-closing tags and namespaced attributes — corruption `axicli` would then fail to parse as valid geometry. `etree` gives a proper parse → allowlist-walk → re-serialize pipeline that preserves SVG structure.

## Considered Options

- `mattn/go-sqlite3` (cgo binding to the real SQLite library) — rejected: faster, but requires cgo, which would have undercut the static-binary/simple-cross-compile benefit that motivated choosing Go in the first place.
- `sqlc` or an ORM (GORM, ent) for data access — rejected: the schema (Job, Pass, Preset, Device Config, webhook subscriptions) is small enough that hand-written `database/sql` queries stay manageable without codegen or ORM machinery.
- A router/framework (chi, Echo, Gin) — rejected: the API surface (CRUD + upload + one SSE endpoint) doesn't need anything stdlib's `net/http` lacks.
- `bluemonday` for SVG sanitization — rejected: HTML-parser-based, risks corrupting valid SVG structure that `axicli` needs to parse correctly.
