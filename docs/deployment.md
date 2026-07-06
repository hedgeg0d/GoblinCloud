# Deployment

Goblin Cloud is one binary plus one config file. This covers running it for real
on Linux, in both LAN and internet-facing setups.

## Install the binary

Drop the `gcloud` binary somewhere on `PATH`, e.g. `/usr/local/bin/gcloud`.
It's static — no runtime to install. See [build.md](build.md) to produce it.

## First run

```sh
sudo gcloud config init                 # writes /etc/goblin/config.toml
sudo -e /etc/goblin/config.toml         # set storage paths, domain, etc.
sudo gcloud set password                # writes the bcrypt hash
sudo gcloud config check                # validate before starting
```

## LAN mode

For a home or office network. Leave `server.domain` empty.

```toml
[server]
listen = ":8080"
domain = ""

[ftp]
enabled = true
listen  = ":2121"
tls     = false
```

- Web UI: `http://<server-ip>:8080`
- FTP: `<server-ip>:2121`

Plain HTTP and plain FTP are fine on a network you trust.

## Global mode (domain + HTTPS)

For public hosting. Set `server.domain`; the program obtains and renews a
Let's Encrypt certificate automatically.

```toml
[server]
domain         = "files.example.com"
autocert_email = "you@example.com"
autocert_cache = "/var/lib/goblin/certs"

[ftp]
enabled = true
tls     = true          # FTPS — never plain FTP on the internet
```

Requirements:

- DNS `A`/`AAAA` for the domain points at the server.
- Ports **80** and **443** reachable from the internet. Port 80 handles the ACME
  challenge and redirects to HTTPS; port 443 serves the site.
- `autocert_cache` is persistent and writable, so certs survive restarts and
  aren't needlessly re-issued (Let's Encrypt rate limits apply).
- For FTPS, open the `ftp.listen` port and the `passive_ports` range.

## systemd service

Goblin Cloud ships with a unit so it starts on boot and restarts on failure.
Run it as a dedicated unprivileged user with a hardened sandbox.

```ini
# /etc/systemd/system/goblin-cloud.service
[Unit]
Description=Goblin Cloud
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=goblin
Group=goblin
ExecStart=/usr/local/bin/gcloud serve --config /etc/goblin/config.toml
Restart=on-failure
RestartSec=5

# Let it write only where it needs to
StateDirectory=goblin
ReadWritePaths=/var/lib/goblin /mnt/disk1/goblin /mnt/disk2/goblin

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true

# Binding :80/:443 as a non-root user
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

Enable it:

```sh
sudo useradd --system --home /var/lib/goblin --shell /usr/sbin/nologin goblin
sudo systemctl daemon-reload
sudo systemctl enable --now goblin-cloud
sudo journalctl -u goblin-cloud -f      # logs
```

> Adjust `ReadWritePaths` to match your actual `storage.paths`, and drop
> `AmbientCapabilities`/`CapabilityBoundingSet` if you only bind high ports
> (LAN mode).

## Firewall checklist

| Mode    | Open ports                                             |
|---------|--------------------------------------------------------|
| LAN     | `8080` (web/API), `2121` + passive range (FTP)         |
| Global  | `80`, `443` (web/API + ACME); FTPS port + passive range |

## Upgrades

Replace the binary and restart:

```sh
sudo systemctl stop goblin-cloud
sudo install -m0755 gcloud /usr/local/bin/gcloud
sudo systemctl start goblin-cloud
```

Config and files are untouched by an upgrade.
