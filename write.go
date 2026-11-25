package syngo

import "time"

// WriteOption changes how one document is written. The zero WriteOption changes nothing.
type WriteOption struct {
	kind uint8
	in   time.Duration
	at   time.Time
	meta any
}

const (
	writeExpireIn uint8 = iota + 1
	writeExpireAt
	writeMeta
)

// ExpireIn makes the document expire that long after it is written. From then on it is gone, as
// if deleted. Zero means it never expires.
//
// When a document expires is part of each write: Put, Insert and Replace without the option
// store a document that never expires, whatever the one before it did. Patch and Modify keep
// what the document had unless they are given the option, and ExpireIn(0) is how they make a
// document permanent again.
//
// The time is counted by the clock of the database: through a server, by the server's.
func ExpireIn(d time.Duration) WriteOption { return WriteOption{kind: writeExpireIn, in: d} }

// ExpireAt makes the document expire at the time given. A time already past stores a document
// that nobody will see. The zero time means it never expires, as ExpireIn(0) does.
//
// Times are kept in nanoseconds, which reach to April of the year 2262: a time past that (a date
// in the year 9999 that stands for "never", say) is kept as that last instant, and the document
// lives until then.
func ExpireAt(t time.Time) WriteOption { return WriteOption{kind: writeExpireAt, at: t} }

// Meta stores a value beside the document: something about the document that is not part of it
// (where it came from, who wrote it). It is returned by Document.Meta, and cannot be queried.
//
// The value is given as a document is: JSON text ([]byte or string) is taken as it is, and any
// other Go value is encoded as JSON. So Meta(`{"by":"import"}`) and Meta(map[string]string{"by":
// "import"}) store the same, and a text that is not JSON is refused by the write. Meta(nil)
// stores none.
//
// Like the expiry, it is part of each write: Put, Insert and Replace without the option store a
// document without metadata. Patch and Modify keep what the document had unless they are given
// the option, and Meta(nil) is how they remove it.
func Meta(v any) WriteOption { return WriteOption{kind: writeMeta, meta: v} }
