package usage

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// collection holds one document per day ("day-2026-11-22") and per month
// ("month-2026-11"), each with the total in "eur" and each environment's
// share in "eur_<environment>".
const collection = "chat_spending"

// Firestore is a Store in a Firestore database of its own (chat-usage), the
// only one the site's runtime may write.
type Firestore struct{ client *firestore.Client }

// NewFirestore opens database in project; the client connects on first use.
func NewFirestore(ctx context.Context, project, database string) (*Firestore, error) {
	client, err := firestore.NewClientWithDatabase(ctx, project, database)
	if err != nil {
		return nil, fmt.Errorf("chat usage database %q: %w", database, err)
	}
	return &Firestore{client: client}, nil
}

// Close releases the client.
func (f *Firestore) Close() error { return f.client.Close() }

func (f *Firestore) refs(day, month string) (*firestore.DocumentRef, *firestore.DocumentRef) {
	c := f.client.Collection(collection)
	return c.Doc("day-" + day), c.Doc("month-" + month)
}

// Spent implements Store with one read of both documents; a missing
// document has spent nothing.
func (f *Firestore) Spent(ctx context.Context, day, month string) (float64, float64, error) {
	d, m := f.refs(day, month)
	snaps, err := f.client.GetAll(ctx, []*firestore.DocumentRef{d, m})
	if err != nil {
		return 0, 0, err
	}
	return eurOf(snaps[0]), eurOf(snaps[1]), nil
}

func eurOf(snap *firestore.DocumentSnapshot) float64 {
	if snap == nil || !snap.Exists() {
		return 0
	}
	switch v := snap.Data()["eur"].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	}
	return 0
}

// Reserve implements Store in one transaction: it reads both totals,
// checks the caps and adds eur, so concurrent reservations near a cap are
// serialized by Firestore and only those that fit get through.
func (f *Firestore) Reserve(ctx context.Context, day, month, env string, eur float64, caps Caps) error {
	d, m := f.refs(day, month)
	return f.client.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snaps, err := tx.GetAll([]*firestore.DocumentRef{d, m})
		if err != nil {
			return err
		}
		if err := caps.check(eurOf(snaps[0]), eurOf(snaps[1])); err != nil {
			return err
		}
		inc := map[string]any{"eur": firestore.Increment(eur), "eur_" + env: firestore.Increment(eur)}
		for _, ref := range []*firestore.DocumentRef{d, m} {
			if err := tx.Set(ref, inc, firestore.MergeAll); err != nil {
				return err
			}
		}
		return nil
	})
}

// Add implements Store with atomic increments, so instances recording at
// once never lose each other's spending.
func (f *Firestore) Add(ctx context.Context, day, month, env string, eur float64) error {
	d, m := f.refs(day, month)
	inc := map[string]any{"eur": firestore.Increment(eur), "eur_" + env: firestore.Increment(eur)}
	batch := f.client.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, 2)
	for _, ref := range []*firestore.DocumentRef{d, m} {
		job, err := batch.Set(ref, inc, firestore.MergeAll)
		if err != nil {
			batch.End()
			return err
		}
		jobs = append(jobs, job)
	}
	batch.End()
	var errs []error
	for _, job := range jobs {
		if _, err := job.Results(); err != nil && status.Code(err) != codes.OK {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
