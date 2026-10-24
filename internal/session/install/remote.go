// Package install puts the session server on a host. The aft binary
// embeds a server build for each supported host platform; Ensure uploads the
// right one when the host has none or has an older one. The installed binary
// is hashed live and reports its build time, so no version file is kept on
// the host.
package install

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/arocomputer/aft/internal/bridge/transport"
	"github.com/arocomputer/aft/internal/session"
)

// Server binaries are placed here by ./x servers as gzipped aft-server-<os>-<arch>
// and embedded into aft for installation on hosts.
//
//go:embed embed
var bins embed.FS

// Blob returns the embedded server for a platform, decompressed.
func Blob(goos, goarch string) ([]byte, error) {
	gz, err := bins.ReadFile("embed/aft-server-" + goos + "-" + goarch + ".gz")
	if err != nil || len(gz) == 0 {
		return nil, fmt.Errorf("this aft build has no server for %s/%s; run ./x build", goos, goarch)
	}
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, fmt.Errorf("decompress server for %s/%s: %w", goos, goarch, err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompress server for %s/%s: %w", goos, goarch, err)
	}
	return b, nil
}

// Platform maps uname output to Go's GOOS and GOARCH.
func Platform(uname string) (goos, goarch string, err error) {
	f := strings.Fields(uname)
	if len(f) < 2 {
		return "", "", fmt.Errorf("unexpected uname output %q", uname)
	}
	switch f[0] {
	case "Linux":
		goos = "linux"
	case "Darwin":
		goos = "darwin"
	default:
		return "", "", fmt.Errorf("unsupported server OS %q", f[0])
	}
	switch f[1] {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported server architecture %q", f[1])
	}
	return goos, goarch, nil
}

// Compare says how the server installed on a host relates to the one this aft
// carries: -1 when it is older, 0 when it is this build, 1 when it is newer.
// Two builds are ordered by their commit time; the same time with a different
// binary is a rebuild of the same commit, which counts as older. It contacts
// no host of its own.
func Compare(info session.Info) (int, error) {
	blob, err := Blob(info.OS, info.Arch)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256(blob)
	return compare(info, hex.EncodeToString(sum[:]), session.BuiltAt()), nil
}

func compare(info session.Info, want string, built int64) int {
	switch {
	case info.SHA == want:
		return 0
	case info.Built > built:
		return 1
	}
	return -1
}

// Info runs the server's info command on the host.
func Info(h transport.Host) (session.Info, error) {
	var info session.Info
	if h.IsLocal() {
		return session.GatherInfo(), nil
	}
	out, err := transport.Output(h, nil, "info")
	if err != nil {
		return info, err
	}
	return info, json.Unmarshal(out, &info)
}

// Ensure makes sure the host has a server this aft can work with, and returns
// its info. One round trip asks the installed server about itself, or, when
// there is none, the host for its platform; the server is uploaded only when
// the host has none, or an older one. A newer one is left alone, so a machine
// that was updated later never pulls a host back. Local hosts need nothing.
func Ensure(h transport.Host) (session.Info, error) {
	if h.IsLocal() {
		return session.GatherInfo(), nil
	}
	server := transport.RemoteServer()
	out, err := transport.RunRaw(h, nil, "[ -x "+server+" ] && "+transport.RemotePrefix()+server+" info || uname -sm")
	if err != nil {
		return session.Info{}, fmt.Errorf("reach %s: %w", h.Dest(), err)
	}
	var info session.Info
	have := json.Unmarshal(bytes.TrimSpace(out), &info) == nil && info.OS != ""
	if !have {
		if info.OS, info.Arch, err = Platform(string(out)); err != nil {
			return session.Info{}, err
		}
	}
	blob, err := Blob(info.OS, info.Arch)
	if err != nil {
		return session.Info{}, err
	}
	sum := sha256.Sum256(blob)
	want := hex.EncodeToString(sum[:])
	if have && compare(info, want, session.BuiltAt()) >= 0 {
		if info.Shims {
			return info, nil
		}
		// Current or newer, only its shims are missing: set it up, never
		// replace it with this build.
		if _, err := transport.RunRaw(h, nil, transport.RemotePrefix()+server+" setup"); err != nil {
			return session.Info{}, fmt.Errorf("set up server on %s: %w", h.Dest(), err)
		}
		return Info(h)
	}
	// Write the binary to a temp name and move it into place, so a server in
	// use is never truncated; then let it set up its own shims.
	dir := transport.RemoteHome()
	dst := transport.RemoteServer()
	script := "mkdir -p -m 700 " + dir + " && cat > " + dst + ".new && chmod 700 " + dst + ".new && mv -f " + dst + ".new " + dst + " && " + transport.RemotePrefix() + dst + " setup"
	if _, err := transport.RunRaw(h, bytes.NewReader(blob), script); err != nil {
		return session.Info{}, fmt.Errorf("install server on %s: %w", h.Dest(), err)
	}
	return Info(h)
}
