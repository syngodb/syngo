# syngo

Core types and interfaces for all SyngoDB consumers.

It is what a Go program that uses SyngoDB is written with, whether the database is embedded in
the program or behind SyngoDB Server: the values (conditions, orders, documents, pages, index
declarations, the options of a write, errors) and the contracts a database keeps.

```go
import "github.com/syngodb/syngo"

users := db.Collection("users") // db: an embedded database, or one behind a server

q := users.Where(syngo.Gte("age", 18), syngo.Eq("status", "active")).
    OrderBy(syngo.Desc("age")).Limit(20)
for doc, err := range q.All(ctx) {
    ...
}

if _, err := users.Get(ctx, id); errors.Is(err, syngo.ErrNotFound) {
    ...
}
```

The package holds no database. One is opened with the embedded library or with the driver of
SyngoDB Server, [`syngodrv`](https://github.com/syngodb/syngodrv), and from there on a program
reads the same.

- **One name for each thing.** A `syngo.Cond` is a condition for the library and for the driver;
  an error met through a server is the error met embedded.
- **Contracts the compiler checks.** `DB`, `Collection`, `Query` and `Tx` are interfaces, generic
  in the types their methods return. Each implementation meets them with its own concrete
  types, so no call goes through an interface.
- **Conditions are descriptions.** A `Cond` says what is asked and holds no code that answers
  it, so it means the same wherever it is answered.

Its only dependency outside the standard library is
[`goccy/go-json`](https://github.com/goccy/go-json). It needs Go 1.26 or later.

**Not released yet.** Version 1.0.0 comes with SyngoDB 7. Until then anything here may change.
