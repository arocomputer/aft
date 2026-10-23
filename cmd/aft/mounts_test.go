package main

import "testing"

// TestSplitUNCandNFS parse the two mount source forms.
func TestSplitUNCandNFS(t *testing.T) {
	if s, sh := splitUNC("//fschr@100.65.233.79/Room"); s != "100.65.233.79" || sh != "Room" {
		t.Fatalf("UNC: %q %q", s, sh)
	}
	if s, sh := splitUNC("//host/Share/sub"); s != "host" || sh != "Share" {
		t.Fatalf("UNC sub: %q %q", s, sh)
	}
	if s, e := splitNFS("server:/export/path"); s != "server" || e != "/export/path" {
		t.Fatalf("NFS: %q %q", s, e)
	}
}
