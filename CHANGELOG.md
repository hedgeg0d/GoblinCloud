# Changelog

## 0.2.0 — logging and translations

### New

- **Configurable logging.** Log level (`debug`/`info`/`warn`/`error`) and format
  (`text`/`json`) are now set in the config file under `[log]`. HTTP access and
  structured server logs make operations transparent.
- **Internationalisation.** The web UI now ships with translations for 11
  languages: English, Russian, German, French, Spanish, Italian, Portuguese,
  Polish, Dutch, Japanese, and Chinese. The browser language is auto-detected;
  the language can be switched via `localStorage`.

## 0.1.0 — first release

The first cut of Goblin Cloud: one small binary that serves your files three
ways at once.

### Highlights

- **Three front doors, one set of files.** A web interface, a REST API, and an
  FTP server, all backed by the same storage.
- **Web UI.** Browse, upload (button or drag-and-drop, with a live progress
  bar), download, rename, create and delete folders. Works on mobile, ships with
  dark and light themes.
- **REST API.** Log in, list, upload, download (with range/resume), rename,
  delete — cookie or HTTP Basic auth.
- **FTP / FTPS.** Plain FTP for the LAN, explicit FTPS when hosted on a domain.
- **Storage across disks.** Point it at several folders and it spreads uploads
  onto whichever has the most free space, presenting one merged tree. No
  database.
- **One password, everywhere.** Set it once (`gcloud set password`); it guards
  the web UI, the API and FTP. Stored only as a bcrypt hash.
- **Runs anywhere on Linux.** LAN mode over plain HTTP, or a public domain with
  automatic HTTPS via Let's Encrypt. Ships with a systemd unit.
- **Just a binary.** No runtime, no dependencies, nothing to install alongside
  it.

### Under the hood

- Configuration in a single TOML file.
- CLI: `serve`, `set password`, `config init|path|check`, `storage status`,
  `version`.
- Test suite at ~88% coverage, race-clean.
