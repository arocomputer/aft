// Package bridge is everything that crosses between the machine you type on
// and the server, apart from the terminal stream itself. It is one of Aft's
// three parts, beside the terminal (internal/terminal, on your machine) and
// the session (internal/session, on the server):
//
//   - the connection: transport reaches a host over your OpenSSH, and proto
//     frames the bytes between terminal and session;
//   - the clipboard, all three directions: a copy on the server (OSC 52)
//     sets your clipboard, the server reads your clipboard right after you
//     paste, and a pasted local file is uploaded and pasted as a server path;
//   - opening URLs from the server in your browser;
//   - mirroring the server's listening ports (ports).
//
// The server half lives in shims.go and Store; everything else here runs on
// your machine.
package bridge

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/arocomputer/aft/internal/bridge/proto"
)

// Handle performs one request from the server on this machine and returns
// the response to send back. clipboardOK says whether the user just pasted;
// clipboard reads are refused otherwise, so a program on the server cannot
// read the clipboard without a paste event.
func Handle(req proto.BridgeRequest, clipboardOK bool) proto.BridgeResponse {
	resp := proto.BridgeResponse{ID: req.ID}
	var args map[string]string
	if len(req.Args) > 0 {
		json.Unmarshal(req.Args, &args)
	}
	switch req.Kind {
	case "open-url":
		if err := OpenURL(args["url"]); err != nil {
			return fail(resp, err.Error())
		}
		resp.OK = true
	case "clipboard-get", "clipboard-types":
		if !clipboardOK {
			return fail(resp, "clipboard access requires a recent paste")
		}
		if req.Kind == "clipboard-types" {
			resp.OK, resp.Data = true, []byte(strings.Join(ClipboardTypes(), "\n")+"\n")
			return resp
		}
		mime := args["mime"]
		if mime == "" {
			mime = "text/plain"
		}
		data, err := Clipboard(mime)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.OK, resp.Data, resp.Mime = true, data, mime
	default:
		return fail(resp, "unknown request "+req.Kind)
	}
	return resp
}

func fail(r proto.BridgeResponse, msg string) proto.BridgeResponse {
	r.OK, r.Error = false, msg
	return r
}

// run executes a local tool with a timeout and returns its stdout.
func run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	return cmd.Output()
}

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
