package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Template is the commented starter config written by `gcloud config init`.
const Template = `# Goblin Cloud configuration.

[server]
# HTTP bind address. Used as-is in LAN mode.
listen = ":8080"

# Public domain. Empty = LAN mode (plain HTTP on ` + "`listen`" + `).
# Set = global mode: automatic HTTPS on :443 via Let's Encrypt.
domain = ""

# Contact email for the Let's Encrypt account. Required when domain is set.
autocert_email = ""

# Persistent, writable directory where issued certificates are cached.
autocert_cache = "/var/lib/goblin/certs"

[auth]
# When false, everything is open with no login. Use only on a trusted LAN.
enabled = true

# bcrypt hash of the password. Set it with: gcloud set password
password_hash = ""

[storage]
# One or more directories that hold files. Uploads are balanced across them.
paths = ["/var/lib/goblin/data"]

# Roots with less free space than this are skipped when choosing where to write.
min_free = "1GB"

[ftp]
# Turn the FTP front door on or off.
enabled = true

# FTP control-connection bind address.
listen = ":2121"

# When true, require FTPS (explicit TLS).
tls = false

# Port range advertised for passive-mode data connections.
passive_ports = "30000-30100"

[log]
# Verbosity: debug | info | warn | error. debug adds per-request access logs.
level = "info"

# Output format: text (human/journal) or json (log aggregators).
format = "text"
`

// WriteTemplate writes the starter config to path. It refuses to overwrite an
// existing file unless force is true.
func WriteTemplate(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}
	return os.WriteFile(path, []byte(Template), 0o644)
}

// SetPasswordHash updates auth.password_hash in the file at path, preserving all
// other lines and comments. If the file or key is absent it is created/appended.
func SetPasswordHash(path, hash string) error {
	line := fmt.Sprintf("password_hash = %q", hash)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config yet: start from the template, then patch.
			if werr := WriteTemplate(path, false); werr != nil {
				return werr
			}
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return err
		}
	}

	var out []string
	replaced := false
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(l), "password_hash") {
			out = append(out, line)
			replaced = true
			continue
		}
		out = append(out, l)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if !replaced {
		out = append(out, line)
	}

	content := strings.Join(out, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
