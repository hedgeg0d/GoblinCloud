<h1 align="center">🧌 Goblin Cloud</h1>

<p align="center">
  <b>Your files. Your server. Three ways in.</b><br>
  A tiny self-hosted file cloud — FTP, a clean REST API, and a web interface — in one small binary.
</p>

---

## What is it?

Goblin Cloud is a small, no-nonsense file server you run on your own machine.
Drop a single binary on a box, point it at a folder (or several), and you get
**three ways to reach your files at once**:

- 🌐 **Web interface** — upload, download, rename, and organise from any browser,
  phone included.
- 🔌 **REST API** — script it, back things up to it, wire it into your own apps.
- 📁 **FTP** — mount it in your file manager or classic FTP client.

All three see the same files. No database to babysit, no containers to wrangle,
no cloud account. Just your server and your stuff.

## Why you might like it

- **One binary.** No runtime, no dependencies. Download, run, done.
- **Spread across disks.** Give it several folders and it spreads uploads across
  them automatically, always writing to wherever there's the most room.
- **Locks the door.** Set one password and the web UI, the API, and FTP are all
  protected. Nothing is stored in plain text.
- **Home or hosted.** Run it on the LAN for the household, or put it on a domain
  with automatic HTTPS and open it to the world — your call.
- **Ships ready for Linux.** Comes with a systemd service so it starts on boot
  and stays up.

## Quick start

```sh
# 1. Create a starter config
gcloud config init

# 2. Set your password (typed in, never shown)
gcloud set password

# 3. Run it
gcloud serve
```

Open `http://localhost:8080`, log in, and you're in.

Point an FTP client at the same machine on port `2121`, or talk to the API — the
files are identical whichever door you use.

## Screenshots

> _Coming soon._

## License

See [LICENSE](LICENSE).

---

<p align="center"><i>Small. Sharp. Yours.</i></p>
