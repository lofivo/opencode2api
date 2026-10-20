package identity

import (
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
	if rotated.Project != original.Project || rotated.ParentSession != original.ParentSession {
		t.Fatalf("RotateSession changed stable fields: %+v", rotated)
	}

	again := RotateSession(original)
	if again.Session == rotated.Session || again.Message == rotated.Message {
		t.Fatal("two rotations produced the same upstream identity")
	}
}
