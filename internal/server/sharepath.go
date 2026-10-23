package server

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// SharePath returns the server directory a Samba share name maps to, by reading
// smb.conf. It lets a client that has an SMB mount learn the real server path
// behind the share without the user typing it. The lookup is case-insensitive
// and ignores the special [global] section.
func SharePath(share string) (string, error) {
	for _, conf := range []string{"/etc/samba/smb.conf", "/usr/local/etc/smb.conf", "/opt/homebrew/etc/smb.conf"} {
		f, err := os.Open(conf)
		if err != nil {
			continue
		}
		path, found := scanShare(f, share)
		f.Close()
		if found {
			if path == "" {
				return "", fmt.Errorf("share %q has no path", share)
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("no Samba share %q found on this server", share)
}

// scanShare walks an smb.conf-style stream for the named section's path.
func scanShare(r *os.File, share string) (string, bool) {
	sc := bufio.NewScanner(r)
	inSection := false
	path := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			if inSection {
				// Left the matched section without this being the one.
				return path, true
			}
			inSection = strings.EqualFold(name, share)
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "path") {
			path = strings.TrimSpace(value)
		}
	}
	return path, inSection
}
