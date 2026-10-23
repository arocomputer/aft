// Package ports mirrors listening ports on the server to the same port on the
// user's loopback interface, over the shared SSH connection, so a dev server
// started remotely opens at localhost in the local browser.
package ports

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fschrhunt/aft/internal/transport"
)

// Range is an inclusive port range.
type Range struct{ Min, Max int }

// ParseRange reads "3000-9999" or a single port.
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{3000, 9999}, nil
	}
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		hi = lo
	}
	a, err1 := strconv.Atoi(strings.TrimSpace(lo))
	b, err2 := strconv.Atoi(strings.TrimSpace(hi))
	if err1 != nil || err2 != nil || a < 1 || b > 65535 || a > b {
		return Range{}, fmt.Errorf("port range must look like 3000-9999: %q", s)
	}
	return Range{a, b}, nil
}

// Mirror keeps local forwards in step with the server's listeners.
type Mirror struct {
	mu     sync.Mutex
	host   transport.Host
	rng    Range
	active map[int]bool
	Log    func(string)
}

// New returns a mirror for one host.
func New(h transport.Host, r Range) *Mirror {
	return &Mirror{host: h, rng: r, active: map[int]bool{}, Log: func(string) {}}
}

// Update reconciles the mirrored set with the server's current listeners.
func (m *Mirror) Update(listening []int) {
	if m.host.IsLocal() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int]bool{}
	for _, p := range listening {
		if p >= m.rng.Min && p <= m.rng.Max {
			want[p] = true
		}
	}
	for p := range m.active {
		if !want[p] {
			transport.Forward(m.host, p, false)
			delete(m.active, p)
			m.Log(fmt.Sprintf("stopped mirroring localhost:%d", p))
		}
	}
	for p := range want {
		if m.active[p] {
			continue
		}
		if !free(p) {
			continue // never displace something already listening here
		}
		if err := transport.Forward(m.host, p, true); err != nil {
			m.Log(err.Error())
			continue
		}
		m.active[p] = true
		m.Log(fmt.Sprintf("mirroring localhost:%d", p))
	}
}

// Active returns the mirrored ports.
func (m *Mirror) Active() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]int, 0, len(m.active))
	for p := range m.active {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// Close cancels every forward this mirror added.
func (m *Mirror) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for p := range m.active {
		transport.Forward(m.host, p, false)
		delete(m.active, p)
	}
}

func free(port int) bool {
	ln, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
