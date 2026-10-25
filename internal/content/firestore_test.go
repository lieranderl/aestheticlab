package content_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/grpc"

	"github.com/lieranderl/aestheticlab/internal/content"
)

// emulator opens a new database on the Firestore emulator
// (FIRESTORE_EMULATOR_HOST, as make firestore and CI start it), or skips.
func emulator(t *testing.T) *content.Firestore {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("Firestore not checked: set FIRESTORE_EMULATOR_HOST (make firestore)")
	}
	db, err := content.NewFirestore(context.Background(), "demo-aestheticlab", "site-"+strings.ToLower(rand.Text()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func snapshotDocs(t *testing.T) content.Documents {
	t.Helper()
	data, err := os.ReadFile("snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := content.DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFirestoreKeepsTheImportedContent(t *testing.T) {
	db := emulator(t)
	ctx := context.Background()
	if err := db.Ping(ctx); err == nil {
		t.Error("Ping() of an empty database = nil, want the missing contact")
	}
	docs := snapshotDocs(t)
	for range 2 { // importing again changes nothing
		n, err := db.Import(ctx, docs)
		if err != nil {
			t.Fatal(err)
		}
		if want := len(docs.ServiceGroups) + len(docs.Services) + len(docs.Staff) + 1; n != want {
			t.Errorf("Import() = %d documents, want %d", n, want)
		}
	}
	exported, err := db.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := asJSON(t, exported), asJSON(t, docs); got != want {
		t.Errorf("export differs from the import:\n got %.300s\nwant %.300s", got, want)
	}
	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping() = %v", err)
	}

	store := content.NewStore(content.Options{Source: db})
	got := store.Content(ctx)
	if got == content.Snapshot() {
		t.Fatal("the store served the snapshot, want the database's content")
	}
	if a, b := asJSON(t, got.In("en-BE")), asJSON(t, content.Snapshot().In("en-BE")); a != b {
		t.Error("the database's content differs from the snapshot it was imported from")
	}
}

// An unanswering Firestore: a gRPC server that implements nothing.
func TestAnUnansweringFirestoreFallsBackToTheSnapshot(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	firestorepb.RegisterFirestoreServer(server, &firestorepb.UnimplementedFirestoreServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	t.Setenv("FIRESTORE_EMULATOR_HOST", listener.Addr().String())
	db, err := content.NewFirestore(context.Background(), "demo-aestheticlab", "site-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := newStore(db)
	if got := h.store.Content(context.Background()); got != content.Snapshot() {
		t.Error("content is not the snapshot")
	}
	if !strings.Contains(h.logs.String(), "content_refresh_failed") {
		t.Errorf("log lacks content_refresh_failed: %s", h.logs)
	}
	if err := h.store.Ping(context.Background()); err == nil {
		t.Error("Ping() = nil, want an error")
	}
}
