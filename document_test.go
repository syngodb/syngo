package syngo

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestDocument(t *testing.T) {
	// The texts are kept as constants as well: what the document says is compared with them, and
	// not with the bytes it was made from, which a document that shared its memory would change
	// along with itself.
	const wantText, wantMeta = `{"_id":"u1","name":"Ann","age":31}`, `{"by":"import"}`
	modified := time.Date(2026, 11, 24, 10, 0, 0, 5, time.UTC)
	expires := modified.Add(time.Hour)
	d := NewDocument("u1", []byte(wantText), DocumentInfo{Revision: 3, Modified: modified.UnixNano(), Expires: expires.UnixNano(), Meta: []byte(wantMeta)})

	if d.ID() != "u1" || d.Revision() != 3 || d.String() != wantText {
		t.Errorf("identifier %q, revision %d, text %s", d.ID(), d.Revision(), d)
	}
	if !d.Modified().Equal(modified) || d.Modified().Location() != time.UTC {
		t.Errorf("modified %v", d.Modified())
	}
	if !d.Expires().Equal(expires) || d.Expires().Location() != time.UTC {
		t.Errorf("expires %v", d.Expires())
	}
	var u struct {
		ID   string `json:"_id"`
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := d.Decode(&u); err != nil || u.ID != "u1" || u.Name != "Ann" || u.Age != 31 {
		t.Errorf("decoded %+v, %v", u, err)
	}

	// What a document hands out is a copy: changing it changes nothing of the document.
	got := d.JSON()
	if string(got) != wantText {
		t.Fatalf("JSON: %s", got)
	}
	got[2] = 'X'
	if d.String() != wantText {
		t.Errorf("a document followed the text it handed out: %s", d)
	}
	m := d.Meta()
	if string(m) != wantMeta {
		t.Fatalf("Meta: %s", m)
	}
	m[2] = 'X'
	if string(d.Meta()) != wantMeta {
		t.Errorf("a document followed the metadata it handed out: %s", d.Meta())
	}
	buf := make([]byte, 0, 64)
	buf = append(buf, "x="...)
	buf = d.AppendJSON(buf)
	if string(buf) != "x="+wantText {
		t.Errorf("AppendJSON: %s", buf)
	}
	buf[3] = 'X'
	if d.String() != wantText {
		t.Errorf("a document followed the buffer it was appended to: %s", d)
	}
}

func TestDocumentWithoutExtras(t *testing.T) {
	d := NewDocument("a", []byte(`{"_id":"a"}`), DocumentInfo{Revision: 1})
	if !d.Modified().IsZero() {
		t.Errorf("a document whose time of writing is not known says %v", d.Modified())
	}
	if !d.Expires().IsZero() {
		t.Errorf("a document that does not expire says %v", d.Expires())
	}
	if d.Meta() != nil {
		t.Errorf("a document without metadata says %s", d.Meta())
	}
}

func TestDocumentThatDoesNotDecode(t *testing.T) {
	d := NewDocument("u9", []byte(`{"_id":"u9","age":"old"}`), DocumentInfo{})
	var u struct {
		Age int `json:"age"`
	}
	err := d.Decode(&u)
	if err == nil || !strings.HasPrefix(err.Error(), `syngo: the document "u9" does not decode into the value given: `) {
		t.Errorf("got %v", err)
	}
}

func TestErrors(t *testing.T) {
	dup := error(&DuplicateError{Collection: "users", Index: "by_email", Holder: "u1"})
	if !errors.Is(dup, ErrDuplicate) || errors.Is(dup, ErrExists) ||
		dup.Error() != `syngo: the value is already taken in the unique index "by_email" of "users" by document "u1"` {
		t.Errorf("duplicate: %v", dup)
	}
	doc := error(&DocumentError{Reason: "a document is a JSON object", Offset: -1})
	if !errors.Is(doc, ErrInvalidDocument) || doc.Error() != `syngo: invalid document: a document is a JSON object` {
		t.Errorf("document: %v", doc)
	}
	doc = &DocumentError{Reason: "unexpected end", Offset: 12}
	if doc.Error() != `syngo: invalid document: unexpected end (at byte 12)` {
		t.Errorf("document with a place: %v", doc)
	}
	conflict := error(&ConflictError{Attempts: 32})
	if !errors.Is(conflict, ErrConflict) || conflict.Error() != `syngo: the transaction conflicted with others 32 times in a row and gave up` {
		t.Errorf("conflict: %v", conflict)
	}
	q := error(&QueryError{Reason: "no such thing"})
	if !errors.Is(q, ErrInvalidQuery) || q.Error() != `syngo: invalid query: no such thing` {
		t.Errorf("query: %v", q)
	}
	q = &QueryError{Reason: `expected a value after "=", found "and"`, Line: 1, Column: 10, Text: "status = and age > 3"}
	want := "syngo: invalid query: line 1, column 10: expected a value after \"=\", found \"and\"\n\tstatus = and age > 3\n\t         ^"
	if q.Error() != want {
		t.Errorf("query with a place:\n%s\nwant\n%s", q, want)
	}
	// A tab before the place stays a tab in the line of the mark, so the two keep in step.
	q = &QueryError{Reason: "x", Line: 2, Column: 4, Text: "\ta = ?"}
	if got := q.Error(); !strings.HasSuffix(got, "\n\t\ta = ?\n\t\t  ^") {
		t.Errorf("query with a tab: %q", got)
	}
	q = &QueryError{Reason: "x", Line: 3, Column: 5}
	if q.Error() != "syngo: invalid query: line 3, column 5: x" {
		t.Errorf("query without its line: %v", q)
	}
	// Every error is itself and no other.
	all := []error{ErrNotFound, ErrExists, ErrDuplicate, ErrConflict, ErrReadOnly, ErrTxDone, ErrTxExpired, ErrOutcomeUnknown,
		ErrClosed, ErrInvalidDocument, ErrInvalidQuery, ErrInvalidIndex, ErrInvalidName, ErrFailed, ErrTooLarge}
	seen := map[string]bool{}
	for i, a := range all {
		if !strings.HasPrefix(a.Error(), "syngo: ") || seen[a.Error()] {
			t.Errorf("the message %q is not one of this package, or is that of another error", a)
		}
		seen[a.Error()] = true
		for j, b := range all {
			if errors.Is(a, b) != (i == j) {
				t.Errorf("%v is %v: %v", a, b, errors.Is(a, b))
			}
		}
	}
}

func TestIdentifiers(t *testing.T) {
	const n = 20000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = NewID()
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if len(id) != 26 || seen[id] {
			t.Fatalf("identifier %d is %q: not 26 characters, or made twice", i, id)
		}
		seen[id] = true
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("identifiers made one after the other do not sort in the order they were made")
	}
	now := time.Now()
	made, ok := IDTime(ids[n-1])
	if !ok || made.After(now.Add(time.Second)) || made.Before(now.Add(-time.Minute)) {
		t.Errorf("the time of an identifier just made: %v, %v", made, ok)
	}
	for _, bad := range []string{"", "short", strings.Repeat("0", 27), strings.Repeat("U", 26), "8" + strings.Repeat("0", 25), strings.Repeat("a", 26)} {
		if _, ok := IDTime(bad); ok {
			t.Errorf("%q was read as an identifier of NewID", bad)
		}
	}
}

func TestIdentifiersOfATime(t *testing.T) {
	// Times far ahead of the machine's clock, so that nothing made meanwhile comes between.
	at := time.Date(2190, 5, 6, 7, 8, 9, 123000000, time.UTC)
	a := NewIDAt(at)
	b := NewIDAt(at)
	earlier := NewIDAt(at.Add(-time.Hour))
	later := NewIDAt(at.Add(time.Hour))
	if got, ok := IDTime(a); !ok || !got.Equal(at) {
		t.Errorf("the time of an identifier made for %v is %v, %v", at, got, ok)
	}
	if !(a < b && b < earlier && earlier < later) {
		t.Errorf("identifiers do not sort in the order they were made: %s %s %s %s", a, b, earlier, later)
	}
	if got, _ := IDTime(later); !got.Equal(at.Add(time.Hour)) {
		t.Errorf("the time of the last one is %v", got)
	}

	floor := IDFloor(at.Add(2 * time.Hour))
	if len(floor) != 26 || !(later < floor) {
		t.Errorf("the floor of a later time, %s, does not sort above %s", floor, later)
	}
	if got, ok := IDTime(floor); !ok || !got.Equal(at.Add(2*time.Hour)) {
		t.Errorf("the time of a floor: %v, %v", got, ok)
	}
	if next := NewIDAt(at.Add(2 * time.Hour)); next < floor {
		t.Errorf("an identifier made at a time, %s, sorts below the floor of that time, %s", next, floor)
	}
	if IDFloor(time.Unix(-5, 0)) != strings.Repeat("0", 26) {
		t.Errorf("the floor of a time before 1970 is not the smallest identifier")
	}
}

// A Document is made for every document a program reads: its size is what a read costs in
// memory. What few documents have is kept behind one pointer, so that it stays at 64 bytes.
func TestDocumentStaysSmall(t *testing.T) {
	if size := unsafe.Sizeof(Document{}); size > 64 {
		t.Errorf("a Document takes %d bytes, and was made to take 64", size)
	}
}
