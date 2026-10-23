// Package proto is the framed byte protocol between an Aft client and a
// session holder. The same frames travel over a Unix socket on the server and,
// unchanged, through the SSH channel that relays them to the client, so the
// relay in between is a plain byte pump.
//
// A frame is a one-byte type, a four-byte big-endian payload length, and the
// payload. Terminal bytes flow as Input and Output frames; everything else is
// control.
package proto

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Type identifies a frame.
type Type uint8

// Frames from the client to the holder.
const (
	Input       Type = 0x01 // terminal input bytes for the application
	Resize      Type = 0x02 // cols, rows as two uint16
	Detach      Type = 0x03 // the client is leaving on purpose
	BridgeReply Type = 0x04 // answer to a BridgeReq
	Ping        Type = 0x05
	End         Type = 0x06 // stop the application and the session
	Kick        Type = 0x07 // drop whichever client is attached (sent by aft detach)
)

// Frames from the holder to the client.
const (
	Output    Type = 0x11 // terminal output bytes from the application
	Exit      Type = 0x12 // the application exited; payload is a JSON ExitInfo
	Title     Type = 0x13 // reserved: holder-set title
	BridgeReq Type = 0x14 // a shim on the server needs the client; JSON BridgeRequest
	Listeners Type = 0x15 // listening TCP ports on the server, uint16 each
	Hello     Type = 0x16 // sent on attach; JSON HelloInfo
	Pong      Type = 0x17
	Detached  Type = 0x18 // another client took over, or aft detach was run
	Error     Type = 0x19 // human-readable error text; the connection then closes
)

// MaxPayload bounds a single frame.
const MaxPayload = 16 << 20

// Frame is one decoded frame.
type Frame struct {
	Type    Type
	Payload []byte
}

// HelloInfo describes the session a client just attached to.
type HelloInfo struct {
	ID      string   `json:"id"`
	Command []string `json:"command"`
	Dir     string   `json:"dir"`
	PID     int      `json:"pid"`
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
	Started string   `json:"started"`
}

// ExitInfo reports how the application ended.
type ExitInfo struct {
	Code   int    `json:"code"`
	Signal string `json:"signal,omitempty"`
}

// BridgeRequest asks the client to do something on the user's machine.
type BridgeRequest struct {
	ID   uint32          `json:"id"`
	Kind string          `json:"kind"` // "open-url", "clipboard-get"
	Args json.RawMessage `json:"args,omitempty"`
}

// BridgeResponse answers a BridgeRequest.
type BridgeResponse struct {
	ID    uint32 `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  []byte `json:"data,omitempty"`
	Mime  string `json:"mime,omitempty"`
}

// Write sends one frame.
func Write(w io.Writer, t Type, payload []byte) error {
	if len(payload) > MaxPayload {
		return fmt.Errorf("frame too large: %d", len(payload))
	}
	var hdr [5]byte
	hdr[0] = byte(t)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSON sends a frame whose payload is the JSON encoding of v.
func WriteJSON(w io.Writer, t Type, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return Write(w, t, b)
}

// Read reads one frame.
func Read(r *bufio.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxPayload {
		return Frame{}, errors.New("frame too large")
	}
	f := Frame{Type: Type(hdr[0])}
	if n > 0 {
		f.Payload = make([]byte, n)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, err
		}
	}
	return f, nil
}

// EncodeResize packs a terminal size.
func EncodeResize(cols, rows int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint16(b[0:], uint16(cols))
	binary.BigEndian.PutUint16(b[2:], uint16(rows))
	return b[:]
}

// DecodeResize unpacks a terminal size.
func DecodeResize(b []byte) (cols, rows int, ok bool) {
	if len(b) != 4 {
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint16(b[0:])), int(binary.BigEndian.Uint16(b[2:])), true
}

// EncodePorts packs listening ports.
func EncodePorts(ports []int) []byte {
	b := make([]byte, 2*len(ports))
	for i, p := range ports {
		binary.BigEndian.PutUint16(b[2*i:], uint16(p))
	}
	return b
}

// DecodePorts unpacks listening ports.
func DecodePorts(b []byte) []int {
	out := make([]int, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		out = append(out, int(binary.BigEndian.Uint16(b[i:])))
	}
	return out
}
