# Data Loading

The site's treatments, prices, staff and contact details live in Firestore. `docs/CONTENT.md` describes the data model and how to edit it; this guide covers how the code reads it.

## Flow

```text
Firestore (site-staging | site-production)
  → content.Firestore   reads every content document (Source)
  → content.Store       per-instance cache, refresh, fallbacks
  → content.Parse       validates and projects all five locales once per fetch
  → (*Content).In(l)    the Localized view for one locale
  → internal/views      renders it
```

`cmd/web` creates the store at startup and prefetches once the server listens. Handlers in `internal/server` call `store.Content(ctx).In(locale)` and pass the result to a view. Views never fetch.

## Cache and Fallbacks

| Setting | Value | Meaning |
| --- | --- | --- |
| TTL | 15 min | Content edits reach visitors within about this long |
| Stale wait | 300 ms | A request with stale content waits this long for the refresh, then renders the stale content |
| Cold wait | 1.5 s | A request on a cold instance waits this long, then renders the snapshot |
| Fetch timeout | 5 s | A refresh outlives its request; a stuck refresh is replaced by the next request |

- One refresh runs at a time, and concurrent requests join it.
- On error the store keeps the last good content, else the embedded `internal/content/snapshot.json` (logged as `content_snapshot_served`).
- Content without an active group, an active service or a complete contact is rejected, so a half-edited database never replaces good content.
- Without `FIRESTORE_DATABASE` the site serves the snapshot and `/dependencyz` answers 503.

## Changing the Data Model

1. Change the documents in the Firestore console (staging first), and describe the field in `docs/CONTENT.md`.
2. Write the test first in `internal/content` (table-driven, over documents), then parse the field in `content.go` or `fields.go` and expose it on `Localized`.
3. Localized fields follow the `<name>_<lang>` pattern with English fallback; add all five.
4. Render it in `internal/views`, then refresh the snapshot with `make snapshot` and commit it.

## Rules

- The runtime has read-only, database-conditioned IAM and no keys. Never add a write path to the site.
- Keep Firestore reads bounded: one fetch per refresh. The production alert fires above 2,000 reads an hour.
- Tests use stand-in sources; `make firestore` and `make test-content` run the emulator tests.
