package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/content"
)

func TestOnlySiteDatabasesAreTouched(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "usage"},
		{[]string{"delete"}, "usage"},
		{[]string{"export", "-database", "site-staging"}, "-project"},
		{[]string{"export", "-project", "p", "-database", "(default)"}, "not a site database"},
		{[]string{"import", "-project", "p", "-database", "aestheticlab-income", "x.json"}, "not a site database"},
		{[]string{"import", "-project", "p", "-database", "site-staging"}, "one export file"},
	} {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "")
		err := run(context.Background(), tc.args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("run(%q) = %v, want an error about %q", tc.args, err, tc.want)
		}
	}
}

func TestTheExportFormatKeepsTextReadable(t *testing.T) {
	data, err := encode(contentWith("Brows & lashes <b>"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"Brows & lashes <b>"`) || !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("encode() = %s, want unescaped text and a final newline", data)
	}
}

func contentWith(name string) content.Documents {
	return content.Documents{ServiceGroups: []content.Document{{"name": name}}}
}
