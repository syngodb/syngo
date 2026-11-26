package syngo

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// This file is for whoever implements the contracts: the embedded library, the driver, the
// server that rebuilds what a driver sent. A program that only uses a database needs none of it.
//
// The rule it serves: what a program hands to a database (a value to compare with, a document,
// the options of a write) is turned into plain data here, once, by one piece of code. Every
// implementation starts from the same plain data, so no two of them can read a Go value
// differently.

// Op is what a condition is: a comparison of one field, or a combination of conditions.
type Op uint8

const (
	// OpAnd holds when every condition of Conds holds; with none, for every document.
	OpAnd Op = iota + 1
	// OpOr holds when any condition of Conds holds; with none, for no document.
	OpOr
	// OpEq holds when the field equals the value.
	OpEq
	// OpGt, OpGte, OpLt and OpLte are the ranges against one value.
	OpGt
	OpGte
	OpLt
	OpLte
	// OpBetween is the range between two values, both included.
	OpBetween
	// OpIn holds when the field equals any of the values; with none, for no document.
	OpIn
	// OpExists holds when the field is there and is not null, and OpMissing when it is not there
	// or is null. They have no value.
	OpExists
	OpMissing
	// OpContains holds when the field, a list or a text, holds the value.
	OpContains
	// OpStartsWith and OpEndsWith hold when the field is a string that begins, or ends, with the
	// value, which is a string.
	OpStartsWith
	OpEndsWith
	// OpLike holds when the field is a string that matches the value, a pattern, ignoring case.
	OpLike
	// OpMatches and OpMatchesAny are the full text searches: the value is the expression, a
	// string, and Language names the language when the search names one.
	OpMatches
	OpMatchesAny
)

// Err is why the condition cannot be one, or nil. A condition that has an error has nothing else:
// the query it is given to fails with this error whenever it is asked to answer.
func (c Cond) Err() error { return c.err }

// Op is what the condition is. The zero Cond is OpAnd without conditions: it holds for every
// document.
func (c Cond) Op() Op {
	if c.n == nil {
		return OpAnd
	}
	return c.n.op
}

// Negated says the condition holds exactly when what Op, Field, Values and Conds describe does
// not.
func (c Cond) Negated() bool { return c.n != nil && c.n.neg }

// Field is the path of the field a comparison is about, as the program wrote it. It is empty for
// OpAnd and OpOr.
func (c Cond) Field() string {
	if c.n == nil {
		return ""
	}
	return c.n.field
}

// Values are the values of a comparison, each in its plain form (see PlainValue): none for
// OpExists and OpMissing, two for OpBetween, any number for OpIn, one for every other comparison.
// They are not to be changed.
func (c Cond) Values() [][]byte {
	if c.n == nil {
		return nil
	}
	return c.n.values
}

// Language is the language a full text search names, or empty when it names none.
func (c Cond) Language() string {
	if c.n == nil {
		return ""
	}
	return c.n.language
}

// Conds are the conditions OpAnd and OpOr combine. They are not to be changed.
func (c Cond) Conds() []Cond {
	if c.n == nil {
		return nil
	}
	return c.n.conds
}

// PlainCond makes a condition from its plain description: what Op, Negated, Field, Values,
// Language and Conds say of one. It is for a server, which rebuilds the condition a driver
// described to it. The values are JSON text and are kept as they are given: whoever answers the
// query checks them, and they must not be changed afterwards.
//
// A description no function of this package could have made (an Op that is none, conditions
// under a comparison, a value where none is taken) gives a condition with an error, which the
// query it is given to fails with.
func PlainCond(op Op, negated bool, field string, values [][]byte, language string, conds []Cond) Cond {
	var c Cond
	switch {
	case op == OpAnd || op == OpOr:
		if field != "" || len(values) != 0 || language != "" {
			return Cond{err: &QueryError{Reason: "a combination of conditions has no field, no value and no language of its own"}}
		}
		c = group(op, conds)
	case op >= OpEq && op <= OpMatchesAny:
		if len(conds) != 0 {
			return Cond{err: &QueryError{Reason: fmt.Sprintf("the comparison on %q has conditions under it", field)}}
		}
		takes := 1
		switch op {
		case OpExists, OpMissing:
			takes = 0
		case OpBetween:
			takes = 2
		case OpIn:
			takes = len(values)
		}
		if len(values) != takes {
			return Cond{err: &QueryError{Reason: fmt.Sprintf("the comparison on %q has %d values and takes %d", field, len(values), takes)}}
		}
		if language != "" {
			if op != OpMatches && op != OpMatchesAny {
				return Cond{err: &QueryError{Reason: fmt.Sprintf("the comparison on %q names a language, and only a full text search has one", field)}}
			}
			if strings.TrimSpace(language) == "" {
				return Cond{err: &QueryError{Reason: "the language of a full text search is named, and this name is empty"}}
			}
		}
		c = Cond{n: &condNode{op: op, field: field, values: values, language: language}}
	default:
		return Cond{err: &QueryError{Reason: fmt.Sprintf("a condition of a kind this version does not know (%d)", op)}}
	}
	if negated {
		return Not(c)
	}
	return c
}

// Field is the path of the field the answer is ordered by, as the program wrote it. It is empty
// for the order by relevance.
func (o Order) Field() string { return o.field }

// Descending says the highest come first; for the order by relevance, the documents that answer
// best.
func (o Order) Descending() bool { return o.desc }

// ByRelevance says the order is by the relevance of the documents, not by a field.
func (o Order) ByRelevance() bool { return o.relevance }

// PlainValue is the plain form of a Go value given to a condition or as the argument of a query:
// the JSON text of the value, which is what a document given the same value holds.
//
//   - nil is null; a boolean, an integer or a float of any size is itself (a float that is no
//     number, or is infinite, is refused, since no document holds one);
//   - a string is a JSON string, and must be valid UTF-8;
//   - a time.Time is a JSON string, the time as RFC 3339 writes it, with nanoseconds;
//   - a json.RawMessage and a json.Number are taken as they are, unchecked: whoever answers the
//     query checks that they are JSON;
//   - a value that says how it is written (json.Marshaler, encoding.TextMarshaler) is what it
//     writes;
//   - a named type over a boolean, a string or a number is taken as that;
//   - anything else is encoded by its json tags.
//
// The bytes returned are the caller's own.
func PlainValue(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		return strconv.AppendBool(nil, x), nil
	case string:
		return plainText(x)
	case int:
		return strconv.AppendInt(nil, int64(x), 10), nil
	case int64:
		return strconv.AppendInt(nil, x, 10), nil
	case int32:
		return strconv.AppendInt(nil, int64(x), 10), nil
	case int16:
		return strconv.AppendInt(nil, int64(x), 10), nil
	case int8:
		return strconv.AppendInt(nil, int64(x), 10), nil
	case uint:
		return strconv.AppendUint(nil, uint64(x), 10), nil
	case uint64:
		return strconv.AppendUint(nil, x, 10), nil
	case uint32:
		return strconv.AppendUint(nil, uint64(x), 10), nil
	case uint16:
		return strconv.AppendUint(nil, uint64(x), 10), nil
	case uint8:
		return strconv.AppendUint(nil, uint64(x), 10), nil
	case float64:
		return plainFloat(x)
	case float32:
		return plainFloat(float64(x))
	case time.Time:
		return appendQuoted(nil, x.Format(time.RFC3339Nano)), nil
	case json.RawMessage:
		return bytes.Clone(x), nil
	case json.Number:
		// A number, though the type is a string: it is written as one.
		return []byte(x), nil
	case json.Marshaler, encoding.TextMarshaler:
		// A value that says itself how it is written is compared as it is written, whatever
		// basic type it is made of.
		return plainEncoded(v)
	}
	// A named type over a basic one compares like the basic one.
	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.Bool:
		return strconv.AppendBool(nil, r.Bool()), nil
	case reflect.String:
		return plainText(r.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.AppendInt(nil, r.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.AppendUint(nil, r.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return plainFloat(r.Float())
	}
	return plainEncoded(v)
}

// plainEncoded is a Go value through its JSON encoding.
func plainEncoded(v any) ([]byte, error) {
	text, err := EncodeJSON(v)
	if err != nil {
		return nil, fmt.Errorf("cannot be encoded as JSON: %w", err)
	}
	return text, nil
}

// plainText is a text as a JSON string. No document holds text that is not valid UTF-8, and a
// query that compares with some is a mistake worth telling.
func plainText(s string) ([]byte, error) {
	if !utf8.ValidString(s) {
		return nil, errors.New("the text is not valid UTF-8")
	}
	return appendQuoted(make([]byte, 0, len(s)+2), s), nil
}

// plainFloat is a float as a JSON number that reads back as a float: a whole one keeps a
// fraction.
func plainFloat(f float64) ([]byte, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%v is not a number a document can hold", f)
	}
	text := strconv.AppendFloat(nil, f, 'g', -1, 64)
	if bytes.IndexAny(text, ".eE") < 0 {
		text = append(text, '.', '0')
	}
	return text, nil
}

const hexDigits = "0123456789abcdef"

// appendQuoted writes a text as a JSON string.
func appendQuoted(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		b := s[i]
		if b >= 0x20 && b != '"' && b != 0x5C && b < utf8.RuneSelf {
			i++
			continue
		}
		if b >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r != utf8.RuneError || size > 1 {
				i += size
				continue
			}
			dst = append(dst, s[start:i]...)
			dst = utf8.AppendRune(dst, utf8.RuneError)
			i++
			start = i
			continue
		}
		dst = append(dst, s[start:i]...)
		switch b {
		case '"':
			dst = append(dst, 0x5C, '"')
		case 0x5C:
			dst = append(dst, 0x5C, 0x5C)
		case '\n':
			dst = append(dst, 0x5C, 'n')
		case '\r':
			dst = append(dst, 0x5C, 'r')
		case '\t':
			dst = append(dst, 0x5C, 't')
		default:
			dst = append(dst, 0x5C, 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xF])
		}
		i++
		start = i
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// PlainList is the elements of a slice or an array handed over as one value, as In takes it and
// as the argument of "in :names" is read. ok is false for anything else. Bytes are not a list:
// they are JSON text, as everywhere.
func PlainList(v any) (elements []any, ok bool) {
	switch x := v.(type) {
	case nil, []byte, json.RawMessage:
		return nil, false
	case []any:
		return x, true
	}
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, r.Len())
	for i := range out {
		out[i] = r.Index(i).Interface()
	}
	return out, true
}

// PlainDocument is the JSON text of what a caller hands in as a document, or as a patch: text is
// taken as it is ([]byte, string, json.RawMessage), unchecked; a *Document is its text; anything
// else is encoded by its json tags.
//
// The text returned may be the very bytes that were handed in, or those of the Document: it is
// read, never changed.
func PlainDocument(doc any) ([]byte, error) {
	switch v := doc.(type) {
	case []byte:
		return v, nil
	case json.RawMessage:
		return v, nil
	case string:
		return []byte(v), nil
	case *Document:
		if v == nil {
			return nil, &DocumentError{Reason: "the document is nil", Offset: -1}
		}
		return v.text, nil
	case nil:
		return nil, &DocumentError{Reason: "the document is nil", Offset: -1}
	}
	text, err := EncodeJSON(doc)
	if err != nil {
		return nil, &DocumentError{Reason: "the value cannot be encoded as JSON: " + err.Error(), Offset: -1}
	}
	return text, nil
}

// Write is what the options of one write come to.
type Write struct {
	// Timed says an option stated when the document expires, "never" included. The document then
	// expires at ExpireAt when that is not the zero time, ExpireIn after it is written when that
	// is not zero, and never when both are zero. Without Timed, a write that keeps what the
	// document had (Patch, Modify) keeps its expiry, and any other stores a document that never
	// expires.
	Timed    bool
	ExpireIn time.Duration
	ExpireAt time.Time
	// HasMeta says an option stated the metadata. Meta is then its JSON text, unchecked, or nil
	// for none. A text that was given is never nil, not even one with nothing in it: that one is
	// not JSON, and whoever checks the text refuses it. Without HasMeta, a write that keeps what
	// the document had keeps its metadata, and any other stores a document without.
	HasMeta bool
	Meta    []byte
}

// PlainWrite works out what the options of a write come to: of two options that state the same
// thing, the last one counts.
func PlainWrite(options []WriteOption) (Write, error) {
	var w Write
	var meta any
	for i := range options {
		switch o := &options[i]; o.kind {
		case writeExpireIn:
			w.Timed, w.ExpireIn, w.ExpireAt = true, o.in, time.Time{}
		case writeExpireAt:
			w.Timed, w.ExpireAt, w.ExpireIn = true, o.at, 0
		case writeMeta:
			w.HasMeta, meta = true, o.meta
		}
	}
	switch v := meta.(type) {
	case nil:
	case []byte:
		w.Meta = textGiven(v)
	case json.RawMessage:
		w.Meta = textGiven(v)
	case string:
		w.Meta = textGiven([]byte(v))
	default:
		text, err := EncodeJSON(meta)
		if err != nil {
			return Write{}, &DocumentError{Reason: "the metadata cannot be encoded as JSON: " + err.Error(), Offset: -1}
		}
		w.Meta = text
	}
	return w, nil
}

// textGiven keeps a text that was given apart from no text: only the second is nil.
func textGiven(text []byte) []byte {
	if text == nil {
		return []byte{}
	}
	return text
}

// DocumentInfo is what a database keeps about a document, beside its text.
type DocumentInfo struct {
	// Revision counts how many times the document was written: 1 for a new document.
	Revision uint64
	// Modified is when the document was last written, in nanoseconds since 1970, or zero when
	// that is not known.
	Modified int64
	// Expires is when the document expires, in nanoseconds since 1970, or zero when it does not.
	Expires int64
	// Meta is what was stored beside the document, as JSON text, or nil.
	Meta []byte
}

// NewDocument makes a Document from what a database read: the identifier, the JSON text (compact,
// with the identifier in it) and what the database keeps about it. The Document keeps the bytes
// it is given, and hands out copies only: they must not be changed afterwards.
func NewDocument(id string, text []byte, info DocumentInfo) *Document {
	d := &Document{id: id, text: text, modified: info.Modified, revision: info.Revision}
	if info.Expires != 0 || info.Meta != nil {
		d.rare = &rareFacts{meta: info.Meta, expires: info.Expires}
	}
	return d
}

// DocumentParts is what NewDocument was given: the JSON text of a document and what the database
// keeps about it, where they lie and without a copy. It is for an implementation that writes them
// somewhere else, as a server does to its client. They are not to be changed.
func DocumentParts(d *Document) (text []byte, info DocumentInfo) {
	info = DocumentInfo{Revision: d.revision, Modified: d.modified}
	if d.rare != nil {
		info.Expires, info.Meta = d.rare.expires, d.rare.meta
	}
	return d.text, info
}
