package telemetry

import "testing"

func TestAttemptAggregateTracksStatusModelAndProxy(t *testing.T) {
	monitor := Monitor()
	monitor.RecordAttempt(UpstreamAttempt{
		Model: "m", Tier: "zen", Channel: "anonymous", Proxy: "direct", Status: 403, Success: false,
	})
	monitor.RecordAttempt(UpstreamAttempt{
		Model: "m", Tier: "zen", Channel: "anonymous", Proxy: "direct", Status: 403, Success: false,
	})
	monitor.RecordAttempt(UpstreamAttempt{
		Model: "m", Tier: "zen", Channel: "anonymous", Proxy: "direct", Status: 200, Success: true,
	})
	snapshot := monitor.Snapshot()

	lifetime := snapshot.Upstream.Lifetime
	if got := lifetime.Statuses["403"].Failed; got != 2 {
		t.Fatalf("403 failures = %d, want 2", got)
	}
	if got := lifetime.Statuses["200"].Success; got != 1 {
		t.Fatalf("200 successes = %d, want 1", got)
	}
	if got := lifetime.Models["m"].Total; got != 3 {
		t.Fatalf("model attempts = %d, want 3", got)
	}
	if got := lifetime.Models["m"].Failed; got != 2 {
		t.Fatalf("model failures = %d, want 2", got)
	}
	if got := lifetime.Proxies["direct"].Failed; got != 2 {
		t.Fatalf("proxy failures = %d, want 2", got)
	}
	if got := lifetime.Tiers["zen"].Total; got != 3 {
		t.Fatalf("tier attempts = %d, want 3", got)
	}
	if len(lifetime.Failures) != 1 {
		t.Fatalf("failure breakdown = %+v, want one merged tuple", lifetime.Failures)
	}
	failure := lifetime.Failures[0]
	if failure.Status != 403 || failure.Tier != "zen" || failure.Channel != "anonymous" ||
		failure.Model != "m" || failure.Proxy != "direct" || failure.Count != 2 {
		t.Fatalf("failure tuple = %+v, want 403/zen/anonymous/m/direct ×2", failure)
	}
	if got := snapshot.Upstream.Window.Statuses["403"].Failed; got != 2 {
		t.Fatalf("window 403 failures = %d, want 2", got)
	}
	if len(snapshot.Upstream.Window.Failures) != 1 || snapshot.Upstream.Window.Failures[0].Count != 2 {
		t.Fatalf("window failure breakdown = %+v, want one tuple ×2", snapshot.Upstream.Window.Failures)
	}
}
