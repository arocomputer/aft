package udp

import (
	"io"
	"strings"
	"testing"
	"time"
)

// TestOnlyTheOfferedClientGetsIn connects with the offer and echoes bytes, and
// refuses a client with the wrong token or a server with the wrong certificate.
func TestOnlyTheOfferedClientGetsIn(t *testing.T) {
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}()

	c, err := Dial("127.0.0.1", l.Offer)
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("ping"))
	got := make([]byte, 4)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(c, got); err != nil || string(got) != "ping" {
		t.Fatalf("echo: %q %v", got, err)
	}
	c.Close()

	bad := l.Offer
	bad.Token = strings.Repeat("00", 32)
	if c, err := Dial("127.0.0.1", bad); err == nil {
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		c.Write([]byte("ping"))
		if _, err := io.ReadFull(c, got); err == nil {
			t.Fatal("a wrong token was admitted")
		}
	}

	bad = l.Offer
	bad.Cert = strings.Repeat("ab", 32)
	if _, err := Dial("127.0.0.1", bad); err == nil {
		t.Fatal("a certificate that does not match the offer was trusted")
	}
}
