# Configuration

All behaviour is driven by a single TOML file. This document is the **contract**:
the running program must accept exactly these keys, and any change to behaviour
must be reflected here first.

## File location

Resolved in this order:

1. `--config <path>` flag.
2. `$GOBLIN_CONFIG` environment variable.
3. `/etc/goblin/config.toml` (system default, used by the systemd unit).
4. `./config.toml` (working directory, handy for local runs).

`gcloud config path` prints the path that would be used. `gcloud config init`
writes a starter file to the resolved location.

## Full reference

```toml
[server]
# HTTP bind address. Used as-is in LAN mode.
listen = ":8080"

# Public domain. Empty = LAN mode (plain HTTP on `listen`).
# Set = global mode: automatic HTTPS on :443 via Let's Encrypt, and :80 is used
# for the ACME challenge + redirect to HTTPS.
domain = ""

# Contact email for the Let's Encrypt account. Required when `domain` is set.
autocert_email = ""

# Where issued certificates are cached on disk. Must be writable and persistent
# so certs survive restarts and aren't re-issued (rate limits apply).
autocert_cache = "/var/lib/goblin/certs"

[auth]
# When false, the web UI, API, and FTP are open with no login. Use only on a
# trusted LAN.
enabled = true

# bcrypt hash of the password. Never a plaintext password. Set it with:
#   gcloud set password
# which prompts interactively and writes the hash here.
password_hash = ""

[storage]
# One or more directories that hold the files. Uploads are balanced across them
# (see docs/storage.md). A single entry is perfectly fine.
paths = ["/mnt/disk1/goblin", "/mnt/disk2/goblin"]

# Roots with less free space than this are skipped when choosing where to write.
# Accepts KB / MB / GB / TB (powers of 1024). Reads still work from any root.
min_free = "1GB"

[ftp]
# Turn the FTP front door on or off.
enabled = true

# FTP control-connection bind address.
listen = ":2121"

# When true, require FTPS (explicit TLS). Reuses the autocert HTTPS certificate,
# so it requires global mode (server.domain set). Fails at startup otherwise.
tls = false

# Port range advertised for passive-mode data connections. Open this range on
# the firewall when hosting behind NAT.
passive_ports = "30000-30100"
```

## Notes on individual keys

### `server.domain` and TLS

- **Empty** → LAN mode. `server.listen` is used verbatim; no TLS.
- **Set** → global mode. The program binds `:443` for HTTPS and `:80` for the
  ACME HTTP-01 challenge and a redirect. `server.listen` is ignored.
  `autocert_email` becomes required, and `autocert_cache` must be persistent.

### `auth.password_hash`

The only credential in the system. It gates all three front doors. Editing it by
hand is discouraged — use `gcloud set password` so a correct bcrypt hash is
written. Rotating it invalidates existing web sessions.

### `storage.paths`

Order matters only for reads: on a name collision across roots, the first listed
root wins. Writes ignore order and go to whichever eligible root has the most
free space. See [storage.md](storage.md) for the full rules.

### `storage.min_free`

A safety margin so a disk is never filled completely. It only affects **write**
target selection. If every root is below `min_free`, writes fail with an
out-of-space error rather than filling a disk.

### `ftp.passive_ports`

Passive FTP needs a predictable data-port range through firewalls and NAT. Keep
this range open externally and matching between the config and the firewall rule.

## Validation

`gcloud config check` loads the file, applies defaults, and reports problems
without starting any service. It flags, among others:

- `domain` set but `autocert_email` empty.
- `storage.paths` empty, or a path that doesn't exist / isn't writable.
- Malformed `min_free` or `passive_ports`.
- `auth.enabled = true` with an empty `password_hash`.
