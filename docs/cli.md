# CLI reference

The binary is `gcloud`. It runs the server and handles a handful of admin tasks.
Everything else is configuration — see [configuration.md](configuration.md).

## Synopsis

```
gcloud [--config <path>] <command> [args]
```

`--config` overrides config resolution for any command (see
[configuration.md](configuration.md) for the resolution order).

## Commands

### `gcloud serve`

Start all enabled front doors (web/API always; FTP if `ftp.enabled`) using the
resolved config. This is the default command — running `gcloud` with no command
is equivalent to `gcloud serve`. This is what the systemd unit runs.

Runs in the foreground and logs to stdout/stderr; systemd captures the journal.

### `gcloud set password`

Prompt for a new password interactively (input is never echoed, entered twice to
confirm), hash it with bcrypt, and write the hash into `auth.password_hash` in
the config file. The plaintext is never stored or logged.

```
$ gcloud set password
New password:
Confirm password:
Password updated.
```

### `gcloud config init`

Write a starter `config.toml` with sane defaults and inline comments to the
resolved config path. Refuses to overwrite an existing file unless `--force` is
given.

### `gcloud config path`

Print the config path that would be used, given the current flags and
environment. Prints nothing else — handy in scripts.

### `gcloud config check`

Load and validate the config without starting anything. Exit code `0` if valid,
non-zero with a list of problems otherwise. See the validation list in
[configuration.md](configuration.md).

### `gcloud storage status`

Print a per-root report: path, total size, free space, whether it's currently
eligible for writes (i.e. above `min_free`). Useful for confirming balancing
behaviour and spotting a full or missing disk.

```
$ gcloud storage status
ROOT                  TOTAL     FREE      WRITABLE
/mnt/disk1/goblin     500 GB    488 GB    yes
/mnt/disk2/goblin     500 GB    12 GB     yes
```

### `gcloud version`

Print the version and exit. `gcloud --version` and `gcloud -v` are aliases.

```
$ gcloud version
gcloud 0.1.0
```

The version is baked into the binary. Release builds may stamp a git tag over it
with `-ldflags "-X main.version=…"` (see [build.md](build.md)).

## Exit codes

| Code | Meaning                                   |
|------|-------------------------------------------|
| 0    | Success                                   |
| 1    | Generic error                             |
| 2    | Config invalid or not found               |
| 3    | Runtime/bind error while serving          |
