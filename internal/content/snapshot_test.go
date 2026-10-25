package content_test

import (
	"os"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/i18n"
)

func TestTheSnapshotCoversEveryLocale(t *testing.T) {
	data, err := os.ReadFile("snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := content.DecodeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	c, err := content.Parse(docs)
	if err != nil {
		t.Fatalf("Parse(snapshot) error = %v", err)
	}
	for _, l := range i18n.Supported() {
		loc := c.In(l)
		if len(loc.ServiceGroups) == 0 || len(loc.Services) == 0 || len(loc.Staff) == 0 {
			t.Errorf("%s: %d groups, %d services, %d staff", l, len(loc.ServiceGroups), len(loc.Services), len(loc.Staff))
		}
	}
	// The snapshot never carries what the site does not show.
	for _, m := range docs.Staff {
		if _, ok := m["email"]; ok {
			t.Errorf("staff %v carries an email address", m["id"])
		}
	}
	if content.Snapshot() == nil {
		t.Error("Snapshot() = nil")
	}
}
