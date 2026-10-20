package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/identity"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
	"opencode2api/internal/telemetry"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode prepared body: %v", err)
	}
	return payload
}

func TestPrepareAnonymousBodyV1InjectsDeprecatedReadTool(t *testing.T) {
	body := []byte(`{"model":"m","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	prepared, alias := prepareAnonymousBody(body, wire.Chat, true)
	if alias != "" {
		t.Fatalf("unexpected alias %q for a request without a read tool", alias)
	}
	payload := decodeBody(t, prepared)
	if streaming, _ := payload["stream"].(bool); !streaming {
		t.Fatal("v1 body was not forced to stream")
	}
	options, _ := payload["stream_options"].(map[string]any)
	if include, _ := options["include_usage"].(bool); !include {
		t.Fatal("v1 Chat body did not request stream usage")
	}
	tools, _ := payload["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("v1 injected %d tools, want 1", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	function, _ := tool["function"].(map[string]any)
	if name := function["name"]; name != v1ReadTool {
		t.Fatalf("v1 tool name = %v, want %q", name, v1ReadTool)
	}
	if description, _ := function["description"].(string); !strings.Contains(description, "DEPRECATED") {
		t.Fatalf("v1 stub description does not mark the tool deprecated: %q", description)
	}
}

func TestPrepareAnonymousBodyV1AliasesClientReadTool(t *testing.T) {
	body := []byte(`{
		"model":"m",
		"stream":true,
		"tools":[{"type":"function","function":{"name":"read_file","description":"client read","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}}],
		"tool_choice":{"type":"function","function":{"name":"read_file"}},
		"messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}"}}]}]
	}`)
	prepared, alias := prepareAnonymousBody(body, wire.Chat, true)
	if alias != "read_file" {
		t.Fatalf("alias = %q, want read_file", alias)
	}
	payload := decodeBody(t, prepared)
	tools, _ := payload["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("aliased body has %d tools, want 1", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	function, _ := tool["function"].(map[string]any)
	if name := function["name"]; name != v1ReadTool {
		t.Fatalf("aliased tool name = %v, want %q", name, v1ReadTool)
	}
	if description, _ := function["description"].(string); description != "client read" {
		t.Fatalf("aliasing changed the client description: %q", description)
	}
	choice, _ := payload["tool_choice"].(map[string]any)
	choiceFunction, _ := choice["function"].(map[string]any)
	if name := choiceFunction["name"]; name != v1ReadTool {
		t.Fatalf("tool_choice name = %v, want %q", name, v1ReadTool)
	}
	messages, _ := payload["messages"].([]any)
	message, _ := messages[0].(map[string]any)
	calls, _ := message["tool_calls"].([]any)
	call, _ := calls[0].(map[string]any)
	callFunction, _ := call["function"].(map[string]any)
	if name := callFunction["name"]; name != v1ReadTool {
		t.Fatalf("historical tool call name = %v, want %q", name, v1ReadTool)
	}
}

func TestPrepareAnonymousBodyV2InjectsGenuineCoreTools(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"tools":[{"type":"function","function":{"name":"read_file","description":"client read","parameters":{"type":"object"}}}]}`)
	prepared, alias := prepareAnonymousBody(body, wire.Chat, false)
	if alias != "" {
		t.Fatalf("v2 body unexpectedly returned alias %q", alias)
	}
	payload := decodeBody(t, prepared)
	tools, _ := payload["tools"].([]any)
	names := map[string]map[string]any{}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		names[name] = function
	}
	if _, ok := names["read_file"]; !ok {
		t.Fatal("v2 shaper dropped an unrecognised client tool")
	}
	for _, name := range v2CoreTools {
		function, ok := names[name]
		if !ok {
			t.Fatalf("v2 shaper did not inject %q", name)
		}
		if description, _ := function["description"].(string); description == "" {
			t.Fatalf("v2 tool %q has no description", name)
		}
		if parameters, ok := function["parameters"].(map[string]any); !ok || parameters["type"] != "object" {
			t.Fatalf("v2 tool %q has no object parameters: %#v", name, function["parameters"])
		}
	}
}

func TestSetV1Headers(t *testing.T) {
	const session = "ses_eb5907d57ffe6OjPD1n2KSIRdM"
	ids := identity.RequestIDs{
		Session: session, Message: "msg_eb5907d57ffe6OjPD1n2KSIRdM",
		Project: "global", ParentSession: "ses_eb5b07785ffeBUGIG4j0rMvFM8",
	}
	header := http.Header{}
	setV1Headers(header, wire.Chat, []byte(`{}`), ids)
	if got := header.Get("x-opencode-session"); got != session {
		t.Fatalf("x-opencode-session = %q", got)
	}
	if got := header.Get("x-opencode-session-id"); got != session {
		t.Fatalf("x-opencode-session-id = %q", got)
	}
	if got := header.Get("x-opencode-request"); got != ids.Message {
		t.Fatalf("x-opencode-request = %q, want %q", got, ids.Message)
	}
	if got := header.Get("x-opencode-project"); got != "global" {
		t.Fatalf("x-opencode-project = %q, want global", got)
	}
	if got := header.Get("x-opencode-parent-session-id"); got != ids.ParentSession {
		t.Fatalf("x-opencode-parent-session-id = %q", got)
	}
	if got := header.Get("x-parent-session-id"); got != ids.ParentSession {
		t.Fatalf("x-parent-session-id = %q", got)
	}
	if header.Get("x-session-affinity") != "" || header.Get("X-Session-Id") != "" {
		t.Fatal("v1 headers leaked v2-only session headers")
	}
	if ua := header.Get("User-Agent"); !strings.HasPrefix(ua, "opencode/1.19.3 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14") {
		t.Fatalf("v1 User-Agent = %q", ua)
	}
	if accept := header.Get("Accept"); accept != "*/*" {
		t.Fatalf("v1 Accept = %q", accept)
	}
}

func TestSetV2Headers(t *testing.T) {
	const session = "ses_eb5907d57ffe6OjPD1n2KSIRdM"
	ids := identity.RequestIDs{
		Session: session, Message: "msg_eb5907d57ffe6OjPD1n2KSIRdM",
		Project: "conversation", ParentSession: "ses_eb5b07785ffeBUGIG4j0rMvFM8",
	}
	header := http.Header{}
	setV2Headers(header, wire.Chat, ids)
	for _, name := range []string{"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "X-Session-Id"} {
		if got := header.Get(name); got != session {
			t.Fatalf("%s = %q, want %q", name, got, session)
		}
	}
	if got := header.Get("x-opencode-request"); got != "" {
		t.Fatalf("v2 headers must not send x-opencode-request, got %q", got)
	}
	if project := header.Get("x-opencode-project"); !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(project) {
		t.Fatalf("v2 project = %q, want 40 lowercase hex characters", project)
	}
	if got := header.Get("x-parent-session-id"); got != ids.ParentSession {
		t.Fatalf("x-parent-session-id = %q", got)
	}
	if got := header.Get("x-opencode-parent-session-id"); got != "" {
		t.Fatalf("v2 headers must not send x-opencode-parent-session-id, got %q", got)
	}
	if ua := header.Get("User-Agent"); ua != "opencode/latest/2.0.32/cli" {
		t.Fatalf("v2 User-Agent = %q", ua)
	}
	if accept := header.Get("Accept"); accept != "application/json, text/event-stream" {
		t.Fatalf("v2 Accept = %q", accept)
	}
}

func TestProjectIDShapes(t *testing.T) {
	if got := v1ProjectID("global"); got != "global" {
		t.Fatalf("v1ProjectID(global) = %q", got)
	}
	const commit = "0123456789abcdef0123456789abcdef01234567"
	if got := v1ProjectID(commit); got != commit {
		t.Fatalf("v1ProjectID(commit) = %q, want the commit unchanged", got)
	}
	if got := v1ProjectID("some conversation"); !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(got) {
		t.Fatalf("v1ProjectID(other) = %q, want 40 lowercase hex characters", got)
	}
	if first, second := v2ProjectID("project"), v2ProjectID("project"); first != second {
		t.Fatalf("v2ProjectID is not deterministic: %q != %q", first, second)
	}
}

func TestAnonymous403RetrySucceedsWithFreshSession(t *testing.T) {
	var sessions []string
	fake := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sessions = append(sessions, req.Header.Get("x-opencode-session"))
		if len(sessions) <= 2 {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"FreeTierError"}}`)),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    req,
		}, nil
	})

	proxy := &proxyTransport{index: 0, name: "direct", client: &http.Client{Transport: fake}, bun: &http.Client{Transport: fake}}
	proxy.healthy.Store(true)
	transports := &proxyTransports{items: []*proxyTransport{proxy}}
	g := &Gateway{
		cfg:        config.Config{Upstream: config.UpstreamConfig{Anonymous: "https://opencode.ai/inference"}},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		transports: transports,
		anonymous:  anonymousPool(true, transports, time.Minute),
		catalog:    models.Catalog(config.TierZen, nil),
		monitor:    telemetry.Monitor(),
	}
	route := models.Route{ID: "test-free", Tier: config.TierZen, Protocol: wire.Chat, Anonymous: true}
	bodies := map[config.Tier][]byte{config.TierZen: []byte(`{"model":"test-free","stream":true,"messages":[{"role":"user","content":"hi"}]}`)}
	ctx := telemetry.WithRequestMeta(context.Background(), &telemetry.RequestMeta{Request: "req_test"})
	resp, err, attempts := g.doAnonymousUpstream(ctx, route, bodies, identity.RequestIDs{
		Session: identity.CanonicalSessionID("initial"), Request: "req_test", Message: identity.AscendingID("msg", time.Now()),
	}, 0)
	if err != nil {
		t.Fatalf("doAnonymousUpstream: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("final response = %#v, want 200", resp)
	}
	defer resp.Body.Close()
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (two rejected lanes + one recovered v1 attempt)", attempts)
	}
	if len(sessions) != 3 {
		t.Fatalf("fake transport saw %d attempts, want 3", len(sessions))
	}
	if sessions[2] == sessions[0] {
		t.Fatalf("recovery reused the rejected session %q", sessions[2])
	}
	if meta := telemetry.MetaFromContext(ctx); meta.SessionRetries != 1 || meta.Session != sessions[2] {
		t.Fatalf("meta = session_retries %d, session %q; want 1, %q", meta.SessionRetries, meta.Session, sessions[2])
	}
}

func TestAnonymous403RotatesSessionAndStopsAfterTwoRetries(t *testing.T) {
	type seenAttempt struct {
		session string
		message string
		path    string
	}
	var seen []seenAttempt
	fake := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, seenAttempt{
			session: req.Header.Get("x-opencode-session"),
			message: req.Header.Get("x-opencode-request"),
			path:    req.URL.Path,
		})
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"FreeTierError","message":"rejected"}}`)),
			Request:    req,
		}, nil
	})

	proxy := &proxyTransport{
		index: 0, name: "direct", client: &http.Client{Transport: fake}, bun: &http.Client{Transport: fake},
	}
	proxy.healthy.Store(true)
	transports := &proxyTransports{items: []*proxyTransport{proxy}}
	cfg := config.Config{Upstream: config.UpstreamConfig{Anonymous: "https://opencode.ai/inference"}}
	g := &Gateway{
		cfg:        cfg,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		transports: transports,
		anonymous:  anonymousPool(true, transports, time.Minute),
		catalog:    models.Catalog(config.TierZen, nil),
		monitor:    telemetry.Monitor(),
	}
	route := models.Route{ID: "test-free", Tier: config.TierZen, Protocol: wire.Chat, Anonymous: true}
	bodies := map[config.Tier][]byte{config.TierZen: []byte(`{"model":"test-free","stream":true,"messages":[{"role":"user","content":"hi"}]}`)}
	ctx := telemetry.WithRequestMeta(context.Background(), &telemetry.RequestMeta{Request: "req_test"})
	resp, err, attempts := g.doAnonymousUpstream(ctx, route, bodies, identity.RequestIDs{
		Session: identity.CanonicalSessionID("initial"), Request: "req_test", Message: identity.AscendingID("msg", time.Now()),
	}, 0)
	if err != nil {
		t.Fatalf("doAnonymousUpstream: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("final response = %#v, want 403", resp)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read final error body: %v", err)
	}
	if !strings.Contains(string(body), "FreeTierError") {
		t.Fatalf("final error body was consumed by snippet logging: %q", body)
	}
	if attempts != 6 {
		t.Fatalf("attempts = %d, want 6 (3 identities x 2 lanes)", attempts)
	}
	if len(seen) != attempts {
		t.Fatalf("fake transport saw %d attempts, want %d", len(seen), attempts)
	}
	meta := telemetry.MetaFromContext(ctx)
	if meta.SessionRetries != 2 {
		t.Fatalf("SessionRetries = %d, want 2", meta.SessionRetries)
	}
	if meta.Attempts != 6 {
		t.Fatalf("meta.Attempts = %d, want 6", meta.Attempts)
	}
	if meta.Session != seen[4].session {
		t.Fatalf("meta.Session = %q, want the last rotated session %q", meta.Session, seen[4].session)
	}

	for round := 0; round < 3; round++ {
		v1, v2 := seen[round*2], seen[round*2+1]
		if !strings.HasSuffix(v1.path, "/zen/v1/chat/completions") {
			t.Fatalf("round %d first path = %q", round, v1.path)
		}
		if !strings.HasSuffix(v2.path, "/inference/v1/chat/completions") {
			t.Fatalf("round %d second path = %q", round, v2.path)
		}
		if v1.session != v2.session {
			t.Fatalf("round %d lanes used different sessions: %q != %q", round, v1.session, v2.session)
		}
		if v1.message == "" {
			t.Fatalf("round %d v1 request did not carry x-opencode-request", round)
		}
		if v2.message != "" {
			t.Fatalf("round %d v2 request leaked x-opencode-request %q", round, v2.message)
		}
		if round > 0 && v1.session == seen[(round-1)*2].session {
			t.Fatalf("round %d reused the rejected session %q", round, v1.session)
		}
		if round > 0 && v1.message == seen[(round-1)*2].message {
			t.Fatalf("round %d reused the rejected message id %q", round, v1.message)
		}
	}
}
