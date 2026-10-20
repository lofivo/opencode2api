package httpx

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestBunRequestHeadMatchesFetchLayout(t *testing.T) {
	const body = `{"x":1}`
	req, err := http.NewRequest(http.MethodPost, "https://opencode.ai:443/zen/v1/chat/completions", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "opencode/1.19.3 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14")
	req.Header.Set("x-opencode-client", "cli")
	req.Header.Set("x-opencode-project", "global")
	req.Header.Set("x-opencode-request", "msg_eb5907d57ffe6OjPD1n2KSIRdM")
	req.Header.Set("x-opencode-session", "ses_eb5907d57ffe6OjPD1n2KSIRdM")
	req.Header.Set("x-opencode-session-id", "ses_eb5907d57ffe6OjPD1n2KSIRdM")

	head, decompress, err := bunRequestHead(req, []byte(body))
	if err != nil {
		t.Fatalf("bunRequestHead: %v", err)
	}
	if !decompress {
		t.Fatal("transport did not add Accept-Encoding")
	}
	want := strings.Join([]string{
		"POST /zen/v1/chat/completions HTTP/1.1",
		"Content-Type: application/json",
		"User-Agent: opencode/1.19.3 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14",
		"x-opencode-client: cli",
		"x-opencode-project: global",
		"x-opencode-request: msg_eb5907d57ffe6OjPD1n2KSIRdM",
		"x-opencode-session: ses_eb5907d57ffe6OjPD1n2KSIRdM",
		"x-opencode-session-id: ses_eb5907d57ffe6OjPD1n2KSIRdM",
		"Connection: keep-alive",
		"Accept: */*",
		"Host: opencode.ai",
		"Accept-Encoding: gzip, deflate, br, zstd",
		"Content-Length: 7",
		"",
		"",
	}, "\r\n")
	if string(head) != want {
		t.Fatalf("unexpected request head:\n--- got ---\n%s\n--- want ---\n%s", head, want)
	}
}

func TestBunRequestHeadHonoursCallerEncoding(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://opencode.ai/zen/v1/chat/completions", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	head, decompress, err := bunRequestHead(req, []byte("{}"))
	if err != nil {
		t.Fatalf("bunRequestHead: %v", err)
	}
	if decompress {
		t.Fatal("transport claimed to add Accept-Encoding when the caller supplied one")
	}
	if strings.Count(string(head), "Accept-Encoding:") != 1 {
		t.Fatalf("Accept-Encoding was duplicated:\n%s", head)
	}
	if !strings.Contains(string(head), "Accept-Encoding: identity\r\n") {
		t.Fatalf("caller Accept-Encoding missing:\n%s", head)
	}
}

func TestBunRequestHeadRejectsHeaderInjection(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://opencode.ai/zen/v1/chat/completions", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-test", "value\r\nInjected: yes")
	if _, _, err := bunRequestHead(req, []byte("{}")); err == nil {
		t.Fatal("bunRequestHead accepted a header value containing a CRLF")
	}
}
