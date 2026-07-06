# Storage: the merged view

Goblin Cloud can spread files across several directories — typically one per
physical disk — while presenting them as a single tree. It does this **without a
database**. This document explains exactly how.

## The idea

You configure one or more roots:

```toml
[storage]
paths = ["/mnt/disk1/goblin", "/mnt/disk2/goblin"]
```

Every logical path (say `/photos/a.jpg`) may physically live under any one root
(`/mnt/disk1/goblin/photos/a.jpg` **or** `/mnt/disk2/goblin/photos/a.jpg`). The
storage layer is the only code that knows which; everything above it sees one
clean tree rooted at `/`.

No index file, no sqlite. The filesystem *is* the index — we look things up by
checking the roots directly.

## Rules

### Write (upload, create file)

Pick the **eligible root with the most free space**, and write there.

- A root is *eligible* if its free space is at or above `storage.min_free`.
- If several are eligible, the one with the most free bytes wins. This naturally
  levels usage over time — the emptiest disk keeps attracting writes until
  another becomes emptier.
- If no root is eligible, the write fails with an out-of-space error (`507` over
  HTTP). No disk is ever filled past its margin.

This is the whole balancing strategy: greedy toward free space, evaluated per
write. It's simple, needs no state, and self-corrects.

### Read (download, stat)

Search the roots **in configured order**; the first one that has the path wins.
On a name collision across roots, the earlier-listed root shadows the later one.

### List directory

Return the **union** of entries from that directory across all roots, de-duplicated
by name. A directory "exists" if it exists on any root. This is why a folder can
show files that physically sit on different disks in one listing.

### Create directory

Create the (empty) directory on **all** roots. Cheap, and it keeps listings and
future writes consistent regardless of which root a later file lands on.

### Delete

Remove the target from **whichever root(s)** hold it. For a directory, recurse
and remove it from every root that has it.

### Rename / move

- Same root → a plain `rename` (cheap, atomic).
- Different roots (or the natural target root differs) → a **move**: copy to the
  destination then remove the source.

## Path safety

Every logical path is cleaned and confined:

- Normalise with `filepath.Clean`.
- Reject any path that escapes the root (`..` traversal) → `400`.
- Join onto each candidate root and re-verify the result is still under that
  root before touching disk.

No request can ever address a file outside the configured roots.

## Consequences & trade-offs

- **Collisions:** the same relative path can, in principle, exist on two roots.
  The rules above make reads deterministic (first root wins) and creates
  balanced, so this is rare in practice and always resolvable.
- **`storage status`:** `gcloud storage status` reports per-root totals, free
  space, and write-eligibility so you can see the balance at a glance.
- **Adding a disk:** append it to `paths` and restart. New writes flow to it
  first (it's the emptiest); existing files stay put.
- **Removing a disk:** move its contents under another root, drop it from
  `paths`, restart. There's no metadata to reconcile — it's just files.
