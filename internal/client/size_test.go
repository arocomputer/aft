package client

import (
	"errors"
	"testing"
)

// TestSaneSize guards the fallback that stops a 0x0 terminal from sending a
// resize the session treats as a control connection (so the session never
// attaches).
func TestSaneSize(t *testing.T) {
	cases := []struct {
		cols, rows int
		err        error
		wc, wr     int
	}{
		{120, 40, nil, 120, 40},
		{0, 0, nil, 80, 24},
		{100, 0, nil, 80, 24},
		{0, 30, nil, 80, 24},
		{-1, -1, nil, 80, 24},
		{120, 40, errors.New("x"), 80, 24},
	}
	for _, c := range cases {
		gc, gr := saneSize(c.cols, c.rows, c.err)
		if gc != c.wc || gr != c.wr {
			t.Errorf("saneSize(%d,%d,%v)=%d,%d want %d,%d", c.cols, c.rows, c.err, gc, gr, c.wc, c.wr)
		}
	}
}
