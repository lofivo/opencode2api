package identity

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanonicalSessionIDShapeAndDeterminism(t *testing.T) {
	const signal = "conversation:first-user-turn"
	first := CanonicalSessionID(signal)
	second := CanonicalSessionID(signal)
	if first != second {
		t.Fatalf("CanonicalSessionID is not deterministic: %q != %q", first, second)
	}
	if !canonicalSessionPattern.MatchString(first) {
		t.Fatalf("session %q does not match the canonical ses_ shape", first)
	}
	if !descendingTimePartPattern.MatchString(first[4:16]) {
		t.Fatalf("session %q does not carry a descending timestamp part", first)
	}
	if other := CanonicalSessionID("conversation:other"); other == first {
		t.Fatalf("different signals produced the same session %q", first)
	}
}

func TestCanonicalSessionIDKeepsOfficialSession(t *testing.T) {
	const official = "ses_eb5907d57ffe6OjPD1n2KSIRdM"
	if got := CanonicalSessionID(official); got != official {
		t.Fatalf("official session was rewritten: got %q, want %q", got, official)
	}
}

func TestRotateSession(t *testing.T) {
	original := RequestIDs{
		Session:       CanonicalSessionID("conversation"),
		Request:       "req_local_trace",
		Message:       AscendingID("msg", time.Now()),
		Project:       "global",
		ParentSession: "ses_parent",
	}
	rotated := RotateSession(original)
	if rotated.Session == original.Session {
		t.Fatal("RotateSession kept the rejected session")
	}
	if !canonicalSessionPattern.MatchString(rotated.Session) {
		t.Fatalf("rotated session %q does not match the canonical ses_ shape", rotated.Session)
	}
	if rotated.Message == original.Message {
		t.Fatal("RotateSession kept the rejected message id")
	}
	if !messageIDPattern.MatchString(rotated.Message) {
		t.Fatalf("rotated message %q does not match the msg_ shape", rotated.Message)
	}
	if rotated.Request != original.Request {
		t.Fatalf("RotateSession changed the local request id: got %q, want %q", rotated.Request, original.Request)
	}
	if rotated.Project != original.Project {
		t.Fatalf("RotateSession changed stable fields: %+v", rotated)
	}
	if rotated.ParentSession == original.ParentSession || !isOfficialSessionID(rotated.ParentSession) {
		t.Fatalf("RotateSession did not replace the rejected parent: %+v", rotated)
	}
	if rotated.ChildSession != "" {
		t.Fatalf("RotateSession introduced a child session: %+v", rotated)
	}

	again := RotateSession(original)
	if again.Session == rotated.Session || again.Message == rotated.Message {
		t.Fatal("two rotations produced the same upstream identity")
	}
}

func TestRotateSessionPreservesSessionRelationships(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent string
		child  string
	}{
		{name: "no parent or child"},
		{name: "root is also child", child: "root"},
		{name: "root is parent", parent: "root", child: "child"},
		{name: "root is child", parent: "parent", child: "root"},
		{name: "all distinct", parent: "parent", child: "child"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := RequestIDs{Session: "root", ChildSession: tc.child, ParentSession: tc.parent}
			rotated := RotateSession(original)
			before := []string{original.Session, original.ChildSession, original.ParentSession}
			after := []string{rotated.Session, rotated.ChildSession, rotated.ParentSession}
			for i, previous := range before {
				if previous == "" {
					if after[i] != "" {
						t.Fatalf("absent session field %d became %q", i, after[i])
					}
					continue
				}
				if previous == after[i] || !isOfficialSessionID(after[i]) {
					t.Fatalf("session field %d did not rotate to a valid session: %q", i, after[i])
				}
				for j, other := range before {
					if (previous == other) != (after[i] == after[j]) {
						t.Fatalf("session relationship changed: before=%v, after=%v", before, after)
					}
				}
			}
		})
	}
}

func TestDeriveRequestIDsScopesParentAndChild(t *testing.T) {
	for _, header := range []string{"x-opencode-parent-session-id", "x-parent-session-id", "metadata"} {
		t.Run(header, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			req.Header.Set("x-opencode-session", "root")
			req.Header.Set("x-opencode-session-id", "child")
			body := map[string]any{}
			if header == "metadata" {
				body["metadata"] = map[string]any{"parent_session_id": "parent"}
			} else {
				req.Header.Set(header, "parent")
			}
			first := DeriveRequestIDs(req, body, "server-key-a")
			again := DeriveRequestIDs(req, body, "server-key-a")
			other := DeriveRequestIDs(req, body, "server-key-b")
			for _, values := range [][3]string{
				{first.Session, again.Session, other.Session},
				{first.ChildSession, again.ChildSession, other.ChildSession},
				{first.ParentSession, again.ParentSession, other.ParentSession},
			} {
				if !isOfficialSessionID(values[0]) || values[0] != values[1] || values[0] == values[2] {
					t.Fatalf("session is invalid, unstable or shared across callers: %v", values)
				}
			}
		})
	}
}

func TestDeriveRequestIDsKeepsOfficialSubsession(t *testing.T) {
	const parent = "ses_eb5b07785ffeBUGIG4j0rMvFM8"
	const child = "ses_eb5907d57ffe6OjPD1n2KSIRdM"
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("x-opencode-session", parent)
	req.Header.Set("x-opencode-session-id", child)
	req.Header.Set("x-opencode-parent-session-id", parent)
	req.Header.Set("x-parent-session-id", "legacy-parent")
	for _, scope := range []string{"server-key-a", "server-key-b"} {
		ids := DeriveRequestIDs(req, nil, scope)
		if ids.Session != parent || ids.ChildSession != child || ids.ParentSession != parent {
			t.Fatalf("official parent/child changed or legacy alias overrode parent: %+v", ids)
		}
	}
}
