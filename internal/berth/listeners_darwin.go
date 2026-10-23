package berth

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Listeners returns TCP ports with a listening socket on loopback or all
// interfaces, using the lsof that ships with macOS.
func Listeners() ([]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fn").Output()
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		addr := line[1:]
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			continue
		}
		host := addr[:i]
		if host != "*" && host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "[::]" {
			continue
		}
		p, err := strconv.Atoi(addr[i+1:])
		if err == nil {
			seen[p] = true
		}
	}
	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	return ports, nil
}
