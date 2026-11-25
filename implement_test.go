package syngo

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type (
	namedBool   bool
	namedString string
	namedInt    int32
	namedUint   uint16
	namedFloat  float32
)

// says is a value that writes itself as JSON, whatever it is made of.
type says int

func (s says) MarshalJSON() ([]byte, error) {
	return []byte(`{"says":` + strings.Repeat("1", int(s)) + `}`), nil
}

// spells is a value that writes itself as text.
type spells int

func (s spells) MarshalText() ([]byte, error) { return []byte(strings.Repeat("a", int(s))), nil }

type tagged struct {
	Name string `json:"name"`
	Age  int    `json:"age,omitempty"`
	skip int
}

func TestPlainValue(t *testing.T) {
	when := time.Date(2026, 11, 24, 10, 30, 0, 123456789, time.FixedZone("x", 3600))
	var nilPointer *tagged
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, `null`},
		{"true", true, `true`},
		{"false", false, `false`},
		{"string", "active", `"active"`},
		{"empty string", "", `""`},
		{"quotes and controls", "a\"b\\c\nd\te\rf\x01g", `"a\"b\\c\nd\te\rf\u0001g"`},
		{"letters past ASCII", "caffè 日本", `"caffè 日本"`},
		{"int", 30, `30`},
		{"negative", -7, `-7`},
		{"int8", int8(-128), `-128`},
		{"int16", int16(300), `300`},
		{"int32", int32(-70000), `-70000`},
		{"smallest int64", int64(math.MinInt64), `-9223372036854775808`},
		{"uint", uint(7), `7`},
		{"uint8", uint8(255), `255`},
		{"uint16", uint16(65535), `65535`},
		{"uint32", uint32(4000000000), `4000000000`},
		{"largest uint64", uint64(math.MaxUint64), `18446744073709551615`},
		{"float", 1.5, `1.5`},
		{"whole float", 5.0, `5.0`},
		{"negative zero", math.Copysign(0, -1), `-0.0`},
		{"large float", 1e300, `1e+300`},
		{"small float", 5e-324, `5e-324`},
		{"float32", float32(0.5), `0.5`},
		{"time", when, `"2026-11-24T10:30:00.123456789+01:00"`},
		{"raw", json.RawMessage(`{"a": [1, 2]}`), `{"a": [1, 2]}`},
		{"raw that is no JSON", json.RawMessage(`{nope`), `{nope`},
		{"number", json.Number("12.50"), `12.50`},
		{"marshaler", says(3), `{"says":111}`},
		{"text marshaler", spells(2), `"aa"`},
		{"named bool", namedBool(true), `true`},
		{"named string", namedString("x"), `"x"`},
		{"named int", namedInt(-5), `-5`},
		{"named uint", namedUint(9), `9`},
		{"named float", namedFloat(2), `2.0`},
		{"struct", tagged{Name: "Ann", skip: 1}, `{"name":"Ann"}`},
		{"pointer", &tagged{Name: "Bo", Age: 4}, `{"name":"Bo","age":4}`},
		{"nil pointer", nilPointer, `null`},
		{"slice", []int{1, 2, 3}, `[1,2,3]`},
		{"empty slice", []string{}, `[]`},
		{"map", map[string]int{"a": 1}, `{"a":1}`},
		{"bytes", []byte("hi"), `"aGk="`},
	}
	for _, c := range cases {
		got, err := PlainValue(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestPlainValueRefuses(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"not a number", math.NaN(), "is not a number a document can hold"},
		{"infinite", math.Inf(1), "is not a number a document can hold"},
		{"infinite float32", float32(math.Inf(-1)), "is not a number a document can hold"},
		{"named infinite", namedFloat(float32(math.Inf(1))), "is not a number a document can hold"},
		{"bad text", "a\xffb", "the text is not valid UTF-8"},
		{"bad named text", namedString("\xc3"), "the text is not valid UTF-8"},
		{"a channel", make(chan int), "cannot be encoded as JSON"},
		{"a function", func() {}, "cannot be encoded as JSON"},
	}
	for _, c := range cases {
		got, err := PlainValue(c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %s, %v; want an error with %q", c.name, got, err, c.want)
		}
	}
}

// What PlainValue returns belongs to the caller: it shares nothing with what it was given.
func TestPlainValueOwnsItsBytes(t *testing.T) {
	raw := json.RawMessage(`[1,2]`)
	got, err := PlainValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[1] = '9'
	if string(got) != `[1,2]` {
		t.Fatalf("the plain form followed the bytes it was made from: %s", got)
	}
}

// A text of any content comes back, read as JSON, as the text it was.
func TestQuotedTextReadsBack(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	alphabet := []rune("ab\"\\/\b\f\n\r\t\x00\x1f\x7f <>&é日  \U0001F600")
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := r.IntN(12); n > 0; n-- {
			b.WriteRune(alphabet[r.IntN(len(alphabet))])
		}
		want := b.String()
		text, err := PlainValue(want)
		if err != nil {
			t.Fatalf("%q: %v", want, err)
		}
		var got string
		if err := json.Unmarshal(text, &got); err != nil {
			t.Fatalf("%q was written as %s, which does not read: %v", want, text, err)
		}
		if got != want {
			t.Fatalf("%q was written as %s, which reads as %q", want, text, got)
		}
		if !utf8.Valid(text) {
			t.Fatalf("%q was written as bytes that are not UTF-8", want)
		}
	}
}

// A number of any type comes back, read as JSON, as the number it was.
func TestNumbersReadBack(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	for i := 0; i < 20000; i++ {
		f := math.Float64frombits(r.Uint64())
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		text, err := PlainValue(f)
		if err != nil {
			t.Fatal(err)
		}
		var got float64
		if err := json.Unmarshal(text, &got); err != nil || got != f {
			t.Fatalf("%v was written as %s, which reads as %v (%v)", f, text, got, err)
		}
		if !bytes.ContainsAny(text, ".eE") {
			t.Fatalf("the float %v was written as %s, which reads as a whole number", f, text)
		}
		n := int64(r.Uint64())
		text, _ = PlainValue(n)
		var back int64
		if err := json.Unmarshal(text, &back); err != nil || back != n {
			t.Fatalf("%d was written as %s", n, text)
		}
	}
}

func TestPlainList(t *testing.T) {
	if got, ok := PlainList([]string{"a", "b"}); !ok || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("a slice of strings: %v, %v", got, ok)
	}
	if got, ok := PlainList([2]int{4, 5}); !ok || len(got) != 2 || got[1] != 5 {
		t.Errorf("an array: %v, %v", got, ok)
	}
	if got, ok := PlainList([]any{1, "x"}); !ok || len(got) != 2 {
		t.Errorf("a slice of anything: %v, %v", got, ok)
	}
	if got, ok := PlainList([]int{}); !ok || len(got) != 0 {
		t.Errorf("an empty slice is a list of nothing: %v, %v", got, ok)
	}
	for _, v := range []any{nil, []byte("[1]"), json.RawMessage("[1]"), "ab", 7, map[string]int{}, tagged{}} {
		if got, ok := PlainList(v); ok {
			t.Errorf("%T is no list, and was read as %v", v, got)
		}
	}
}

func TestPlainDocument(t *testing.T) {
	doc := NewDocument("d1", []byte(`{"_id":"d1"}`), DocumentInfo{})
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"bytes", []byte(`{"a": 1}`), `{"a": 1}`},
		{"raw", json.RawMessage(`{"a":2}`), `{"a":2}`},
		{"string", `{"a":3}`, `{"a":3}`},
		{"text that is no JSON", `nope`, `nope`},
		{"document", doc, `{"_id":"d1"}`},
		{"struct", tagged{Name: "Ann"}, `{"name":"Ann"}`},
		{"map", map[string]any{"a": true}, `{"a":true}`},
	}
	for _, c := range cases {
		got, err := PlainDocument(c.in)
		if err != nil || string(got) != c.want {
			t.Errorf("%s: got %s, %v; want %s", c.name, got, err, c.want)
		}
	}
	var none *Document
	for _, in := range []any{nil, none} {
		if _, err := PlainDocument(in); !errors.Is(err, ErrInvalidDocument) {
			t.Errorf("%T: got %v, want an invalid document", in, err)
		}
	}
	var de *DocumentError
	if _, err := PlainDocument(make(chan int)); !errors.As(err, &de) || de.Offset != -1 || !strings.Contains(de.Reason, "cannot be encoded as JSON") {
		t.Errorf("a value that cannot be encoded: %v", err)
	}
}

func TestPlainWrite(t *testing.T) {
	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	w, err := PlainWrite(nil)
	if err != nil || w.Timed || w.HasMeta || w.Meta != nil || w.ExpireIn != 0 || !w.ExpireAt.IsZero() {
		t.Errorf("no option: %+v, %v", w, err)
	}
	w, _ = PlainWrite([]WriteOption{{}, ExpireIn(time.Hour), {}})
	if !w.Timed || w.ExpireIn != time.Hour || !w.ExpireAt.IsZero() || w.HasMeta {
		t.Errorf("ExpireIn: %+v", w)
	}
	w, _ = PlainWrite([]WriteOption{ExpireIn(time.Hour), ExpireAt(at)})
	if !w.Timed || w.ExpireIn != 0 || !w.ExpireAt.Equal(at) {
		t.Errorf("ExpireAt after ExpireIn: %+v", w)
	}
	w, _ = PlainWrite([]WriteOption{ExpireAt(at), ExpireIn(time.Minute)})
	if !w.Timed || w.ExpireIn != time.Minute || !w.ExpireAt.IsZero() {
		t.Errorf("ExpireIn after ExpireAt: %+v", w)
	}
	w, _ = PlainWrite([]WriteOption{ExpireIn(0)})
	if !w.Timed || w.ExpireIn != 0 || !w.ExpireAt.IsZero() {
		t.Errorf("never: %+v", w)
	}
	for name, c := range map[string]struct {
		in   any
		want string
	}{
		"string": {`{"by":"import"}`, `{"by":"import"}`},
		"bytes":  {[]byte(`{"by": 1}`), `{"by": 1}`},
		"raw":    {json.RawMessage(`[1]`), `[1]`},
		"map":    {map[string]string{"by": "import"}, `{"by":"import"}`},
		"number": {7, `7`},
	} {
		w, err := PlainWrite([]WriteOption{Meta("first"), Meta(c.in)})
		if err != nil || !w.HasMeta || string(w.Meta) != c.want || w.Timed {
			t.Errorf("Meta of a %s: %+v, %v", name, w, err)
		}
	}
	w, err = PlainWrite([]WriteOption{Meta(`{"a":1}`), Meta(nil)})
	if err != nil || !w.HasMeta || w.Meta != nil {
		t.Errorf("Meta(nil) removes: %+v, %v", w, err)
	}
	if _, err := PlainWrite([]WriteOption{Meta(make(chan int))}); !errors.Is(err, ErrInvalidDocument) || !strings.Contains(err.Error(), "the metadata cannot be encoded as JSON") {
		t.Errorf("metadata that cannot be encoded: %v", err)
	}
	// One that cannot be encoded and is replaced is never encoded.
	if w, err := PlainWrite([]WriteOption{Meta(make(chan int)), Meta(1)}); err != nil || string(w.Meta) != "1" {
		t.Errorf("the last Meta counts: %+v, %v", w, err)
	}
}
