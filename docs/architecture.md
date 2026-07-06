# Architecture

Goblin Cloud is a single Go binary (`gcloud`) that runs three front doors — a web
UI, a REST API, and an FTP server — over one shared file layer. There is no
database and no external runtime.

## Design goals

1. **One binary out.** Everything, including the web UI, is compiled and embedded.
   Dependencies are a build-time concern; the artifact is a single static file.
2. **No moving parts.** State lives on the filesystem. No DB to install, migrate,
   or corrupt.
3. **Config, not flags.** Behaviour is driven by one TOML file. CLI flags exist
   only for one-off admin actions.
4. **Same files, three protocols.** The web UI, API, and FTP are thin adapters
   over a common storage and auth core.

## Component layout

```
cmd/gcloud/           entry point + CLI dispatch (serve, set password, config …)
internal/config/      TOML load, defaults, validation
internal/auth/        bcrypt hashing, credential check, web session tokens
internal/storage/     the merged-view file layer (balancing, listing, I/O)
internal/httpapi/     REST handlers (also serves the web UI)
internal/web/         go:embed'd static assets (HTML/CSS/vanilla JS)
internal/ftpsrv/      ftpserverlib adapter mapping FTP ops onto storage
internal/server/      wires config → auth + storage → HTTP + FTP, runs both
```

### The core

Two packages are shared by every front door:

- **`internal/storage`** — the only thing that touches disk. Presents a single
  logical tree even though files may live across several physical roots. See
  [storage.md](storage.md).
- **`internal/auth`** — one credential (a bcrypt hash from config) gates all
  three protocols. The web UI trades the password for a session cookie; the API
  and FTP check the credential directly.

### The front doors

- **HTTP (`internal/httpapi` + `internal/web`)** — serves both the JSON REST API
  under `/api/*` and the embedded single-page web UI at `/`. See [api.md](api.md).
- **FTP (`internal/ftpsrv`)** — wraps `fclairamb/ftpserverlib`, translating FTP
  commands into `internal/storage` calls. Optional FTPS. See [ftp.md](ftp.md).

Both are started and supervised by `internal/server` from the same config.

## Request lifecycle (example: web upload)

1. Browser POSTs a multipart file to `/api/upload?path=/photos`.
2. `httpapi` checks the session cookie via `auth`.
3. `httpapi` hands the stream to `storage.Create("/photos/x.jpg")`.
4. `storage` picks the physical root with the most free space (respecting
   `min_free`), writes the file there, and returns.

An FTP `STOR` of the same path takes steps 3–4 identically — the balancing logic
lives in one place.

## Runtime modes

The same binary covers two deployment shapes, switched purely by config:

- **LAN mode** — `server.domain` empty. Plain HTTP, plain FTP. For a trusted
  home or office network.
- **Global mode** — `server.domain` set. Automatic HTTPS via Let's Encrypt
  (autocert), and FTPS available by toggling `ftp.tls`. For public hosting.

See [deployment.md](deployment.md) for both.

## Logging

All output goes through the standard library's `log/slog`. At startup the
process installs one handler on stderr, built from the `[log]` config (level and
text/json format), so every layer — HTTP, FTP, CLI — shares it. Level `debug`
turns on per-request access logs. systemd/journald handles capture and rotation.

## What's deliberately absent

- No database.
- No user accounts — a single shared password, by design.
- No plugin system, no per-file ACLs, no versioning. Scope stays small on
  purpose.
