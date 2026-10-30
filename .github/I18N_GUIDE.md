# i18n Guide

The site speaks five locales, each with its own URL prefix: `en-BE` (default), `nl-BE`, `fr-BE`, `ru-BE` and `uk-BE`. `/` serves English; `internal/server` routes the prefixes and `internal/i18n` holds the texts.

## Texts

English is the key. A template writes the English text and gets it back in the request's locale:

| Helper (in `internal/views`) | Use |
| --- | --- |
| `tr(ctx, "Book now")` | A plain text |
| `trf(ctx, "Book with %v", name)` | A text with arguments |
| `trc(ctx, "treatment overview", "Back to categories")` | The same English that must translate differently elsewhere |
| `trJS(ctx, "Close")` | A text quoted for a Datastar expression |

They wrap `i18n.T`, `i18n.Tf` and `i18n.C`. A missing translation falls back to English.

## Locale Files

The source is `internal/i18n/locales/<locale>.json`: nested objects whose dotted path names a text (`app.nav.services`). The key only names it; templates use the English.

To add or change a text:

1. Add it under a descriptive key in all five files, with `{{name}}` placeholders where it takes arguments. Placeholders must match across locales.
2. Run `make i18n`. It writes `internal/i18n/messages_gen.go`, where placeholders become `%v` verbs. Commit both.
3. Use the English in the template.

`make i18n-check` (part of `make ci`) fails when the tables are stale. The conversion fails on a missing translation, mismatched placeholders, a key in a translation but not in English, or two keys that share English but translate differently. For the last case, add the key to `contexts` in `cmd/i18n-convert` and use `trc`. `internal/views` tests check that every text a template uses has a translation.

## Content in Five Languages

Firestore fields carry their locale in their name (`name_nl`, `description_fr`); `internal/content` projects them per locale with English fallback (see `DATA_LOADING.md`).

## Adding a Locale

1. Add the constant and its native name in `internal/i18n/i18n.go`, and the entry in `locales` in `cmd/i18n-convert`.
2. Add `internal/i18n/locales/<locale>.json` with every text, then `make i18n`.
3. Add the content fields in Firestore and their projection in `internal/content`.
4. Add the link-preview image (`bun run share-images`), the `sitemap.xml` and `hreflang` entries, and the browser specs' locale cases.

## Rules

- Never hard-code visible text in a template, including `alt`, `aria-label` and `title`.
- Keep sentences whole; don't build them from fragments, since word order differs between languages.
- Format prices and dates per locale through the existing helpers in `internal/views`.
