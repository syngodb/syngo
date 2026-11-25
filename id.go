package syngo

import (
	crand "crypto/rand"
	"math/rand/v2"
	"sync"
	"time"
)

// Identifiers. A document stored without one is given one by NewID: 26 characters, a ULID. The
// first ten spell the millisecond it was made in and the other sixteen are random, so identifiers
// sort by creation time, and two made in the same millisecond by one process still come out in
// the order they were made.

const idAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

type idSource struct {
	mu     sync.Mutex
	random *rand.ChaCha8
	last   uint64   // the millisecond of the last identifier
	tail   [10]byte // its random part
}

var ids = newIDSource()

func newIDSource() *idSource {
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		panic("syngo: no randomness: " + err.Error())
	}
	return &idSource{random: rand.NewChaCha8(seed)}
}

func (s *idSource) next(now time.Time) string {
	ms := uint64(now.UnixMilli())
	s.mu.Lock()
	if ms <= s.last {
		// Same millisecond as the last one (or the clock stepped back): the next identifier is the
		// last one plus one.
		ms = s.last
		for i := len(s.tail) - 1; i >= 0; i-- {
			s.tail[i]++
			if s.tail[i] != 0 {
				break
			}
		}
	} else {
		s.last = ms
		a, b := s.random.Uint64(), s.random.Uint64()
		for i := 0; i < 8; i++ {
			s.tail[i] = byte(a >> (8 * i))
		}
		s.tail[8], s.tail[9] = byte(b), byte(b>>8)
	}
	tail := s.tail
	s.mu.Unlock()

	var out [26]byte
	// Ten characters for the 48 bits of time, five bits each, the first one holding three.
	for i := 9; i >= 0; i-- {
		out[i] = idAlphabet[ms&31]
		ms >>= 5
	}
	// Sixteen characters for the 80 random bits.
	hi := uint64(tail[0])<<32 | uint64(tail[1])<<24 | uint64(tail[2])<<16 | uint64(tail[3])<<8 | uint64(tail[4])
	lo := uint64(tail[5])<<32 | uint64(tail[6])<<24 | uint64(tail[7])<<16 | uint64(tail[8])<<8 | uint64(tail[9])
	for i := 7; i >= 0; i-- {
		out[10+i] = idAlphabet[hi&31]
		hi >>= 5
		out[18+i] = idAlphabet[lo&31]
		lo >>= 5
	}
	return string(out[:])
}

// MaxIDLen is the longest identifier a document may have, in bytes.
const MaxIDLen = 512

// NewID makes a new document identifier: a ULID, 26 characters that sort by the time they were
// made. It is what a document stored without an identifier is given.
func NewID() string {
	return ids.next(time.Now())
}

// NewIDAt makes the identifier NewID would make at the time given: for a database whose clock is
// not the machine's, and for documents brought in from elsewhere whose identifiers are to sort by
// the time they were first made. Identifiers come from one source: those made for one
// millisecond, or for a time before that of the last one made, still come out in the order they
// were made.
func NewIDAt(t time.Time) string {
	return ids.next(t)
}

// IDTime reads the creation time out of an identifier made by NewID, to the millisecond. ok is
// false for an identifier of any other form.
func IDTime(id string) (t time.Time, ok bool) {
	if len(id) != 26 {
		return time.Time{}, false
	}
	var ms uint64
	for i := 0; i < 26; i++ {
		c := id[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'A' && c <= 'Z':
			v = -1
			for j := 10; j < len(idAlphabet); j++ {
				if idAlphabet[j] == c {
					v = j
					break
				}
			}
		default:
			v = -1
		}
		if v < 0 {
			return time.Time{}, false
		}
		if i < 10 {
			ms = ms<<5 | uint64(v)
		}
	}
	if id[0] > '7' {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(ms)).UTC(), true
}

// IDFloor is the smallest identifier NewID could have made at time t: every identifier made
// before t sorts below it, and every one made at or after t sorts at or above it. A collection
// whose identifiers come from NewID can therefore be read by creation time as a range of
// identifiers.
func IDFloor(t time.Time) string {
	ms := uint64(0)
	if t.UnixMilli() > 0 {
		ms = uint64(t.UnixMilli())
	}
	out := []byte("00000000000000000000000000")
	for i := 9; i >= 0; i-- {
		out[i] = idAlphabet[ms&31]
		ms >>= 5
	}
	return string(out)
}
