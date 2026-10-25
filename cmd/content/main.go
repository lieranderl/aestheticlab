// Command content copies the site's content into and out of a site
// Firestore database (docs/CONTENT.md), with the operator's application
// default credentials (gcloud auth application-default login).
//
// Usage:
//
//	go run ./cmd/content import -project P -database site-staging FILE
//	go run ./cmd/content export -project P -database site-production [-out FILE]
//
// import writes an export file (the Supabase export at the migration, or a
// previous export) into the database, replacing documents with the same ID.
// export writes the database's content in the same format; make snapshot
// uses it to refresh internal/content/snapshot.json. Both check that the
// content is complete first, and both refuse databases not named site-*,
// so a typo cannot touch the project's other databases.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/lieranderl/aestheticlab/internal/content"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "content:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || (args[0] != "import" && args[0] != "export") {
		return errors.New("usage: content import|export -project P -database site-NAME [FILE | -out FILE]")
	}
	cmd := args[0]
	flags := flag.NewFlagSet(cmd, flag.ContinueOnError)
	project := flags.String("project", os.Getenv("GOOGLE_CLOUD_PROJECT"), "Google Cloud project")
	database := flags.String("database", "", "site database (site-staging, site-production)")
	out := flags.String("out", "", "export: file to write (default stdout)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *project == "" {
		return errors.New("-project (or GOOGLE_CLOUD_PROJECT) is required")
	}
	if !strings.HasPrefix(*database, "site-") {
		return fmt.Errorf("-database %q is not a site database (site-*)", *database)
	}

	var file string
	if cmd == "import" {
		if flags.NArg() != 1 {
			return errors.New("import takes one export file")
		}
		file = flags.Arg(0)
	}
	db, err := content.NewFirestore(ctx, *project, *database)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if cmd == "import" {
		return importFile(ctx, db, file, stdout)
	}
	return export(ctx, db, *out, stdout)
}

func importFile(ctx context.Context, db *content.Firestore, file string, stdout io.Writer) error {
	data, err := os.ReadFile(file) // #nosec G304 G703 -- the operator names the file to import.
	if err != nil {
		return err
	}
	docs, err := content.DecodeJSON(data)
	if err != nil {
		return err
	}
	if _, err := content.Parse(docs); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	n, err := db.Import(ctx, docs)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Imported %d documents (%d service groups, %d services, %d staff, 1 contact).\n",
		n, len(docs.ServiceGroups), len(docs.Services), len(docs.Staff))
	return err
}

func export(ctx context.Context, db *content.Firestore, out string, stdout io.Writer) error {
	docs, err := db.Export(ctx)
	if err != nil {
		return err
	}
	if _, err := content.Parse(docs); err != nil {
		return fmt.Errorf("exported content: %w", err)
	}
	data, err := encode(docs)
	if err != nil {
		return err
	}
	if out == "" {
		_, err = stdout.Write(data)
		return err
	}
	return os.WriteFile(out, data, 0o600) // #nosec G703 -- the operator names the file to write.
}

// encode writes docs in the export format: indented, keys sorted, text
// unescaped, with a final newline.
func encode(docs content.Documents) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(docs); err != nil {
		return nil, fmt.Errorf("encode content: %w", err)
	}
	return buf.Bytes(), nil
}
