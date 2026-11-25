package syngo

import (
	"context"
	"iter"
)

// The contracts: what a database does, whoever implements it. The embedded library and the
// driver of SyngoDB Server both do, with concrete types of their own, and each says so in its
// source in a line the compiler checks:
//
//	var _ syngo.Collection[*Query] = (*Collection)(nil)
//
// The interfaces are generic in the types their methods return (a collection returns its own
// kind of query, a database its own kind of collection and of transaction), so a program that
// uses one implementation calls its methods directly, and never through an interface. Code that
// must run on either is written against the contracts, with type parameters the compiler finds
// by itself:
//
//	func count[D syngo.DB[C, T], C syngo.Collection[Q], Q syngo.Query[Q], T syngo.Tx[C]](ctx context.Context, db D) (int64, error) {
//		return db.Collection("users").Where(syngo.Eq("status", "active")).Count(ctx)
//	}
//
// What the contracts leave out belongs to one implementation: how a database is opened and with
// which options, the views of a collection through a Go type (Typed), and what only makes sense
// where the files are (backups, checkpoints, compaction).

// Query is a question asked of a collection: conditions, an order, a window. Q is the type that
// implements it. Each method that refines a query returns a new one and leaves the first
// untouched, so a query can be kept and refined several ways. Nothing is read until one of the
// methods that answer is called.
//
// An answer has exactly one order: the order asked for, and, between documents that are equal on
// it, the order of their identifiers (descending when the last field of the order is). A query
// with no order answers in identifier order. An answer is read from one moment of the database:
// writes made while it is being read do not show in it. An answer has no size limit.
type Query[Q any] interface {
	// OrderBy orders the answer by the fields, in turn. It replaces the order the query had, the
	// one its text stated included; with no argument it takes the order away.
	OrderBy(order ...Order) Q
	// Limit keeps at most n documents of the answer: none of them when n is zero. A negative n
	// takes away the limit the query had, the one its text stated included.
	Limit(n int) Q
	// Offset skips the first n documents of the answer. It is skipped once: a query that resumes
	// its answer with After skips nothing more.
	Offset(n int) Q
	// After resumes the answer after the place a token stands for: the token is Page.Next of the
	// page before, from a query with the same order. An empty token is the beginning of the
	// answer. Nothing is kept between two pages.
	After(token string) Q

	// All answers the query as a sequence: for doc, err := range q.All(ctx). The sequence ends
	// after the first error. Everything the query holds is released when the loop ends, however
	// it ends.
	All(ctx context.Context) iter.Seq2[*Document, error]
	// Slice answers the query as a slice.
	Slice(ctx context.Context) ([]*Document, error)
	// First answers with the first document of the answer, or ErrNotFound when the answer is
	// empty.
	First(ctx context.Context) (*Document, error)
	// IDs answers with the identifiers of the documents, in the order of the answer.
	IDs(ctx context.Context) ([]string, error)
	// Count is the number of documents in the answer.
	Count(ctx context.Context) (int64, error)
	// Page answers with the next size documents of the answer, and the token to go on from there:
	//
	//	page, err := q.Page(ctx, 50)
	//	for err == nil && page.Next != "" {
	//		page, err = q.After(page.Next).Page(ctx, 50)
	//	}
	//
	// The pages, put end to end, are the answer, no more and no less.
	Page(ctx context.Context, size int) (*Page, error)
	// Delete removes the documents of the answer, its window included, and returns how many it
	// removed. In a transaction the removals are part of it. Outside one, an answer of any size
	// is removed a part at a time, and what was removed stays removed when Delete fails.
	Delete(ctx context.Context) (int64, error)
	// Explain says how the query would be answered: the way the documents are found, whether each
	// is then checked against the conditions, and how the answer gets its order.
	Explain(ctx context.Context) (string, error)
}

// Collection is a handle on one collection of a database: a set of JSON documents, of any shape,
// each under its own identifier. Q is its type of query. A handle costs nothing to make and may
// be kept, copied and shared between goroutines; it never fails and touches nothing, since a
// collection comes into being when something is first stored in it.
//
// A document is a JSON object. Its identifier is its member "_id", a string of 1 to MaxIDLen
// bytes; a document stored without one, or with an empty one, is given one (see NewID), written
// into the document.
type Collection[Q any] interface {
	// Name is the collection's name.
	Name() string

	// Get returns the document with the identifier, or ErrNotFound.
	Get(ctx context.Context, id string) (*Document, error)
	// Exists says whether a document has the identifier.
	Exists(ctx context.Context, id string) (bool, error)
	// Count is the number of documents in the collection, exactly: documents that have expired
	// are not counted.
	Count(ctx context.Context) (int64, error)

	// Put stores a document, replacing the one that has its identifier if there is one, and
	// returns the identifier. The document is JSON text ([]byte or string) or any Go value, which
	// is encoded by its json tags.
	Put(ctx context.Context, doc any, options ...WriteOption) (id string, err error)
	// Insert stores a new document and returns its identifier, or ErrExists when a document
	// already has the identifier.
	Insert(ctx context.Context, doc any, options ...WriteOption) (id string, err error)
	// Replace stores a document in the place of the one that has its identifier, or returns
	// ErrNotFound when there is none.
	Replace(ctx context.Context, doc any, options ...WriteOption) error
	// PutMany stores documents, replacing the ones whose identifiers are taken, and returns their
	// identifiers in the order given: all of them are stored, or none.
	PutMany(ctx context.Context, docs []any, options ...WriteOption) ([]string, error)
	// Patch changes some members of a document and leaves the others as they are. The patch is a
	// JSON object laid over the document, as a JSON merge patch is (RFC 7386). It returns
	// ErrNotFound when there is no such document. The document keeps its identifier and, unless
	// the options say otherwise, the time it expires at and its metadata.
	Patch(ctx context.Context, id string, patch any, options ...WriteOption) error
	// Delete removes the document with the identifier. Deleting a document that is not there is
	// not an error.
	Delete(ctx context.Context, id string) error

	// Where asks for the documents that satisfy every condition; with no conditions, for every
	// document.
	Where(conds ...Cond) Q
	// Query asks the collection a question written in SyngoQL, the query language, with the
	// values of its parameters:
	//
	//	users.Query(`age >= :min and status = "active" order by age desc limit 20`, syngo.Args{"min": 18})
	//
	// A text that is not a query makes every answer fail with a *QueryError that says where and
	// why.
	Query(text string, args ...Args) Q

	// EnsureIndex makes sure the collection has the index, and returns once the index is ready.
	// Declaring again an index that exists is not an error; declaring one under a name that
	// another definition holds is. A unique index that the present documents violate is refused
	// with a *DuplicateError, and is not kept. When ctx ends, EnsureIndex stops waiting and the
	// build goes on.
	EnsureIndex(ctx context.Context, def Index) error
	// RebuildIndex builds an index again from the documents, or returns ErrNotFound when there is
	// no such index. It is a repair: an index never needs it to stay right.
	RebuildIndex(ctx context.Context, name string) error
	// DropIndex removes an index. Dropping an index that is not there is not an error.
	DropIndex(ctx context.Context, name string) error
	// Indexes lists the collection's indexes, by name.
	Indexes(ctx context.Context) ([]Index, error)
}

// Tx is a transaction: a set of reads and writes that happen as one, or not at all. C is its type
// of collection.
//
// A transaction sees the database as it was when it began, plus its own writes. Nothing it writes
// can be seen by anyone else until it commits, and then all of it is seen at once. Transactions
// are strictly serializable: the result is as if they had run one at a time, in the order they
// committed, queries included. Nothing is locked while a transaction runs: when it commits, what
// it read is checked against what was committed since it began.
//
// A Tx is for one goroutine at a time.
type Tx[C any] interface {
	// Collection returns the collection with that name, as the transaction sees it: the documents
	// read, written and asked for through it are part of the transaction. Its indexes are not:
	// they are declared, rebuilt and dropped through the collection of the database.
	Collection(name string) C
	// Commit writes what the transaction did, all of it or none, and ends the transaction,
	// whatever it returns. It returns an error that is ErrConflict when what the transaction read
	// was changed by another since it began: nothing is written then. It is for a transaction
	// made by Begin; one that Update or View runs is ended by what its function returns.
	Commit(ctx context.Context) error
	// Rollback ends the transaction without writing anything. Rolling back a transaction that has
	// ended is not an error.
	Rollback()
}

// DB is an open database. C is its type of collection and T its type of transaction. It is safe
// for use by any number of goroutines.
type DB[C, T any] interface {
	// Collection returns a handle on the collection with that name. A name is 1 to 255 bytes of
	// valid UTF-8; what is first done through the handle of any other name fails with an error
	// that is ErrInvalidName.
	Collection(name string) C
	// Collections lists the names of the collections, in order.
	Collections(ctx context.Context) ([]string, error)
	// DropCollection removes a collection with its documents and indexes. Dropping a collection
	// that is not there is not an error.
	DropCollection(ctx context.Context, name string) error

	// Update runs fn in a transaction and commits what it wrote, all of it or none. When fn
	// returns an error nothing is written and Update returns that error. When what fn read was
	// changed by someone else before it could commit, fn is run again, from the start, on the
	// database as it then is: fn may run more than once, and must have no effect outside the
	// transaction that running it twice would repeat. Everything fn does must go through tx.
	Update(ctx context.Context, fn func(tx T) error) error
	// View runs fn in a transaction that only reads: everything fn reads, it reads from one
	// moment of the database, whatever is written meanwhile. fn runs once.
	View(ctx context.Context, fn func(tx T) error) error
	// Begin starts a transaction that the caller ends itself, with Commit or Rollback. Every
	// transaction begun must be ended. Nothing is run again here: when Commit returns an error
	// that is ErrConflict, nothing was written, and it is for the caller to begin again. One
	// that stays open for longer than the database can follow it is told so with ErrTxExpired,
	// at its next write or at its commit. Wherever the transaction fits in a function, Update is
	// simpler and safer.
	Begin(ctx context.Context) (T, error)
	// BeginView starts a transaction that only reads and that the caller ends itself, with
	// Commit or Rollback: Begin for what View does. It reads the moment it began for as long as
	// it is open, is checked against nothing, and never ends with a conflict.
	BeginView(ctx context.Context) (T, error)

	// Close ends the use of the database. Every later operation returns ErrClosed.
	Close() error
}
