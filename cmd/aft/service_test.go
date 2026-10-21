package main

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// TestServiceQuoting ensures generated configs preserve paths rather than executing expansions.
func TestServiceQuoting(t *testing.T) {
	linux, err := serviceContent("linux", `/home/a $b%/aft`, "server", "/tmp/a $b% log", 3000, 3999)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(linux, "StandardOutput=append:/tmp/a $b%% log\n") {
		t.Fatal(linux)
	}
	if !strings.Contains(linux, `"/home/a $$b%%/aft"`) || !strings.Contains(linux, "KillMode=control-group") {
		t.Fatal(linux)
	}
	mac, err := serviceContent("darwin", "/Users/a & b/aft", "server", "/tmp/a&b.log", 3000, 3999)
	if err != nil {
		t.Fatal(err)
	}
	d := xml.NewDecoder(strings.NewReader(mac))
	for {
		_, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(mac, "a &amp; b") {
		t.Fatal(mac)
	}
	if _, err := serviceContent("linux", "/bin/aft\nExecStart=bad", "server", "/tmp/log", 3000, 3999); err == nil {
		t.Fatal("accepted newline")
	}
}
