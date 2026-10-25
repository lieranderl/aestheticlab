package content

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// snapshotJSON is production's content at the last make snapshot, in the
// export format (Documents). It is what a cold instance renders when
// Firestore does not answer in time.
//
//go:embed snapshot.json
var snapshotJSON []byte

// Snapshot is the embedded snapshot, parsed the first time it is needed:
// a healthy instance never parses it. TestTheSnapshotCoversEveryLocale
// keeps it parseable.
var Snapshot = sync.OnceValue(func() *Content {
	c, err := ParseJSON(snapshotJSON)
	if err != nil {
		panic(fmt.Sprintf("content: embedded snapshot: %v", err))
	}
	return c
})

// ParseJSON parses content in the export format.
func ParseJSON(data []byte) (*Content, error) {
	docs, err := DecodeJSON(data)
	if err != nil {
		return nil, err
	}
	return Parse(docs)
}

// DecodeJSON decodes documents in the export format.
func DecodeJSON(data []byte) (Documents, error) {
	var docs Documents
	if err := json.Unmarshal(data, &docs); err != nil {
		return Documents{}, fmt.Errorf("decode content: %w", err)
	}
	return docs, nil
}
