# Building

Goblin Cloud builds to a single static binary. The web UI is embedded at compile
time with `go:embed`, so there are no separate asset files to ship.

## Requirements

- Go 1.26 or newer.
- No C toolchain — the build is pure Go (`CGO_ENABLED=0`), which is what keeps the
  binary static and portable.

## Build

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gcloud ./cmd/gcloud
```

- `CGO_ENABLED=0` — static binary, no libc dependency.
- `-trimpath` — strip local filesystem paths from the binary.
- `-ldflags "-s -w"` — drop the symbol table and DWARF for a smaller file.

The result is a single `gcloud` you can copy to any Linux box of the same
architecture.

## Cross-compiling

Go cross-compiles without extra tooling. For a typical amd64 Linux server from
any host:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o dist/gcloud-linux-amd64 ./cmd/gcloud
```

Swap `GOARCH=arm64` for ARM servers and single-board machines.

## Version stamping

Version info is injected at build time via `-ldflags -X`:

```sh
VERSION=$(git describe --tags --always)
go build -ldflags "-s -w -X main.version=$VERSION" -o gcloud ./cmd/gcloud
```

`gcloud version` prints whatever was stamped in.

## Embedded assets

The web UI (`internal/web`) is pulled in with `go:embed`. Editing the HTML/CSS/JS
and rebuilding is all that's needed — there is no separate bundler or asset
pipeline. Nothing about the UI lives outside the binary at runtime.

## Dependencies

Kept intentionally small. The notable ones:

- `github.com/BurntSushi/toml` — config parsing.
- `github.com/fclairamb/ftpserverlib` — the FTP server.
- `golang.org/x/crypto/bcrypt` — password hashing.
- `golang.org/x/crypto/acme/autocert` — automatic HTTPS.

All are compiled in; the shipped artifact is still one file.
