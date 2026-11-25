package syngo

import (
	"errors"
	"strconv"
)

// What a caller may want to tell apart is told apart with errors.Is, embedded and through a
// server alike. The errors that say more (which index, which document, where in the text of a
// query) are *DuplicateError, *DocumentError, *QueryError and *ConflictError.
var (
	// ErrNotFound is returned when the document asked for is not there.
	ErrNotFound = errors.New("syngo: not found")
	// ErrExists is returned by Insert when a document already has the identifier.
	ErrExists = errors.New("syngo: a document with this identifier already exists")
	// ErrDuplicate is returned when a write would give two documents the same value in a unique
	// index. The error is a *DuplicateError that names the index.
	ErrDuplicate = errors.New("syngo: the value is already taken in a unique index")
	// ErrConflict is returned when a transaction kept conflicting with others and gave up.
	ErrConflict = errors.New("syngo: the transaction conflicted with another")
	// ErrReadOnly is returned by every write on a database opened read only, and by every write
	// in a transaction that only reads (View).
	ErrReadOnly = errors.New("syngo: read only")
	// ErrTxDone is returned by whatever is done with a transaction after it has ended.
	ErrTxDone = errors.New("syngo: the transaction has ended")
	// ErrTxExpired is returned by the writes and by the commit of a transaction that was kept open
	// for longer than the database could keep what it needs to check it. Nothing of it is
	// written. It is not a conflict: running the same transaction again, as slowly, ends the same
	// way.
	ErrTxExpired = errors.New("syngo: the transaction was open for longer than the database could follow it")
	// ErrOutcomeUnknown is returned for a write, or the commit of a transaction, whose answer
	// never came: the connection to the server was lost after the request had left. The write may
	// have been made, or not. Nothing runs such a request again by itself, since doing it twice
	// is not doing it once: it is for the program to read what is there and decide. The embedded
	// library never returns it.
	ErrOutcomeUnknown = errors.New("syngo: the connection was lost before the answer came, and the write may or may not have been made")
	// ErrClosed is returned by every operation on a closed database.
	ErrClosed = errors.New("syngo: the database is closed")
	// ErrInvalidDocument is what every *DocumentError is.
	ErrInvalidDocument = errors.New("syngo: invalid document")
	// ErrInvalidQuery is what every *QueryError is: a query text that does not read, a condition
	// that cannot be compared, a token After cannot resume from.
	ErrInvalidQuery = errors.New("syngo: invalid query")
	// ErrInvalidIndex is what errors.Is finds in the error returned for an index that cannot be
	// declared as asked: its name, its fields, or a name the collection already gave to an index
	// declared otherwise.
	ErrInvalidIndex = errors.New("syngo: invalid index")
	// ErrInvalidName is what errors.Is finds in the error returned for a collection name that is
	// not one: a name is 1 to 255 bytes of valid UTF-8.
	ErrInvalidName = errors.New("syngo: invalid name")
	// ErrFailed is what errors.Is finds in the error of every write once a write could not be made
	// durable (the disk is full, the device failed). What was confirmed before is safe; nothing
	// more is written until the database is closed and opened again.
	ErrFailed = errors.New("syngo: a write failed, the database must be reopened")
	// ErrTooLarge is what errors.Is finds in the error of a commit that would write more than a
	// database takes in one piece: a PutMany of that many documents, a transaction that wrote
	// that much. Nothing of it is written; it is to be written in several commits.
	ErrTooLarge = errors.New("syngo: too much to write in one commit")
	// ErrCorrupt is what errors.Is finds in the error of whatever met, in the database, something
	// that cannot be read as what it must be: the record of a document, an entry of an index, a
	// count. Nothing a program writes makes it: the files of the database were damaged, or
	// changed by something else. The error says what was met.
	ErrCorrupt = errors.New("syngo: corrupt store")
)

// DuplicateError says which unique index refused a write, and which document holds the value.
type DuplicateError struct {
	// Collection and Index name the index.
	Collection, Index string
	// Holder is the identifier of the document that already has the value.
	Holder string
}

// Error names the index, the collection and the document that holds the value.
func (e *DuplicateError) Error() string {
	return "syngo: the value is already taken in the unique index " + strconv.Quote(e.Index) + " of " +
		strconv.Quote(e.Collection) + " by document " + strconv.Quote(e.Holder)
}

// Is makes errors.Is(err, ErrDuplicate) true.
func (e *DuplicateError) Is(target error) bool { return target == ErrDuplicate }

// DocumentError says why a document was refused.
type DocumentError struct {
	// Reason says what is wrong.
	Reason string
	// Offset is the byte of the JSON text the problem was met at, or -1 when it is not about a
	// place in the text.
	Offset int
}

// Error says what is wrong with the document, and where in its text when that is known.
func (e *DocumentError) Error() string {
	if e.Offset < 0 {
		return "syngo: invalid document: " + e.Reason
	}
	return "syngo: invalid document: " + e.Reason + " (at byte " + strconv.Itoa(e.Offset) + ")"
}

// Is makes errors.Is(err, ErrInvalidDocument) true.
func (e *DocumentError) Is(target error) bool { return target == ErrInvalidDocument }

// QueryError says why a query was refused.
type QueryError struct {
	// Reason says what is wrong.
	Reason string
	// Line and Column say where in the text of the query the problem is, counted from one. Both
	// are zero when the error is not about a place in a text.
	Line, Column int
	// Text is the line of the query the problem is on, when the error is about a place in a text.
	Text string
}

// Error says what is wrong and, for a query written as text, shows the line with a mark under the
// place:
//
//	syngo: invalid query: line 1, column 10: expected a value after "=", found "and"
//		status = and age > 3
//		         ^
func (e *QueryError) Error() string {
	if e.Line == 0 {
		return "syngo: invalid query: " + e.Reason
	}
	msg := "syngo: invalid query: line " + strconv.Itoa(e.Line) + ", column " + strconv.Itoa(e.Column) + ": " + e.Reason
	if e.Text == "" {
		return msg
	}
	// The mark stands under the column: what precedes it is spaces, a tab staying a tab so that
	// the two lines keep in step however tabs are shown.
	var pad []rune
	for i, r := range []rune(e.Text) {
		if i >= e.Column-1 {
			break
		}
		if r == '\t' {
			pad = append(pad, '\t')
		} else {
			pad = append(pad, ' ')
		}
	}
	for len(pad) < e.Column-1 {
		pad = append(pad, ' ')
	}
	return msg + "\n\t" + e.Text + "\n\t" + string(pad) + "^"
}

// Is makes errors.Is(err, ErrInvalidQuery) true.
func (e *QueryError) Is(target error) bool { return target == ErrInvalidQuery }

// ConflictError is returned by Update when a transaction kept finding, at commit, that what it
// read had changed, and gave up; and by the Commit of a transaction made by Begin, which runs
// once.
type ConflictError struct {
	// Attempts is how many times the transaction ran.
	Attempts int
}

// Error says how many times the transaction ran before it gave up.
func (e *ConflictError) Error() string {
	return "syngo: the transaction conflicted with others " + strconv.Itoa(e.Attempts) + " times in a row and gave up"
}

// Is makes errors.Is(err, ErrConflict) true.
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }
