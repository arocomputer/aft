package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// listeners extracts unique IPv4 TCP listeners reachable through remote localhost.
// IPv6-only and specific-interface binds are deliberately not advertised.
func listeners(output string) []int {
	seen := make(map[int]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "LISTEN" {
			continue
		}
		address, value, err := net.SplitHostPort(fields[3])
		if err != nil || (address != "127.0.0.1" && address != "0.0.0.0" && address != "*") {
			continue
		}
		p, err := port(value)
		if err == nil {
			seen[p] = true
		}
	}
	result := make([]int, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Ints(result)
	return result
}

// discover uses read-only Linux ss output over SSH; no remote service is installed.
func discover(ctx context.Context, host string, batch bool) ([]int, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{"-o", "ConnectTimeout=5"}
	if batch {
		args = append(args, "-o", "BatchMode=yes")
	}
	args = append(args, "--", host, "LC_ALL=C ss -H -ltn4")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discover ports on %s (requires Linux ss): %w", host, err)
	}
	return listeners(string(data)), nil
}

// mirror owns one SSH process. Its private connection cannot alter user masters.
type mirror struct {
	cancel context.CancelFunc
	done   chan error
}

// startMirror binds an explicit loopback listener; SSH detects binding conflicts.
func startMirror(parent context.Context, host string, p int) (*mirror, error) {
	ctx, cancel := context.WithCancel(parent)
	args, _ := sshArgs([]string{"ports", host, strconv.Itoa(p)}, os.Stdout)
	args = append([]string{"-S", "none", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}, args...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	m := &mirror{cancel: cancel, done: make(chan error, 1)}
	go func() { m.done <- cmd.Wait() }()
	return m, nil
}

// stop waits for the owned SSH process, releasing its listeners before returning.
func (m *mirror) stop() {
	m.cancel()
	<-m.done
}

// runPorts lists ports or reconciles opt-in mirrors until SIGINT/SIGTERM.
// Watch requires noninteractive SSH authentication and never changes SSH config.
func runPorts(args []string) error {
	flags := flag.NewFlagSet("ports "+args[0], flag.ContinueOnError)
	min, max := 3000, 9999
	if args[0] == "watch" {
		flags.IntVar(&min, "min", min, "lowest port to mirror")
		flags.IntVar(&max, "max", max, "highest port to mirror")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 1 || !validHost(flags.Arg(0)) {
		return fmt.Errorf("expected one SSH host; put flags before HOST")
	}
	if min < 1024 || max > 65535 || min > max {
		return fmt.Errorf("port range must be within 1024–65535, min <= max")
	}
	host := flags.Arg(0)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if args[0] == "list" {
		ports, err := discover(ctx, host, false)
		if err != nil {
			return err
		}
		for _, p := range ports {
			fmt.Println(p)
		}
		return nil
	}
	fmt.Printf("Watching %s ports %d–%d; localhost only; Ctrl+C stops mirrors.\n", host, min, max)
	active := make(map[int]*mirror)
	defer func() {
		for _, m := range active {
			m.stop()
		}
	}()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		ports, err := discover(ctx, host, true)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			// Unknown remote state is not an empty snapshot: retain existing mirrors.
		} else {
			wanted := make(map[int]bool)
			for _, p := range ports {
				if p >= min && p <= max && p != 5900 && p != 7377 {
					wanted[p] = true
				}
			}
			for p, m := range active {
				if !wanted[p] {
					m.stop()
					delete(active, p)
					fmt.Printf("Stopped localhost:%d\n", p)
					continue
				}
				select {
				case err := <-m.done:
					m.cancel()
					delete(active, p)
					fmt.Fprintf(os.Stderr, "localhost:%d tunnel ended (%v); retrying\n", p, err)
				default:
				}
			}
			for p := range wanted {
				if active[p] != nil {
					continue
				}
				// Do not displace a local service. SSH still checks the bind race.
				probe, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", p))
				if err != nil {
					fmt.Fprintf(os.Stderr, "localhost:%d occupied; skipped\n", p)
					continue
				}
				probe.Close()
				m, err := startMirror(ctx, host, p)
				if err != nil {
					return err
				}
				active[p] = m
				fmt.Printf("Starting localhost:%d → %s:%d\n", p, host, p)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
