// Package remote installs and verifies the helper on a server. The aft binary
// embeds a helper build for each supported server platform; Ensure uploads the
// right one when the server has none or has a different version.
package remote

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arocomputer/aft/internal/helper"
	"github.com/arocomputer/aft/internal/transport"
)

// Helper binaries are placed here by ./x build as helper-<os>-<arch> and are
// embedded into aft for installation on servers.
//
//go:embed embed
var bins embed.FS

// Blob returns the embedded helper for a platform.
func Blob(goos, goarch string) ([]byte, error) {
	b, err := bins.ReadFile("embed/helper-" + goos + "-" + goarch)
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("this aft build has no helper for %s/%s; run ./x build", goos, goarch)
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

// Info runs the helper's info command on the host.
func Info(h transport.Host) (helper.Info, error) {
	var info helper.Info
	if h.IsLocal() {
		return helper.GatherInfo(), nil
	}
	out, err := transport.Output(h, nil, "info")
	if err != nil {
		return info, err
	}
	return info, json.Unmarshal(out, &info)
}

// Ensure installs or updates the helper on the host and prepares it. It returns
// the server's info afterwards. Local hosts need nothing.
func Ensure(h transport.Host) (helper.Info, error) {
	if h.IsLocal() {
		return helper.GatherInfo(), nil
	}
	uname, err := transport.RunRaw(h, nil, "uname -sm")
	if err != nil {
		return helper.Info{}, fmt.Errorf("reach %s: %w", h.Dest(), err)
	}
	goos, goarch, err := Platform(string(uname))
	if err != nil {
		return helper.Info{}, err
	}
	blob, err := Blob(goos, goarch)
	if err != nil {
		return helper.Info{}, err
	}
	sum := sha256.Sum256(blob)
	want := hex.EncodeToString(sum[:])
	info, err := Info(h)
	if err == nil && info.SHA == want && info.Shims {
		return info, nil
	}
	bin := transport.RemoteHome() + "/bin"
	script := "mkdir -p " + bin + " && cat > " + bin + "/helper.new && chmod 700 " + bin + "/helper.new && mv -f " + bin + "/helper.new " + bin + "/helper && " + transport.RemotePrefix() + bin + "/helper setup --sha " + want
	if _, err := transport.RunRaw(h, bytes.NewReader(blob), script); err != nil {
		return helper.Info{}, fmt.Errorf("install helper on %s: %w", h.Dest(), err)
	}
	return Info(h)
}
