package vt

import "unicode/utf8"

// parser is the escape-sequence state machine (the VT500 model, with UTF-8
// decoding in the ground state). It collects a sequence and hands it to the
// Terminal once complete.
type parser struct {
	state   pstate
	utf8buf [4]byte
	utf8n   int
	utf8len int

	params  [][]int // one entry per parameter; sub-parameters after ':' nest
	hasVal  bool    // the current parameter has received at least one digit
	private byte    // '?', '>', '<', '=' or 0
	inter   []byte  // intermediate bytes such as '$', ' ', '"'
	str     []byte  // OSC, DCS or APC payload
	dcsFin  byte    // final byte of a DCS introducer
}

type pstate uint8

const (
	stGround pstate = iota
	stEscape
	stEscapeInter
	stCSIEntry
	stCSIParam
	stCSIInter
	stCSIIgnore
	stOSC
	stDCSEntry
	stDCSParam
	stDCSInter
	stDCSPass
	stDCSIgnore
	stString // SOS, PM, APC: swallowed until ST
)

const maxParams = 32
const maxString = 1 << 16

func (p *parser) clear() {
	p.params = p.params[:0]
	p.hasVal = false
	p.private = 0
	p.inter = p.inter[:0]
	p.str = p.str[:0]
	p.dcsFin = 0
}

func (p *parser) feed(t *Terminal, c byte) {
	// A pending UTF-8 sequence only continues in ground state.
	if p.utf8n > 0 {
		if p.state == stGround && c >= 0x80 && c < 0xC0 {
			p.utf8buf[p.utf8n] = c
			p.utf8n++
			if p.utf8n == p.utf8len {
				r, _ := utf8.DecodeRune(p.utf8buf[:p.utf8n])
				p.utf8n = 0
				t.print(r)
			}
			return
		}
		// Invalid continuation: drop the partial character and reprocess.
		p.utf8n = 0
		t.print(utf8.RuneError)
	}

	// C0 controls act in every state except inside strings, where only a few do.
	if c < 0x20 || c == 0x7F {
		switch p.state {
		case stOSC, stDCSPass, stString, stDCSEntry, stDCSParam, stDCSInter, stDCSIgnore:
			switch c {
			case 0x1B:
				// ESC ends the string, whether or not a '\\' (ST) follows.
				switch p.state {
				case stOSC:
					t.oscDispatch(p.str)
				case stDCSPass:
					t.dcsDispatch(p.private, p.params, p.inter, p.dcsFin, p.str)
				}
				p.clear()
				p.state = stEscape
				return
			case 0x07:
				if p.state == stOSC {
					t.oscDispatch(p.str)
					p.clear()
					p.state = stGround
				}
				return
			case 0x18, 0x1A:
				p.clear()
				p.state = stGround
				return
			}
			if p.state == stDCSPass || p.state == stOSC {
				p.str = appendBounded(p.str, c)
			}
			return
		}
		switch c {
		case 0x1B:
			p.clear()
			p.state = stEscape
		case 0x18, 0x1A:
			p.clear()
			p.state = stGround
		default:
			t.control(c)
		}
		return
	}

	switch p.state {
	case stGround:
		if c < 0x80 {
			t.print(rune(c))
			return
		}
		switch {
		case c >= 0xC2 && c < 0xE0:
			p.utf8len = 2
		case c >= 0xE0 && c < 0xF0:
			p.utf8len = 3
		case c >= 0xF0 && c < 0xF5:
			p.utf8len = 4
		default:
			t.print(utf8.RuneError)
			return
		}
		p.utf8buf[0] = c
		p.utf8n = 1

	case stEscape:
		switch {
		case c == '[':
			p.state = stCSIEntry
		case c == ']':
			p.state = stOSC
		case c == 'P':
			p.state = stDCSEntry
		case c == 'X' || c == '^' || c == '_':
			p.state = stString
		case c >= 0x20 && c < 0x30:
			p.inter = append(p.inter, c)
			p.state = stEscapeInter
		case c == '\\':
			// String terminator with nothing pending.
			p.state = stGround
		default:
			t.escDispatch(p.inter, c)
			p.clear()
			p.state = stGround
		}

	case stEscapeInter:
		if c >= 0x20 && c < 0x30 {
			p.inter = append(p.inter, c)
			return
		}
		t.escDispatch(p.inter, c)
		p.clear()
		p.state = stGround

	case stCSIEntry, stCSIParam, stCSIInter:
		switch {
		case c >= '0' && c <= '9':
			if p.state == stCSIInter {
				p.state = stCSIIgnore
				return
			}
			p.param(int(c - '0'))
			p.state = stCSIParam
		case c == ';':
			if p.state == stCSIInter {
				p.state = stCSIIgnore
				return
			}
			p.nextParam()
			p.state = stCSIParam
		case c == ':':
			if p.state == stCSIInter {
				p.state = stCSIIgnore
				return
			}
			p.subParam()
			p.state = stCSIParam
		case c >= 0x3C && c <= 0x3F:
			if p.state != stCSIEntry {
				p.state = stCSIIgnore
				return
			}
			p.private = c
			p.state = stCSIParam
		case c >= 0x20 && c < 0x30:
			p.inter = append(p.inter, c)
			p.state = stCSIInter
		case c >= 0x40 && c < 0x7F:
			p.finishParams()
			t.csiDispatch(p.private, p.params, p.inter, c)
			p.clear()
			p.state = stGround
		default:
			p.state = stCSIIgnore
		}

	case stCSIIgnore:
		if c >= 0x40 && c < 0x7F {
			p.clear()
			p.state = stGround
		}

	case stOSC:
		p.str = appendBounded(p.str, c)

	case stDCSEntry, stDCSParam, stDCSInter:
		switch {
		case c >= '0' && c <= '9':
			p.param(int(c - '0'))
			p.state = stDCSParam
		case c == ';':
			p.nextParam()
			p.state = stDCSParam
		case c == ':':
			p.subParam()
			p.state = stDCSParam
		case c >= 0x3C && c <= 0x3F && p.state == stDCSEntry:
			p.private = c
			p.state = stDCSParam
		case c >= 0x20 && c < 0x30:
			p.inter = append(p.inter, c)
			p.state = stDCSInter
		case c >= 0x40 && c < 0x7F:
			p.dcsFin = c
			p.finishParams()
			p.state = stDCSPass
		default:
			p.state = stDCSIgnore
		}

	case stDCSPass:
		p.str = appendBounded(p.str, c)

	case stDCSIgnore, stString:
		// swallowed until ESC \ (handled in the control branch above)
	}
}

func appendBounded(b []byte, c byte) []byte {
	if len(b) >= maxString {
		return b
	}
	return append(b, c)
}

func (p *parser) param(d int) {
	if len(p.params) == 0 {
		p.params = append(p.params, []int{0})
	}
	cur := p.params[len(p.params)-1]
	v := cur[len(cur)-1]
	if v < 1<<24 {
		cur[len(cur)-1] = v*10 + d
	}
	p.hasVal = true
}

func (p *parser) nextParam() {
	if len(p.params) == 0 {
		p.params = append(p.params, []int{0})
	}
	if len(p.params) < maxParams {
		p.params = append(p.params, []int{0})
	}
	p.hasVal = false
}

func (p *parser) subParam() {
	if len(p.params) == 0 {
		p.params = append(p.params, []int{0})
	}
	i := len(p.params) - 1
	if len(p.params[i]) < 8 {
		p.params[i] = append(p.params[i], 0)
	}
}

func (p *parser) finishParams() {}
