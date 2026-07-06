# FTP

The FTP front door is built on
[`fclairamb/ftpserverlib`](https://github.com/fclairamb/ftpserverlib). It's a
thin adapter: FTP commands are translated straight into `internal/storage`
calls, so FTP sees the exact same merged tree as the web UI and the API.

## Configuration

```toml
[ftp]
enabled       = true
listen        = ":2121"
tls           = false
passive_ports = "30000-30100"
```

See [configuration.md](configuration.md) for the authoritative key list.

## Authentication

FTP uses the same single credential as everything else. The username is
ignored (or any value accepted); the password is checked against
`auth.password_hash`. When `auth.enabled` is false, FTP is open.

> ⚠️ **Plain FTP sends the password in cleartext.** Only run plain FTP on a
> trusted LAN. For anything reachable from the internet, enable FTPS (below).

## FTPS (FTP over TLS)

Set `ftp.tls = true` to require explicit FTPS. The server reuses the same
certificate source as HTTPS:

- **Global mode** (`server.domain` set) → the autocert certificate.
- Otherwise → the manually configured certificate, if any.

Clients must use explicit TLS (`AUTH TLS`) and encrypt both the control and data
channels.

## Passive mode

Passive-mode data connections use the port range in `ftp.passive_ports`. When
the server is behind NAT or a firewall:

1. Open the whole range (e.g. `30000-30100/tcp`) to the server.
2. Keep the config range and the firewall rule identical.

Active mode is generally not needed and is discouraged for hosted setups.

## Mapping of FTP operations

| FTP command      | storage operation                       |
|------------------|-----------------------------------------|
| `LIST` / `NLST`  | merged directory listing                |
| `RETR`           | read (first root that has the path)     |
| `STOR`           | write (balanced onto emptiest root)     |
| `MKD`            | create dir on all roots                 |
| `DELE` / `RMD`   | delete from whichever root(s) hold it   |
| `RNFR` / `RNTO`  | rename (same root) or move (cross-root)  |

Balancing, path safety, and the merged view behave identically to the API —
because it's the same code underneath. See [storage.md](storage.md).
