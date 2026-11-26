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

// depthOf is how deep the conditions of one are combined, counted from what an implementation
// reads: a condition without conditions under it is 0 deep.
func depthOf(c Cond) int {
	deepest := -1
	for _, kid := range c.Conds() {
		if d := depthOf(kid); d > deepest {
			deepest = d
		}
	}
	return deepest + 1
}

func TestConditionsCombineOnlySoDeep(t *testing.T) {
	// chain combines a comparison in conditions, each around the one before, so many times.
	chain := func(levels int, around func(i int, inner Cond) Cond) Cond {
		c := Eq("n", 0)
		for i := 1; i <= levels; i++ {
			c = around(i, c)
		}
		return c
	}
	ways := map[string]func(i int, inner Cond) Cond{
		"and": func(i int, inner Cond) Cond { return And(inner, Eq("n", i)) },
		"or":  func(i int, inner Cond) Cond { return Or(Eq("n", i), inner) },
		"one, then the other": func(i int, inner Cond) Cond {
			if i%2 == 0 {
				return And(inner, Gt("m", i))
			}
			return Or(inner, Lt("m", i))
		},
		"negated at every level": func(i int, inner Cond) Cond { return Not(And(Not(inner), Exists("x"))) },
		"beside everything":      func(i int, inner Cond) Cond { return And(Cond{}, inner) },
	}
	for name, around := range ways {
		deepest := chain(MaxCondDepth, around)
		if err := deepest.Err(); err != nil {
			t.Errorf("%s, %d deep: %v", name, MaxCondDepth, err)
			continue
		}
		if got := depthOf(deepest); got != MaxCondDepth {
			t.Errorf("%s: the condition reads %d deep, and was made %d deep", name, got, MaxCondDepth)
		}
		// What can be made can be described and made again: it is what travels to a server.
		if again := rebuilt(deepest); again.Err() != nil || show(again) != show(deepest) {
			t.Errorf("%s, %d deep, made again from its description: %v", name, MaxCondDepth, again.Err())
		}
		// Not adds nothing, and a group of one is that one.
		if c := Not(And(Not(Or(deepest)))); c.Err() != nil || depthOf(c) != MaxCondDepth {
			t.Errorf("%s: negated and alone in a group it is %d deep, %v", name, depthOf(c), c.Err())
		}

		deeper := around(MaxCondDepth+1, deepest)
		var qe *QueryError
		if err := deeper.Err(); !errors.As(err, &qe) || !errors.Is(err, ErrInvalidQuery) ||
			qe.Reason != fmt.Sprintf("the conditions are combined in one another more than %d deep", MaxCondDepth) {
			t.Errorf("%s, one deeper: %v", name, deeper.Err())
		}
		if deeper.Conds() != nil || deeper.Op() != OpAnd {
			t.Errorf("%s: a condition that is too deep has nothing but its error", name)
		}
		// And the error comes through whatever is built on it.
		if c := Or(Eq("a", 1), Not(deeper)); c.Err() != deeper.Err() {
			t.Errorf("%s: the error did not come through: %v", name, c.Err())
		}
		// Described deeper than it can be, it is no condition either: PlainCond counts too.
		if c := PlainCond(OpAnd, false, "", nil, "", []Cond{rebuilt(deepest), Eq("z", 1)}); !errors.Is(c.Err(), ErrInvalidQuery) {
			t.Errorf("%s: a description one deeper was taken: %v", name, c.Err())
		}
	}

	// It is the depth that is bounded, not the number of conditions: a loop that collects them
	// and combines once is 1 deep, and two such under one another 2.
	many := make([]Cond, 5000)
	for i := range many {
		many[i] = Eq("n", i)
	}
	if c := Or(many...); c.Err() != nil || depthOf(c) != 1 {
		t.Errorf("5,000 conditions combined once: %d deep, %v", depthOf(c), c.Err())
	}
	if c := And(Or(many...), Or(many[:10]...), Eq("a", 1)); c.Err() != nil || depthOf(c) != 2 {
		t.Errorf("two combinations under one: %d deep, %v", depthOf(c), c.Err())
	}
	// What is combined counts, whatever it is: everything, nothing, and their like are conditions
	// one level down as a comparison is, though they have nothing under them.
	and := func(i int, inner Cond) Cond { return And(inner, Eq("n", i)) }
	for name, innermost := range map[string]Cond{
		"everything twice":       Or(Cond{}, Cond{}),
		"nothing and everything": And(Or(), And()),
		"not everything, twice":  Or(Not(Cond{}), Not(Cond{})),
	} {
		if innermost.Err() != nil || depthOf(innermost) != 1 {
			t.Fatalf("%s: %d deep, %v", name, depthOf(innermost), innermost.Err())
		}
		c := innermost
		for i := 1; i < MaxCondDepth; i++ {
			c = and(i, c)
		}
		if c.Err() != nil || depthOf(c) != MaxCondDepth {
			t.Errorf("%s under %d combinations: %d deep, %v", name, MaxCondDepth-1, depthOf(c), c.Err())
		}
		if deeper := and(0, c); !errors.Is(deeper.Err(), ErrInvalidQuery) {
			t.Errorf("%s under %d combinations was taken: %v", name, MaxCondDepth, deeper.Err())
		}
	}
	// A combination of nothing, by itself, is as deep as a comparison.
	for name, c := range map[string]Cond{"everything": And(), "nothing": Or(), "not everything": Not(Cond{})} {
		if c.Err() != nil || depthOf(c) != 0 {
			t.Errorf("%s: %d deep, %v", name, depthOf(c), c.Err())
		}
	}
}
