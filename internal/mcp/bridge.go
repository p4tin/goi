package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Bridge relays MCP from a stdio client to an HTTP server: each line read
// from in is POSTed to url, and each JSON response is written to out as one
// line. It returns when in ends.
func Bridge(ctx context.Context, in io.Reader, out io.Writer, url, token string, client *http.Client) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	r := bufio.NewReaderSize(in, 64*1024)
	for {
		line, err := r.ReadBytes('\n')
		if msg := bytes.TrimSpace(line); len(msg) > 0 {
			resp, perr := post(ctx, client, url, token, msg)
			if perr != nil {
				resp = bridgeError(msg, perr)
			}
			if len(resp) > 0 {
				if _, werr := out.Write(append(bytes.TrimSpace(resp), '\n')); werr != nil {
					return werr
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func post(ctx context.Context, client *http.Client, url, token string, msg []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return nil, nil
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("goi MCP server: HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return body, nil
}

// bridgeError turns a transport failure into a JSON-RPC error for msg's id
// (nothing for notifications).
func bridgeError(msg []byte, err error) []byte {
	var req struct {
		ID *json.RawMessage `json:"id"`
	}
	if json.Unmarshal(msg, &req) != nil || req.ID == nil {
		return nil
	}
	b, _ := json.Marshal(response{JSONRPC: "2.0", ID: *req.ID, Error: &rpcError{CodeInternalError, err.Error()}})
	return b
}

// RunBridge is the `goi mcp-bridge --url U --token T` command: it relays
// stdin/stdout to the MCP server at U and returns the exit code.
func RunBridge(args []string) int {
	fs := flag.NewFlagSet("mcp-bridge", flag.ContinueOnError)
	url := fs.String("url", "", "Goi MCP server URL")
	token := fs.String("token", "", "bearer token (default: $GOI_MCP_TOKEN)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *url == "" {
		fmt.Fprintln(os.Stderr, "goi mcp-bridge: --url is required")
		return 2
	}
	if *token == "" {
		*token = os.Getenv("GOI_MCP_TOKEN")
	}
	if err := Bridge(context.Background(), os.Stdin, os.Stdout, *url, *token, nil); err != nil {
		fmt.Fprintln(os.Stderr, "goi mcp-bridge:", err)
		return 1
	}
	return 0
}
