// Package remote installs and verifies the server on a host. The aft binary
// embeds a server build for each supported host platform; Ensure uploads the
// right one when the host has none or has a different one. A match is decided
// by hashing the installed binary live, so no version file is kept on the host.
package remote

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

	"github.com/arocomputer/aft/internal/server"
	"github.com/arocomputer/aft/internal/transport"
)

// Server binaries are placed here by ./x servers as gzipped server-<os>-<arch>
// and embedded into aft for installation on hosts.
//
//go:embed embed
var bins embed.FS

// Blob returns the embedded server for a platform, decompressed.
func Blob(goos, goarch string) ([]byte, error) {
	gz, err := bins.ReadFile("embed/server-" + goos + "-" + goarch + ".gz")
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

// UpToDate reports whether the server already installed on a host is the same
// build this aft carries, by comparing the host-reported hash of its running
// binary to the hash of the embedded blob. It contacts no host of its own.
func UpToDate(info server.Info) (bool, error) {
	blob, err := Blob(info.OS, info.Arch)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(blob)
	return info.SHA == hex.EncodeToString(sum[:]), nil
}

// Info runs the server's info command on the host.
func Info(h transport.Host) (server.Info, error) {
	var info server.Info
	if h.IsLocal() {
		return server.GatherInfo(), nil
	}
	out, err := transport.Output(h, nil, "info")
	if err != nil {
		return info, err
	}
	return info, json.Unmarshal(out, &info)
}

// Ensure installs or updates the server on the host and prepares it. It returns
// the server's info afterwards. Local hosts need nothing.
func Ensure(h transport.Host) (server.Info, error) {
	if h.IsLocal() {
		return server.GatherInfo(), nil
	}
	uname, err := transport.RunRaw(h, nil, "uname -sm")
	if err != nil {
		return server.Info{}, fmt.Errorf("reach %s: %w", h.Dest(), err)
	}
	goos, goarch, err := Platform(string(uname))
	if err != nil {
		return server.Info{}, err
	}
	blob, err := Blob(goos, goarch)
	if err != nil {
		return server.Info{}, err
	}
	sum := sha256.Sum256(blob)
	want := hex.EncodeToString(sum[:])
	info, err := Info(h)
	if err == nil && info.SHA == want && info.Shims {
		return info, nil
	}
	// Write the binary to a temp name and move it into place, so a server in
	// use is never truncated; then let it set up its own shims.
	dir := transport.RemoteHome()
	dst := transport.RemoteServer()
	script := "mkdir -p " + dir + " && cat > " + dst + ".new && chmod 700 " + dst + ".new && mv -f " + dst + ".new " + dst + " && " + transport.RemotePrefix() + dst + " setup"
	if _, err := transport.RunRaw(h, bytes.NewReader(blob), script); err != nil {
		return server.Info{}, fmt.Errorf("install server on %s: %w", h.Dest(), err)
	}
	return Info(h)
}
