# Site Content

The Go site (`cmd/web`) reads its treatments, prices, staff and contact details from Firestore. This page describes where they live, what each field means, and how to change them. The Qwik site still reads Supabase until the production cutover (`plans/go-datastar-scale-to-zero.md`, Phase 9).

## Databases

| Environment | Database | Read by |
| --- | --- | --- |
| Staging | `site-staging` | `aesthetic-stage-runtime` |
| Production | `site-production` | `aesthetic-prod-runtime` |

- Both are Firestore Native databases in `europe-west1`, managed in `infra/` with delete protection.
- Each runtime can only read its own database. The project's other databases, `(default)` and `aestheticlab-income`, belong to other apps; the site never touches them.
- The site holds no database key. Cloud Run's service account authenticates it.

## Data Model

Field names are the Supabase column names, so the data moved as is. Document IDs are the Supabase IDs, and each document also keeps its `id` field.

### `service_groups/{id}`

A treatment category, such as Manicure or Laser hair removal FACE.

| Field | Meaning |
| --- | --- |
| `name` | English name. Laser and add-on detection, and the category's images, use it. |
| `name_en` | Optional English display name, used instead of `name` |
| `name_nl`, `name_fr`, `name_ru`, `name_uk` | Translations. When empty, the English name is shown. |
| `priority` | Order. The home page and pricelist show higher priorities first. |
| `active` | `true` to show the group |

### `services/{id}`

A treatment with its price.

| Field | Meaning |
| --- | --- |
| `group_id` | The `id` of its service group |
| `name`, `name_nl`, `name_fr`, `name_ru`, `name_uk` | English name and translations |
| `description`, `description_nl`, `description_fr`, `description_ru`, `description_uk` | English description and translations |
| `price` | Price in euros (a number) |
| `duration` | Duration in minutes (a number) |
| `priority` | Order within the data. Within a group, the site shows services cheapest first. |
| `active` | `true` to show the service |
| `category` | Unused label kept from Supabase |

- Names starting with "Extra", and names containing "coating removal", are add-ons. Add-ons never set a group's "from" price.
- Groups whose English name contains "laser" or "removal" are shown together as one laser category.

### `staff/{id}`

| Field | Meaning |
| --- | --- |
| `name` | Name |
| `role` | Role, in English |
| `about`, `about_nl`, `about_fr`, `about_ru`, `about_uk` | Bio and translations |
| `photo_url` | A bundled photo name (`julia`, `julia.jpg`) or an `https://` URL |
| `active` | `true` to show the person |
| `color` | Unused, kept from Supabase |

Staff email addresses were not copied from Supabase, and don't belong here: every field in this database can end up on the public site.

### `site/contact`

| Field | Meaning |
| --- | --- |
| `email` | Studio email shown on the site |
| `open_hours` | Map: `start_week_day`, `end_week_day`, `from`, `to` (for example `Monday`, `Saturday`, `10:00`, `18:00`) |
| `location` | Map: `name`, `address`, `link` (a maps URL) |
| `parking` | List of maps: `name`, `link` |

All links must be `http://` or `https://` URLs. Any other link is dropped.

## Editing Content

1. Open the database in the Google Cloud console: Firestore, then `site-production` (or `site-staging` to try a change first).
2. Edit the field, keeping its type: numbers stay numbers, `active` stays a boolean.
3. To hide something, set `active` to `false` rather than deleting it.

- Changes reach visitors within about 15 minutes: each instance keeps content for 15 minutes, and pages may be cached for another minute.
- A document with a missing or malformed required field (no name, a price written as text) is left out of the page; the rest still shows.
- If the database ever has no active group, no active service or no complete contact, the site keeps showing the last good content, or the snapshot below.

Who may edit, and through which identity, is still an open question in the plan. Until it is settled, only the project's owners can edit, and run the import.

## When Firestore Is Slow or Down

- The site waits up to 1.5 s for content on a cold instance, and up to 300 ms when it has content older than 15 minutes.
- If the wait runs out, it shows the last good content. A new instance shows the snapshot embedded in the image, `internal/content/snapshot.json`, and logs `content_snapshot_served`.
- `/dependencyz` answers 503 when the database doesn't answer. Deploy smoke tests use it. The production uptime check that calls it today is removed when the site scales to zero (plan, Phase 9a).

## Commands

Commands that touch a real database use your own application default credentials (`gcloud auth application-default login`), and refuse databases not named `site-*`.

| Task | Command |
| --- | --- |
| Refresh the snapshot from production, then commit it | `make snapshot` |
| Copy an export file into a database (replaces documents with the same ID) | `go run ./cmd/content import -project nail-lab-449417 -database site-staging FILE` |
| Export a database | `go run ./cmd/content export -project nail-lab-449417 -database site-staging -out FILE` |
| Run the site against the emulator | `make firestore`, import a file with `FIRESTORE_EMULATOR_HOST=localhost:8087`, then run with `FIRESTORE_DATABASE`, `GOOGLE_CLOUD_PROJECT` and `FIRESTORE_EMULATOR_HOST` set |
| Content tests against the emulator | `make firestore`, then `make test-content` |

### Copying From Supabase

The migration copy is a JSON export in the same format as `cmd/content export`. Run this read-only query in the Supabase SQL editor, and save the `doc` value as a file outside the repository:

```sql
select json_build_object(
  'service_groups', (select json_agg(to_jsonb(g) - 'created_at' order by g.id) from gettimely.service_groups g),
  'services', (select json_agg(to_jsonb(s) - 'created_at' order by s.id) from gettimely.services s),
  'staff', (select json_agg(to_jsonb(st) - 'created_at' - 'email' order by st.id) from gettimely.staff st),
  'contact', (select to_jsonb(c) - 'created_at' from gettimely.contacts c where id = 1)
)::text as doc;
```

Import it into `site-staging` first, check staging, then import it into `site-production`. Running the import again is harmless.
