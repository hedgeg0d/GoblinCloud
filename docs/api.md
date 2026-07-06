# REST API

The HTTP server exposes a small JSON API under `/api` and serves the web UI from
`/`. Same origin, same auth.

## Authentication

When `auth.enabled` is true, every `/api/*` route except `POST /api/login`
requires a valid session.

1. `POST /api/login` with the password → the server sets an `HttpOnly` session
   cookie.
2. Subsequent requests carry the cookie automatically (browser) or you send it
   back manually (scripts).

For non-browser clients that prefer stateless calls, HTTP Basic auth with the
same password is also accepted on `/api/*` routes.

Unauthenticated calls return `401`. When `auth.enabled` is false, all routes are
open and `login` is a no-op.

## Paths

The `path` query parameter is always a logical path rooted at `/` — the merged
view across all storage roots (see [storage.md](storage.md)). Path traversal
(`..`) is rejected with `400`.

## Endpoints

### `POST /api/login`

Body: `{ "password": "…" }`. On success sets the session cookie and returns
`204`. On failure returns `401`.

### `GET /api/files?path=/some/dir`

List a directory. Returns the merged listing across all roots:

```json
{
  "path": "/some/dir",
  "entries": [
    { "name": "photos", "type": "dir",  "size": 0,      "modified": "2026-07-01T10:00:00Z" },
    { "name": "a.jpg",  "type": "file", "size": 20481, "modified": "2026-07-02T14:30:00Z" }
  ]
}
```

`404` if the directory exists on no root.

### `POST /api/folder`

Body: `{ "path": "/some/dir/newname" }`. Create a directory. `201` on success,
`409` if it already exists.

### `POST /api/upload?path=/some/dir`

`multipart/form-data` with one or more file parts. Each file is stored under
`path`, balanced onto the root with the most free space. Returns `201` and a
summary of stored files. `507` if no root has room.

### `GET /api/download?path=/some/dir/a.jpg`

Stream the file with appropriate `Content-Type` and `Content-Disposition`.
`404` if it exists on no root. Supports HTTP range requests for resumable
downloads.

### `POST /api/rename`

Body: `{ "from": "/a/old.txt", "to": "/a/new.txt" }`. Rename or move a file or
directory. Within one root it's a cheap rename; across roots it's a move. `200`
on success, `404` if the source is missing, `409` if the target exists.

### `DELETE /api/files?path=/some/dir/a.jpg`

Delete a file or (recursively) a directory. Removes it from whichever root(s)
hold it. `204` on success, `404` if nothing matched.

## Errors

Errors are JSON: `{ "error": "message" }` with a matching HTTP status.

| Status | Meaning                              |
|--------|--------------------------------------|
| 400    | Bad path or malformed request        |
| 401    | Not authenticated                    |
| 404    | Not found                            |
| 409    | Already exists                       |
| 507    | No storage root has room             |

## Example

```sh
# log in, keep the cookie
curl -c jar.txt -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' -d '{"password":"hunter2"}'

# upload
curl -b jar.txt -F file=@photo.jpg \
  'http://localhost:8080/api/upload?path=/photos'

# list
curl -b jar.txt 'http://localhost:8080/api/files?path=/photos'
```
