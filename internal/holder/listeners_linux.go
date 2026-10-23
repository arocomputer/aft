package holder

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Listeners returns TCP ports with a listening socket bound to loopback or all
// interfaces, read from /proc so no external tool is needed.
func Listeners() ([]int, error) {
	seen := map[int]bool{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 4 || fields[3] != "0A" { // 0A = LISTEN
				continue
			}
			addr, portHex, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			if !loopbackOrAny(addr) {
				continue
			}
			p, err := strconv.ParseInt(portHex, 16, 32)
			if err == nil {
				seen[int(p)] = true
			}
		}
		fh.Close()
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out, nil
}

// loopbackOrAny accepts the hex forms of 0.0.0.0, 127.x, ::, ::1 and
// ::ffff:127.x as they appear in /proc/net/tcp.
func loopbackOrAny(hexAddr string) bool {
	switch len(hexAddr) {
	case 8:
		// Little-endian IPv4: 0100007F is 127.0.0.1.
		return hexAddr == "00000000" || strings.HasSuffix(hexAddr, "7F")
	case 32:
		if hexAddr == strings.Repeat("0", 32) {
			return true
		}
		if hexAddr == strings.Repeat("0", 24)+"01000000" {
			return true
		}
		return strings.HasPrefix(hexAddr, strings.Repeat("0", 16)+"FFFF0000") && strings.HasSuffix(hexAddr, "7F")
	}
	return false
}
