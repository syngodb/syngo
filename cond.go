package syngo

import (
	"fmt"
	"strings"
)

// Cond is a condition on a document: a comparison of one field, or a combination of conditions
// made with And, Or and Not. Conditions are values: building one changes no other, and one can be
// reused in any number of queries. The zero Cond holds for every document.
//
// A field is named by its path ("age", "address.city"); names ignore case. A field that holds an
// array satisfies a comparison when any of its elements does.
//
// A condition is a description of what is asked, not the code that answers it: it means the same
// to an embedded database and to a server. A condition that cannot be one (a value that cannot be
// compared, a field name that is no path) makes every answer of the query it is given to fail
// with a *QueryError.
type Cond struct {
	n   *condNode
	err error
}

// condNode is what a condition says. It is never changed once made.
type condNode struct {
	op       Op
	neg      bool
	field    string
	values   [][]byte
	language string
	conds    []Cond
	// one is where the value of a comparison with one value is kept, which most have: the node
	// and the list of its values are then one piece of memory.
	one [1][]byte
}

func leaf(op Op, field string, operands ...any) Cond {
	n := &condNode{op: op, field: field}
	switch len(operands) {
	case 0:
	case 1:
		n.values = n.one[:]
	default:
		n.values = make([][]byte, len(operands))
	}
	for i, operand := range operands {
		text, err := PlainValue(operand)
		if err != nil {
			return Cond{err: &QueryError{Reason: fmt.Sprintf("the value compared with %q: %v", field, err)}}
		}
		n.values[i] = text
	}
	return Cond{n: n}
}

// Eq holds when the field equals the value. Equality with nil holds for a field that is null or
// missing. On an array field it holds when an element equals the value, or when the value is an
// array equal to the whole field.
func Eq(field string, value any) Cond { return leaf(OpEq, field, value) }

// Ne holds exactly when Eq does not: it also holds for a document that has no such field.
func Ne(field string, value any) Cond { return Not(Eq(field, value)) }

// Gt holds when the field is greater than the value. Like every range, it holds only between two
// values of one kind (two numbers, two strings, two times, two booleans).
func Gt(field string, value any) Cond { return leaf(OpGt, field, value) }

// Gte holds when the field is greater than or equal to the value.
func Gte(field string, value any) Cond { return leaf(OpGte, field, value) }

// Lt holds when the field is less than the value.
func Lt(field string, value any) Cond { return leaf(OpLt, field, value) }

// Lte holds when the field is less than or equal to the value.
func Lte(field string, value any) Cond { return leaf(OpLte, field, value) }

// Between holds when the field is inside the range, both ends included.
func Between(field string, low, high any) Cond { return leaf(OpBetween, field, low, high) }

// In holds when the field equals any of the values: it is Eq with each of them, joined by Or. The
// values are given one by one or as one list, so that a slice of any type can be handed over as
// it is: In("role", "admin", "owner") and In("role", roles) ask the same. With no value, or an
// empty list, it holds for no document.
//
// A field that equals a list, the whole of it, is asked for with Eq; among several values of In a
// list stands for itself.
func In(field string, values ...any) Cond {
	if len(values) == 1 {
		if list, ok := PlainList(values[0]); ok {
			values = list
		}
	}
	return leaf(OpIn, field, values...)
}

// Exists holds when the field is there and is not null.
func Exists(field string) Cond { return leaf(OpExists, field) }

// Missing holds when the field is not there, or is null.
func Missing(field string) Cond { return leaf(OpMissing, field) }

// Contains holds when the field holds the value, in the way the field calls for: a list holds it
// when one of its elements equals it (the elements themselves are not searched), and a text holds
// it when the value is text found inside it. Nothing else holds anything.
func Contains(field string, value any) Cond { return leaf(OpContains, field, value) }

// StartsWith holds when the field is a string that begins with the text.
func StartsWith(field, text string) Cond { return leaf(OpStartsWith, field, text) }

// EndsWith holds when the field is a string that ends with the text.
func EndsWith(field, text string) Cond { return leaf(OpEndsWith, field, text) }

// Like holds when the field is a string that matches the pattern, ignoring case. In the pattern %
// stands for any run of characters and _ for exactly one; a backslash takes the next character
// literally. It is the one comparison that ignores case.
func Like(field, pattern string) Cond { return leaf(OpLike, field, pattern) }

// QuoteLike returns a pattern for Like that matches the text itself, ignoring case: the
// characters a pattern gives a meaning to (%, _ and the backslash) are written so that they stand
// for themselves. It is for texts that come from outside the program, which would otherwise be
// read as patterns: somebody who types a percent sign means a percent sign.
//
//	Like("name", QuoteLike(typed))           // the name is what was typed, in any case
//	Like("name", QuoteLike(typed)+"%")       // the name begins with it
//	Like("name", "%"+QuoteLike(typed)+"%")   // the name holds it
//
// In a query text the pattern is given as a parameter: name like :p, with
// Args{"p": QuoteLike(typed)}.
func QuoteLike(text string) string {
	if !strings.ContainsAny(text, `%_\`) {
		return text
	}
	out := make([]byte, 0, len(text)+4)
	for i := 0; i < len(text); i++ {
		if c := text[i]; c == '%' || c == '_' || c == '\\' {
			out = append(out, '\\')
		}
		out = append(out, text[i])
	}
	return string(out)
}

// Matches holds when the field is text that satisfies a full text search: the words of the
// expression are looked for among the words of the field, as the language brings them together
// (in English, "running" finds "run" and "runs", and "the" is no word to look for).
//
// The expression is words between blanks, all of which the field must hold:
//
//	database engine     the field holds both words
//	"database engine"   the field holds the two words in a row
//	engin*              the field holds a word that begins so
//	engine~  engine~2   the field holds a word one edit away, or two
//	-legacy             the field does not hold the word (or the phrase, the beginning, ...)
//
// The field is a string, or a list whose strings are searched as one text (no phrase holds across
// two of them). Any other value satisfies no search, and neither does an expression that looks
// for nothing.
//
// The language is English unless one is named: Matches("body", "schnelle autos", "de"). "none" is
// no language at all, where words are only lower cased and stripped of their accents. A search
// means the same with or without an index; an index declared on the field for the same language
// ("body text", "body text de") answers it without reading the documents.
func Matches(field, expression string, language ...string) Cond {
	return matches(OpMatches, field, expression, language)
}

// MatchesAny is Matches for a field that holds any of the words, phrases, beginnings or
// approximate words of the expression, where Matches wants them all. What the expression excludes
// (-word) is excluded all the same.
func MatchesAny(field, expression string, language ...string) Cond {
	return matches(OpMatchesAny, field, expression, language)
}

func matches(op Op, field, expression string, language []string) Cond {
	c := leaf(op, field, expression)
	if c.n == nil {
		return c
	}
	switch len(language) {
	case 0:
	case 1:
		if strings.TrimSpace(language[0]) == "" {
			return Cond{err: &QueryError{Reason: "the language of a full text search is named, and this name is empty"}}
		}
		c.n.language = language[0]
	default:
		return Cond{err: &QueryError{Reason: "a full text search has one language"}}
	}
	return c
}

func group(op Op, conds []Cond) Cond {
	for _, c := range conds {
		if c.err != nil {
			return Cond{err: c.err}
		}
	}
	switch {
	case len(conds) == 1:
		// A group of one is that one.
		return conds[0]
	case len(conds) == 0 && op == OpAnd:
		// Every condition of none holds: it is the zero Cond.
		return Cond{}
	}
	// The conditions are copied: the slice they came in is the caller's, to change at will.
	return Cond{n: &condNode{op: op, conds: append([]Cond(nil), conds...)}}
}

// And holds when every condition holds. With no conditions it holds for every document.
func And(conds ...Cond) Cond { return group(OpAnd, conds) }

// Or holds when any condition holds. With no conditions it holds for none.
func Or(conds ...Cond) Cond { return group(OpOr, conds) }

// Not holds exactly when the condition does not.
func Not(c Cond) Cond {
	if c.err != nil {
		return c
	}
	if c.n == nil {
		// The complement of everything is nothing: an Or of no condition.
		return Cond{n: &condNode{op: OpOr}}
	}
	negated := *c.n
	negated.neg = !negated.neg
	return Cond{n: &negated}
}

// Order is one thing a query orders its answer by: a field, or the relevance of the documents.
type Order struct {
	field     string
	desc      bool
	relevance bool
}

// Relevance orders an answer by how well each document answers the full text searches of the
// query (Matches, MatchesAny), the best first:
//
//	notes.Where(syngo.Matches("body", "storage engine")).OrderBy(syngo.Relevance()).Limit(20)
//
// The relevance of a document grows with the number of times its text holds what a search looks
// for, and shrinks with the length of the text: a word found in a title of five words counts
// for more than the same word found once in a page of a thousand. It is worked out from the
// document and the search alone. No other document has a say in it, so the relevance of a
// document does not move when others are written, an answer ordered by it is read page by page
// like any other, and it is the same with or without an index. (It is not weighed by how rare
// each word is in the collection, as search engines do: that is the part that depends on every
// other document.)
//
// Every search of the query that is not under a Not counts, and adds what it finds; a document
// that satisfies no search has no relevance. Documents of the same relevance go by the next
// order given, then by identifier: the larger first when the relevance is the last order given,
// as after any descending order, which for the identifiers NewID makes is the newest first.
//
// In SyngoQL: order by relevance(), and order by relevance() asc for Relevance().Reverse().
func Relevance() Order { return Order{relevance: true, desc: true} }

// Reverse is the same order the other way round: Asc("age").Reverse() is Desc("age"), and
// Relevance().Reverse() puts first the documents that answer least well.
func (o Order) Reverse() Order {
	o.desc = !o.desc
	return o
}

// Asc orders by the field, lowest first.
func Asc(field string) Order { return Order{field: field} }

// Desc orders by the field, highest first.
func Desc(field string) Order { return Order{field: field, desc: true} }

// Args are the values of the parameters of a query written as text: Args{"min": 18} gives :min
// its value. A value is anything a Go condition takes: a number stays the number it is, a
// time.Time is a time, a slice given to "in :names" is the list of its elements.
type Args map[string]any
