package syngo_test

import (
	"context"
	"errors"
	"iter"
	"sort"
	"testing"

	"github.com/syngodb/syngo"
)

// A stand in for an implementation: concrete types of its own, whose methods return its own
// concrete types, held to the contracts by the compiler. It keeps documents in a map, and answers
// only what the test below asks. What it proves is the shape: that the contracts can be met
// without one call going through an interface, and that code written against them runs on
// whatever meets them.

var (
	_ syngo.DB[*fakeCollection, *fakeTx] = (*fakeDB)(nil)
	_ syngo.Collection[*fakeQuery]       = (*fakeCollection)(nil)
	_ syngo.Query[*fakeQuery]            = (*fakeQuery)(nil)
	_ syngo.Tx[*fakeCollection]          = (*fakeTx)(nil)
)

type fakeDB struct {
	docs   map[string]map[string][]byte
	closed bool
}

type fakeCollection struct {
	db   *fakeDB
	name string
}

type fakeQuery struct {
	c     *fakeCollection
	cond  syngo.Cond
	limit int
}

type fakeTx struct{ db *fakeDB }

func (db *fakeDB) Collection(name string) *fakeCollection { return &fakeCollection{db: db, name: name} }

func (db *fakeDB) Collections(ctx context.Context) ([]string, error) {
	var names []string
	for name := range db.docs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (db *fakeDB) DropCollection(ctx context.Context, name string) error {
	delete(db.docs, name)
	return nil
}

func (db *fakeDB) Update(ctx context.Context, fn func(tx *fakeTx) error) error {
	return fn(&fakeTx{db: db})
}
func (db *fakeDB) View(ctx context.Context, fn func(tx *fakeTx) error) error {
	return fn(&fakeTx{db: db})
}
func (db *fakeDB) Begin(ctx context.Context) (*fakeTx, error) { return &fakeTx{db: db}, nil }
func (db *fakeDB) BeginView(ctx context.Context) (*fakeTx, error) {
	return &fakeTx{db: db}, nil
}
func (db *fakeDB) Close() error { db.closed = true; return nil }

func (tx *fakeTx) Collection(name string) *fakeCollection { return tx.db.Collection(name) }
func (tx *fakeTx) Commit(ctx context.Context) error       { return nil }
func (tx *fakeTx) Rollback()                              {}

func (c *fakeCollection) Name() string { return c.name }

func (c *fakeCollection) Get(ctx context.Context, id string) (*syngo.Document, error) {
	text, ok := c.db.docs[c.name][id]
	if !ok {
		return nil, syngo.ErrNotFound
	}
	return syngo.NewDocument(id, text, syngo.DocumentInfo{Revision: 1}), nil
}

func (c *fakeCollection) Exists(ctx context.Context, id string) (bool, error) {
	_, ok := c.db.docs[c.name][id]
	return ok, nil
}

func (c *fakeCollection) Count(ctx context.Context) (int64, error) {
	return int64(len(c.db.docs[c.name])), nil
}

func (c *fakeCollection) Put(ctx context.Context, doc any, options ...syngo.WriteOption) (string, error) {
	text, err := syngo.PlainDocument(doc)
	if err != nil {
		return "", err
	}
	if _, err := syngo.PlainWrite(options); err != nil {
		return "", err
	}
	var with struct {
		ID string `json:"_id"`
	}
	if err := syngo.DecodeJSON(text, &with); err != nil {
		return "", &syngo.DocumentError{Reason: err.Error(), Offset: -1}
	}
	if with.ID == "" {
		return "", &syngo.DocumentError{Reason: "this stand in gives no identifiers", Offset: -1}
	}
	if c.db.docs[c.name] == nil {
		c.db.docs[c.name] = map[string][]byte{}
	}
	c.db.docs[c.name][with.ID] = text
	return with.ID, nil
}

func (c *fakeCollection) Insert(ctx context.Context, doc any, options ...syngo.WriteOption) (string, error) {
	return c.Put(ctx, doc, options...)
}

func (c *fakeCollection) Replace(ctx context.Context, doc any, options ...syngo.WriteOption) error {
	_, err := c.Put(ctx, doc, options...)
	return err
}

func (c *fakeCollection) PutMany(ctx context.Context, docs []any, options ...syngo.WriteOption) ([]string, error) {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		id, err := c.Put(ctx, doc, options...)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

func (c *fakeCollection) Patch(ctx context.Context, id string, patch any, options ...syngo.WriteOption) error {
	return errors.ErrUnsupported
}

func (c *fakeCollection) Delete(ctx context.Context, id string) error {
	delete(c.db.docs[c.name], id)
	return nil
}

func (c *fakeCollection) Where(conds ...syngo.Cond) *fakeQuery {
	return &fakeQuery{c: c, cond: syngo.And(conds...), limit: -1}
}

func (c *fakeCollection) Query(text string, args ...syngo.Args) *fakeQuery {
	return &fakeQuery{c: c, cond: syngo.Not(syngo.Cond{}), limit: -1}
}

func (c *fakeCollection) EnsureIndex(ctx context.Context, def syngo.Index) error { return nil }
func (c *fakeCollection) RebuildIndex(ctx context.Context, name string) error    { return nil }
func (c *fakeCollection) DropIndex(ctx context.Context, name string) error       { return nil }
func (c *fakeCollection) Indexes(ctx context.Context) ([]syngo.Index, error)     { return nil, nil }
func (q *fakeQuery) OrderBy(order ...syngo.Order) *fakeQuery                     { return q }
func (q *fakeQuery) Offset(n int) *fakeQuery                                     { return q }
func (q *fakeQuery) After(token string) *fakeQuery                               { return q }
func (q *fakeQuery) Delete(ctx context.Context) (int64, error)                   { return 0, errors.ErrUnsupported }
func (q *fakeQuery) Explain(ctx context.Context) (string, error)                 { return "reads every document", nil }
func (q *fakeQuery) Page(ctx context.Context, size int) (*syngo.Page, error) {
	return nil, errors.ErrUnsupported
}
func (q *fakeQuery) Limit(n int) *fakeQuery { next := *q; next.limit = n; return &next }
func (q *fakeQuery) First(ctx context.Context) (*syngo.Document, error) {
	docs, err := q.Limit(1).Slice(ctx)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, syngo.ErrNotFound
	}
	return docs[0], nil
}

// holds says a document satisfies a condition, for the two comparisons the stand in knows: it
// reads the condition as the plain data it is.
func holds(c syngo.Cond, text []byte) (bool, error) {
	var out bool
	switch c.Op() {
	case syngo.OpAnd:
		out = true
		for _, kid := range c.Conds() {
			ok, err := holds(kid, text)
			if err != nil {
				return false, err
			}
			out = out && ok
		}
	case syngo.OpOr:
		for _, kid := range c.Conds() {
			ok, err := holds(kid, text)
			if err != nil {
				return false, err
			}
			out = out || ok
		}
	case syngo.OpEq:
		var doc map[string]syngoRaw
		if err := syngo.DecodeJSON(text, &doc); err != nil {
			return false, err
		}
		out = string(doc[c.Field()]) == string(c.Values()[0])
	default:
		return false, errors.ErrUnsupported
	}
	return out != c.Negated(), nil
}

// syngoRaw keeps a member of a document as the text it is.
type syngoRaw []byte

func (r *syngoRaw) UnmarshalJSON(text []byte) error {
	*r = append((*r)[:0], text...)
	return nil
}

func (q *fakeQuery) All(ctx context.Context) iter.Seq2[*syngo.Document, error] {
	return func(yield func(*syngo.Document, error) bool) {
		if err := q.cond.Err(); err != nil {
			yield(nil, err)
			return
		}
		var ids []string
		for id := range q.c.db.docs[q.c.name] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		left := q.limit
		for _, id := range ids {
			text := q.c.db.docs[q.c.name][id]
			ok, err := holds(q.cond, text)
			if err != nil {
				yield(nil, err)
				return
			}
			if !ok {
				continue
			}
			if left == 0 {
				return
			}
			left--
			if !yield(syngo.NewDocument(id, text, syngo.DocumentInfo{Revision: 1}), nil) {
				return
			}
		}
	}
}

func (q *fakeQuery) Slice(ctx context.Context) ([]*syngo.Document, error) {
	var out []*syngo.Document
	for doc, err := range q.All(ctx) {
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func (q *fakeQuery) IDs(ctx context.Context) ([]string, error) {
	docs, err := q.Slice(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID()
	}
	return ids, nil
}

func (q *fakeQuery) Count(ctx context.Context) (int64, error) {
	docs, err := q.Slice(ctx)
	return int64(len(docs)), err
}

// exercise uses a database through the contracts alone. Its type parameters are found by the
// compiler from the database it is given.
func exercise[D syngo.DB[C, T], C syngo.Collection[Q], Q syngo.Query[Q], T syngo.Tx[C]](ctx context.Context, db D) ([]string, error) {
	users := db.Collection("users")
	type user struct {
		ID     string `json:"_id"`
		Status string `json:"status"`
		Age    int    `json:"age"`
	}
	if _, err := users.PutMany(ctx, []any{
		user{"a", "active", 30},
		`{"_id":"b","status":"gone","age":30}`,
		[]byte(`{"_id":"c","status":"active","age":41}`),
	}, syngo.Meta(`{"by":"test"}`)); err != nil {
		return nil, err
	}
	err := db.Update(ctx, func(tx T) error {
		doc, err := tx.Collection("users").Get(ctx, "a")
		if err != nil {
			return err
		}
		_, err = tx.Collection("users").Put(ctx, doc)
		return err
	})
	if err != nil {
		return nil, err
	}
	if _, err := users.Get(ctx, "nobody"); !errors.Is(err, syngo.ErrNotFound) {
		return nil, errors.New("a document that is not there was found")
	}
	q := users.Where(syngo.Eq("status", "active"), syngo.Or(syngo.Eq("age", 30), syngo.Ne("age", 30)))
	if n, err := q.Count(ctx); err != nil || n != 2 {
		return nil, errors.New("the count is not 2")
	}
	first, err := q.OrderBy(syngo.Asc("age")).Limit(1).First(ctx)
	if err != nil || first.ID() != "a" {
		return nil, errors.New("the first is not a")
	}
	return q.IDs(ctx)
}

func TestContractsAreMet(t *testing.T) {
	db := &fakeDB{docs: map[string]map[string][]byte{}}
	ids, err := exercise(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "c" {
		t.Errorf("got %v", ids)
	}
	if err := db.Close(); err != nil || !db.closed {
		t.Errorf("close: %v", err)
	}
}
