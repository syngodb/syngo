// Package syngo is what a program that uses SyngoDB is written with, whether the database is
// embedded in the program or behind a server: the values (conditions, orders, documents, pages,
// index declarations, write options, errors) and the contracts a database keeps.
//
// The package holds no database. A database is opened with the embedded library, or with the
// driver of SyngoDB Server (github.com/syngodb/syngodrv). From there on a program reads the same,
// whichever it opened:
//
//	users := db.Collection("users")
//
//	id, err := users.Put(ctx, User{Name: "Ann", Age: 31})
//	doc, err := users.Get(ctx, id)
//	if errors.Is(err, syngo.ErrNotFound) {
//		...
//	}
//
//	q := users.Where(syngo.Gte("age", 18), syngo.Eq("status", "active")).
//		OrderBy(syngo.Desc("age")).Limit(20)
//	for doc, err := range q.All(ctx) {
//		...
//	}
//
//	err = users.EnsureIndex(ctx, syngo.Index{Name: "active_by_age", Fields: []string{"status", "age desc"}})
//
// Values. Each thing has one type and one name, here: a Cond is a Cond for the library and for the
// driver, an error met through a server is the error met embedded (errors.Is tells them apart the
// same way), and a Document read from either is the same Document.
//
// Contracts. DB, Collection, Query and Tx say what a database does, as interfaces. They are
// generic in the types their methods return, so that the library and the driver implement them
// with their own concrete types and no call goes through an interface. Both declare that they
// comply, which the compiler checks, and code written against the contracts runs on either:
//
//	func active[Q syngo.Query[Q]](users syngo.Collection[Q]) Q {
//		return users.Where(syngo.Eq("status", "active"))
//	}
//
// Whether a program embeds its database or talks to a server is decided when it is built, by what
// it imports. Nothing here chooses at run time.
//
// Conditions are descriptions. A Cond says what is asked (a field, a comparison, the values) and
// holds no code that evaluates it: whoever answers the query does that. So a condition means the
// same thing wherever it is answered, and it can travel.
//
// For whoever implements the contracts. The functions and methods a program has no use for, and
// an implementation cannot do without, are in the file implement.go: reading a Cond, an Order
// and the options of a write as plain data, turning a Go value into the JSON text it stands for
// (PlainValue, PlainList, PlainDocument), and making a Document (NewDocument).
package syngo
