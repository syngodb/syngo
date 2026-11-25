package syngo

import (
	"bytes"
	"fmt"
	"time"
)

// Document is one stored document, as it was at the moment it was read: its identifier, its JSON
// text, and what the database keeps about it. A Document never changes, and may be shared between
// goroutines.
type Document struct {
	id   string
	text []byte
	// modified is an instant in nanoseconds since 1970, or zero when it is not known.
	modified int64
	revision uint64
	// rare is what few documents have, or nil. A Document is made for every document a program
	// reads, and with this behind one pointer it takes 64 bytes instead of 96.
	rare *rareFacts
}

// rareFacts is what few documents have: metadata, and a time to expire at (in nanoseconds since
// 1970, zero for a document that does not expire).
type rareFacts struct {
	meta    []byte
	expires int64
}

// ID is the document's identifier.
func (d *Document) ID() string { return d.id }

// JSON is the document as JSON text, its identifier included. The bytes are the caller's own: a
// copy, to keep and to change at will. A program that reads many documents and wants no memory
// taken for each uses AppendJSON.
func (d *Document) JSON() []byte { return bytes.Clone(d.text) }

// AppendJSON appends the document's JSON text to dst and returns the longer slice, as append
// does. With a buffer that is used again (buf = doc.AppendJSON(buf[:0])) it takes no memory.
func (d *Document) AppendJSON(dst []byte) []byte { return append(dst, d.text...) }

// String is the document as JSON text.
func (d *Document) String() string { return string(d.text) }

// Decode decodes the document into v, by its json tags.
func (d *Document) Decode(v any) error {
	if err := DecodeJSON(d.text, v); err != nil {
		return fmt.Errorf("syngo: the document %q does not decode into the value given: %w", d.id, err)
	}
	return nil
}

// Expires is when the document expires, or the zero time when it does not.
func (d *Document) Expires() time.Time {
	if d.rare != nil && d.rare.expires != 0 {
		return time.Unix(0, d.rare.expires).UTC()
	}
	return time.Time{}
}

// Meta is what was stored beside the document with the Meta option, as JSON text, or nil. Like
// JSON, it is a copy.
func (d *Document) Meta() []byte {
	if d.rare == nil {
		return nil
	}
	return bytes.Clone(d.rare.meta)
}

// Modified is when the document was last written, or the zero time when the database could not
// say.
func (d *Document) Modified() time.Time {
	if d.modified != 0 {
		return time.Unix(0, d.modified).UTC()
	}
	return time.Time{}
}

// Revision counts how many times the document was written: 1 for a new document. A document that
// is deleted and stored again begins again at 1.
func (d *Document) Revision() uint64 { return d.revision }

// Page is one page of an answer.
type Page struct {
	// Items are the documents of the page, in order.
	Items []*Document
	// Next is the token After takes to read the page that follows; it is empty when this page is
	// the last.
	Next string
}

// Index declares an index of a collection. An index makes the queries it serves faster and never
// changes what any query answers, with one exception: a Unique index also refuses a write that
// would give two documents the same value.
type Index struct {
	// Name identifies the index in the collection: 1 to 255 bytes of valid UTF-8.
	Name string
	// Fields are the indexed field paths, in order: "age", "address.city". A field may be followed
	// by its direction, as in a query's order: "created desc". An index over several fields serves
	// the queries that pin its first fields to values and range over, or order by, the next ones:
	// {"status", "created desc"} answers status = "open" order by created desc straight from the
	// index.
	//
	// A field followed by "fold" ("name fold") is indexed ignoring case: the index then serves
	// the operator that ignores case (name like "ann", name like "an%") and equality with a text,
	// and gives no order on that field. It changes no answer, like any index: every other
	// operator goes on comparing the field exactly.
	//
	// A field followed by "text" ("body text") is indexed word by word, for full text search: the
	// index then serves Matches on that field (body matches "database engine") and nothing else.
	// The words are those of English unless a language follows ("body text de", "body text none"),
	// and the index serves the searches of that language. One field of an index may be so
	// declared; with other fields before it ({"tenant", "body text"}), the index answers a search
	// inside the documents that hold those to a value.
	Fields []string
	// Unique makes the index a constraint: no two documents may hold the same value (the same
	// combination of values, for several fields). Each element of an array counts as a value of
	// its own; a field that is missing, null or an empty array holds no value, and a document in
	// which any field of the index holds none is not constrained.
	//
	// Over several fields, each combination of the values counts as one. When more than one of
	// the fields holds an array, a document may hold at most 1,024 combinations: a write of one
	// that holds more is refused, with an error that is ErrInvalidDocument.
	Unique bool
}

// Stats is what a database says about itself at one moment.
type Stats struct {
	// Collections are the collections, by name.
	Collections []CollectionStats
	// DiskBytes is what the database takes on disk.
	DiskBytes int64
	// Cache is the document cache: the documents kept in memory to be handed out again.
	Cache CacheStats
	// Engine is the storage engine's own memory: the pages of its files it keeps, and the writes
	// it has not yet moved to their final place.
	Engine EngineStats
}

// CollectionStats is one collection of a Stats.
type CollectionStats struct {
	// Name is the collection's name.
	Name string
	// Documents is the number of documents stored, the ones that have expired and are not yet
	// removed included (the Count of a collection leaves those out, and costs more when there are
	// some).
	Documents int64
	// Indexes are the indexes, by name.
	Indexes []IndexStats
}

// IndexStats is one index of a Stats.
type IndexStats struct {
	// Name, Fields and Unique are the index as it was declared (see Index).
	Name   string
	Fields []string
	Unique bool
	// Building says the index is still being built: it is maintained, and not yet read.
	Building bool
	// Rebuilding says another index is being built to take the place of this one.
	Rebuilding bool
	// Stale says the index keeps a field word by word, was built by another analysis than that of
	// the program that has the database open, and cannot be built again because the database is
	// open read only: no query reads it, and the searches it would answer read the documents.
	// Opening the database to be written builds it again.
	Stale bool
}

// CacheStats is the document cache of a Stats. A cache that keeps pushing documents out
// (Evictions grows) while documents keep being read from the storage engine (Misses grows) is
// one that would gain from a larger budget; one that pushes nothing out holds everything that is
// being read.
type CacheStats struct {
	// Budget is the most the cache may hold, and Used what it holds, in bytes. When several
	// databases share one cache, Budget is that of the cache they share and Used the part of it
	// this database holds.
	Budget, Used int64
	// Hits and Misses count, since the database was opened, the documents asked for that the
	// cache held and the ones that had to be read from the storage engine. A document a query
	// reads only as part of a scan of its collection is neither: scans do not go through the
	// cache.
	Hits, Misses uint64
	// Evictions counts the documents pushed out of the cache to make room for others.
	Evictions uint64
}

// EngineStats is the storage engine of a Stats.
type EngineStats struct {
	// CacheBudget is the most the engine's own cache may hold, and CacheUsed what it holds, in
	// bytes.
	CacheBudget, CacheUsed int64
	// CacheHits and CacheMisses count the pages of files asked for that the cache held and the
	// ones that had to be read from disk.
	CacheHits, CacheMisses int64
	// Buffered is the size of the writes kept in memory until the engine moves them to its files.
	Buffered int64
}
