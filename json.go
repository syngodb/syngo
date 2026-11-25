package syngo

import (
	"fmt"

	goccy "github.com/goccy/go-json"
)

// The one place a Go value becomes JSON text and JSON text a Go value. The embedded library and
// the driver both encode and decode through these two functions, so that a value is written and
// read the same way wherever the database is. The encoder can panic on a value it does not know
// what to do with; that is an error of the call here, never the end of the program.

// EncodeJSON is a Go value as JSON text, by its json tags.
func EncodeJSON(v any) (text []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = nil, fmt.Errorf("JSON encoding panic: %v", r)
		}
	}()
	return goccy.Marshal(v)
}

// DecodeJSON reads JSON text into the Go value v points to, by its json tags.
func DecodeJSON(text []byte, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("JSON decoding panic: %v", r)
		}
	}()
	return goccy.Unmarshal(text, v)
}
