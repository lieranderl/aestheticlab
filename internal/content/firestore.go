package content

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"cloud.google.com/go/firestore"
	"golang.org/x/sync/errgroup"
)

// Where content lives in a site database (docs/CONTENT.md).
const (
	ServiceGroupsCollection = "service_groups"
	ServicesCollection      = "services"
	StaffCollection         = "staff"
	// siteCollection/contactID is the one contact document.
	siteCollection, contactID = "site", "contact"
)

// Firestore is the Source for a site database (with FIRESTORE_EMULATOR_HOST
// set, that emulator's).
type Firestore struct {
	client *firestore.Client
}

// NewFirestore opens database in project. The connection and credentials
// are set up on the first read, not here.
func NewFirestore(ctx context.Context, project, database string) (*Firestore, error) {
	client, err := firestore.NewClientWithDatabase(ctx, project, database)
	if err != nil {
		return nil, fmt.Errorf("open firestore %s/%s: %w", project, database, err)
	}
	return &Firestore{client: client}, nil
}

// Close releases the connection to Firestore.
func (f *Firestore) Close() error { return f.client.Close() }

// Fetch implements Source: the three collections and the contact document,
// read in parallel, inactive documents included. Each document's "id" field
// is its document ID when it has none.
func (f *Firestore) Fetch(ctx context.Context) (Documents, error) {
	var docs Documents
	g, ctx := errgroup.WithContext(ctx)
	for name, dst := range map[string]*[]Document{
		ServiceGroupsCollection: &docs.ServiceGroups,
		ServicesCollection:      &docs.Services,
		StaffCollection:         &docs.Staff,
	} {
		g.Go(func() error {
			snaps, err := f.client.Collection(name).Documents(ctx).GetAll()
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			for _, snap := range snaps {
				*dst = append(*dst, withID(snap))
			}
			return nil
		})
	}
	g.Go(func() error {
		snap, err := f.client.Collection(siteCollection).Doc(contactID).Get(ctx)
		if err != nil {
			return fmt.Errorf("read %s/%s: %w", siteCollection, contactID, err)
		}
		docs.Contact = snap.Data()
		return nil
	})
	if err := g.Wait(); err != nil {
		return Documents{}, err
	}
	return docs, nil
}

// Ping implements Source with one read: the contact document.
func (f *Firestore) Ping(ctx context.Context) error {
	if _, err := f.client.Collection(siteCollection).Doc(contactID).Get(ctx); err != nil {
		return fmt.Errorf("read %s/%s: %w", siteCollection, contactID, err)
	}
	return nil
}

// Import writes docs into the database, each document by its "id" (the
// Supabase ID) and the contact as site/contact. It replaces documents with
// the same ID and leaves others alone, so running it again is harmless.
// Whole numbers are stored as integers, as Supabase's bigint columns were.
func (f *Firestore) Import(ctx context.Context, docs Documents) (int, error) {
	type write struct {
		ref  *firestore.DocumentRef
		data Document
	}
	var writes []write
	for name, list := range map[string][]Document{
		ServiceGroupsCollection: docs.ServiceGroups,
		ServicesCollection:      docs.Services,
		StaffCollection:         docs.Staff,
	} {
		for _, doc := range list {
			id := readID(doc, "id")
			if id == "" {
				return 0, fmt.Errorf("%s: a document has no id", name)
			}
			writes = append(writes, write{f.client.Collection(name).Doc(id), integers(doc).(Document)})
		}
	}
	if docs.Contact != nil {
		writes = append(writes, write{f.client.Collection(siteCollection).Doc(contactID), integers(docs.Contact).(Document)})
	}
	bw := f.client.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(writes))
	for _, w := range writes {
		job, err := bw.Set(w.ref, w.data)
		if err != nil {
			bw.End()
			return 0, fmt.Errorf("write %s: %w", w.ref.Path, err)
		}
		jobs = append(jobs, job)
	}
	bw.End()
	for i, job := range jobs {
		if _, err := job.Results(); err != nil {
			return 0, fmt.Errorf("write %s: %w", writes[i].ref.Path, err)
		}
	}
	return len(writes), nil
}

// Export is the database's content in the export format, each collection
// sorted by ID so the file diffs cleanly.
func (f *Firestore) Export(ctx context.Context) (Documents, error) {
	docs, err := f.Fetch(ctx)
	if err != nil {
		return Documents{}, err
	}
	for _, list := range []([]Document){docs.ServiceGroups, docs.Services, docs.Staff} {
		slices.SortFunc(list, func(a, b Document) int {
			return cmp.Or(compareIDs(readID(a, "id"), readID(b, "id")), cmp.Compare(readID(a, "id"), readID(b, "id")))
		})
	}
	return docs, nil
}

// withID is snap's fields, with its document ID as "id" when it has none.
func withID(snap *firestore.DocumentSnapshot) Document {
	doc := snap.Data()
	if _, ok := doc["id"]; !ok {
		doc["id"] = snap.Ref.ID
	}
	return doc
}

// integers is v with whole float64 numbers (JSON's) as int64, in maps and
// lists too.
func integers(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = integers(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = integers(x)
		}
		return out
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return int64(v)
		}
	}
	return v
}
