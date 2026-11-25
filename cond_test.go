package syngo

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// show writes a condition as the plain data an implementation reads, for the tests to compare.
func show(c Cond) string {
	if err := c.Err(); err != nil {
		return "error(" + err.Error() + ")"
	}
	var b strings.Builder
	if c.Negated() {
		b.WriteString("not ")
	}
	names := map[Op]string{OpAnd: "and", OpOr: "or", OpEq: "eq", OpGt: "gt", OpGte: "gte", OpLt: "lt", OpLte: "lte",
		OpBetween: "between", OpIn: "in", OpExists: "exists", OpMissing: "missing", OpContains: "contains",
		OpStartsWith: "starts", OpEndsWith: "ends", OpLike: "like", OpMatches: "matches", OpMatchesAny: "matchesany"}
	name, ok := names[c.Op()]
	if !ok {
		name = fmt.Sprintf("op%d", c.Op())
	}
	b.WriteString(name + "(")
	if c.Op() == OpAnd || c.Op() == OpOr {
		for i, kid := range c.Conds() {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(show(kid))
		}
	} else {
		b.WriteString(c.Field())
		for _, v := range c.Values() {
			b.WriteString(", " + string(v))
		}
		if c.Language() != "" {
			b.WriteString("; " + c.Language())
		}
	}
	b.WriteString(")")
	return b.String()
}

func TestConditionsAsPlainData(t *testing.T) {
	roles := []string{"admin", "owner"}
	cases := []struct {
		c    Cond
		want string
	}{
		{Cond{}, `and()`},
		{Eq("age", 30), `eq(age, 30)`},
		{Eq("Address.City", "Turin"), `eq(Address.City, "Turin")`},
		{Eq("gone", nil), `eq(gone, null)`},
		{Ne("age", 30), `not eq(age, 30)`},
		{Gt("a", 1), `gt(a, 1)`},
		{Gte("a", 1.5), `gte(a, 1.5)`},
		{Lt("a", uint8(2)), `lt(a, 2)`},
		{Lte("a", "x"), `lte(a, "x")`},
		{Between("a", 1, 9), `between(a, 1, 9)`},
		{In("role", "admin", "owner"), `in(role, "admin", "owner")`},
		{In("role", roles), `in(role, "admin", "owner")`},
		{In("role", []any{1, "x", nil}), `in(role, 1, "x", null)`},
		{In("role"), `in(role)`},
		{In("role", []string{}), `in(role)`},
		{In("tags", []int{1}, []int{2}), `in(tags, [1], [2])`},
		{In("role", "admin"), `in(role, "admin")`},
		{Exists("a"), `exists(a)`},
		{Missing("a"), `missing(a)`},
		{Not(Exists("a")), `not exists(a)`},
		{Contains("tags", "red"), `contains(tags, "red")`},
		{StartsWith("name", "sm"), `starts(name, "sm")`},
		{EndsWith("name", "th"), `ends(name, "th")`},
		{Like("name", "sm%"), `like(name, "sm%")`},
		{Matches("body", "storage engine"), `matches(body, "storage engine")`},
		{Matches("body", "schnelle autos", "de"), `matches(body, "schnelle autos"; de)`},
		{MatchesAny("body", "disk memory", "none"), `matchesany(body, "disk memory"; none)`},
		{Matches("body", "30"), `matches(body, "30")`},
		{And(), `and()`},
		{Not(And()), `or()`},
		{Or(), `or()`},
		{And(Eq("a", 1)), `eq(a, 1)`},
		{Or(Eq("a", 1)), `eq(a, 1)`},
		{And(Eq("a", 1), Gt("b", 2)), `and(eq(a, 1), gt(b, 2))`},
		{Or(Eq("a", 1), And(Gt("b", 2), Lt("b", 5))), `or(eq(a, 1), and(gt(b, 2), lt(b, 5)))`},
		{And(Eq("a", 1), Cond{}), `and(eq(a, 1), and())`},
		{And(Cond{}), `and()`},
		{Not(Cond{}), `or()`},
		{Not(Not(Cond{})), `not or()`},
		{Not(And(Eq("a", 1), Gt("b", 2))), `not and(eq(a, 1), gt(b, 2))`},
		{Not(Not(Eq("a", 1))), `eq(a, 1)`},
		{Not(Ne("a", 1)), `eq(a, 1)`},
	}
	for _, c := range cases {
		if got := show(c.c); got != c.want {
			t.Errorf("got %s, want %s", got, c.want)
		}
	}
}

func TestConditionsThatAreNone(t *testing.T) {
	bad := Eq("a", "\xff")
	var qe *QueryError
	if err := bad.Err(); !errors.As(err, &qe) || !errors.Is(err, ErrInvalidQuery) ||
		qe.Reason != `the value compared with "a": the text is not valid UTF-8` {
		t.Fatalf("a text that is not UTF-8: %v", bad.Err())
	}
	if bad.Op() != OpAnd || bad.Field() != "" || bad.Values() != nil || bad.Conds() != nil || bad.Negated() || bad.Language() != "" {
		t.Errorf("a condition that has an error has nothing else")
	}
	for name, c := range map[string]Cond{
		"in a group":       And(Eq("b", 1), bad),
		"in an or":         Or(bad, Eq("b", 1)),
		"alone in a group": And(bad),
		"negated":          Not(bad),
		"deep":             Or(Eq("x", 1), And(Eq("y", 2), Not(bad))),
	} {
		if c.Err() != bad.Err() {
			t.Errorf("%s: the error did not come through: %v", name, c.Err())
		}
	}
	for name, c := range map[string]struct {
		c    Cond
		want string
	}{
		"between":       {Between("a", 1, make(chan int)), `the value compared with "a": cannot be encoded as JSON`},
		"in":            {In("a", 1, 2, func() {}), `the value compared with "a": cannot be encoded as JSON`},
		"in a list":     {In("a", []float64{1, inf()}), `is not a number a document can hold`},
		"two languages": {Matches("body", "x", "en", "de"), `a full text search has one language`},
		"empty name":    {MatchesAny("body", "x", "  "), `the language of a full text search is named, and this name is empty`},
		"bad search":    {Matches("body", "\xff"), `the text is not valid UTF-8`},
		"bad pattern":   {Like("name", "\xff%"), `the text is not valid UTF-8`},
	} {
		if err := c.c.Err(); !errors.Is(err, ErrInvalidQuery) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an invalid query with %q", name, err, c.want)
		}
	}
}

func inf() float64 {
	var zero float64
	return 1 / zero
}

// Building a condition changes no other, and what a condition was built from can be changed
// afterwards without changing it.
func TestConditionsAreValues(t *testing.T) {
	a, b := Eq("a", 1), Eq("b", 2)
	parts := []Cond{a, b}
	both := And(parts...)
	parts[0] = Eq("z", 26)
	if got := show(both); got != `and(eq(a, 1), eq(b, 2))` {
		t.Errorf("a group followed the slice it was made from: %s", got)
	}
	neither := Not(both)
	if show(both) != `and(eq(a, 1), eq(b, 2))` || show(neither) != `not and(eq(a, 1), eq(b, 2))` {
		t.Errorf("Not changed what it was given: %s, %s", show(both), show(neither))
	}
	roles := []any{"admin", "owner"}
	in := In("role", roles)
	roles[0] = "nobody"
	if got := show(in); got != `in(role, "admin", "owner")` {
		t.Errorf("In followed the list it was made from: %s", got)
	}
	// One condition in two groups is the same in both.
	one, two := And(a, b), Or(a, Not(b))
	if show(one) != `and(eq(a, 1), eq(b, 2))` || show(two) != `or(eq(a, 1), not eq(b, 2))` {
		t.Errorf("a condition used twice: %s, %s", show(one), show(two))
	}
}

// Every way of saying "every document" is the zero condition.
func TestEverythingIsTheZeroCondition(t *testing.T) {
	for name, c := range map[string]Cond{"And()": And(), "And(Cond{})": And(Cond{}), "And(And())": And(And())} {
		if c != (Cond{}) {
			t.Errorf("%s is not the zero condition: %s", name, show(c))
		}
	}
}

func TestQuoteLike(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"plain":     "plain",
		"100%":      `100\%`,
		"a_b":       `a\_b`,
		`back\lash`: `back\\lash`,
		`%_\`:       `\%\_\\`,
		"caffè%":    `caffè\%`,
	} {
		if got := QuoteLike(in); got != want {
			t.Errorf("QuoteLike(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrders(t *testing.T) {
	cases := []struct {
		o         Order
		field     string
		desc, rel bool
	}{
		{Asc("age"), "age", false, false},
		{Desc("age"), "age", true, false},
		{Asc("age").Reverse(), "age", true, false},
		{Desc("a.b").Reverse(), "a.b", false, false},
		{Relevance(), "", true, true},
		{Relevance().Reverse(), "", false, true},
		{Relevance().Reverse().Reverse(), "", true, true},
	}
	for i, c := range cases {
		if c.o.Field() != c.field || c.o.Descending() != c.desc || c.o.ByRelevance() != c.rel {
			t.Errorf("case %d: %q, descending %v, by relevance %v", i, c.o.Field(), c.o.Descending(), c.o.ByRelevance())
		}
	}
	first := Asc("age")
	_ = first.Reverse()
	if first.Descending() {
		t.Errorf("Reverse changed the order it was called on")
	}
}
