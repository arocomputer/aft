package main

import (
	"reflect"
	"testing"
)

// TestListeners excludes addresses the localhost tunnel cannot reach and deduplicates.
func TestListeners(t *testing.T) {
	output := `LISTEN 0 128 0.0.0.0:3000 0.0.0.0:*
LISTEN 0 128 127.0.0.1:8080 0.0.0.0:*
LISTEN 0 128 127.0.0.1:3000 0.0.0.0:*
LISTEN 0 128 100.64.0.1:9000 0.0.0.0:*
LISTEN 0 128 [::1]:4000 [::]:*
LISTEN 0 128 0.0.0.0:22 0.0.0.0:*
LISTEN 0 128 *:5000 *:*
malformed`
	if got, want := listeners(output), []int{3000, 5000, 8080}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
