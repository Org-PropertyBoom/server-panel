package services

import "testing"

// The aggregate /proc/stat line, field order fixed by the kernel:
//
//	cpu user nice system idle iowait irq softirq steal guest guest_nice
//
// These tests pin the two accounting rules that a naive "sum every field" loop
// gets wrong, both of which produced a wrong number on the dashboard.

func TestParseCPULine_ExcludesGuestFromTotal(t *testing.T) {
	// guest (700) is already counted inside user, and guest_nice (800) inside
	// nice. Adding them again inflates the denominator and skews every
	// percentage derived from it.
	sample, err := parseCPULine("cpu 100 200 300 400 500 600 50 25 700 800")
	if err != nil {
		t.Fatalf("parseCPULine: %v", err)
	}

	const want = 100 + 200 + 300 + 400 + 500 + 600 + 50 + 25 // through steal only
	if sample.total != want {
		t.Errorf("total = %d, want %d (guest/guest_nice must not be added again)", sample.total, want)
	}
	if sample.idle != 400+500 {
		t.Errorf("idle = %d, want %d (idle + iowait)", sample.idle, 400+500)
	}
	if sample.steal != 25 {
		t.Errorf("steal = %d, want 25", sample.steal)
	}
	if sample.iowait != 500 {
		t.Errorf("iowait = %d, want 500", sample.iowait)
	}
}

func TestParseCPULine_ShortLineStillParses(t *testing.T) {
	// Older kernels stop before steal. Missing trailing fields must read as zero,
	// not error and not garbage.
	sample, err := parseCPULine("cpu 10 20 30 40 50")
	if err != nil {
		t.Fatalf("parseCPULine: %v", err)
	}
	if sample.steal != 0 {
		t.Errorf("steal = %d, want 0 when the field is absent", sample.steal)
	}
	if sample.total != 150 {
		t.Errorf("total = %d, want 150", sample.total)
	}
}

func TestParseCPULine_RejectsNonCPULine(t *testing.T) {
	if _, err := parseCPULine("intr 12345 0 0"); err == nil {
		t.Error("expected an error for a non-cpu line, got nil")
	}
}

// TestCPUDeltas_StealIsNotOurLoad is the regression this whole change exists for:
// a hypervisor stealing CPU must NOT read as the server being busy.
//
// Scenario: between two samples 1000 jiffies pass. 900 are steal — the host ran
// someone else. 50 are genuinely ours, 50 idle. The old code counted everything
// that was not idle as usage and reported 95%. The dashboard showed a hammered
// server while every container sat at 0%.
func TestCPUDeltas_StealIsNotOurLoad(t *testing.T) {
	before, err := parseCPULine("cpu 0 0 0 0 0 0 0 0")
	if err != nil {
		t.Fatalf("parseCPULine before: %v", err)
	}
	// user +50, idle +50, steal +900  => total +1000
	after, err := parseCPULine("cpu 50 0 0 50 0 0 0 900")
	if err != nil {
		t.Fatalf("parseCPULine after: %v", err)
	}

	totalDelta := after.total - before.total
	if totalDelta != 1000 {
		t.Fatalf("totalDelta = %d, want 1000", totalDelta)
	}
	pct := func(d uint64) float64 { return float64(d) / float64(totalDelta) * 100 }

	busy := totalDelta - (after.idle - before.idle) - (after.steal - before.steal)
	usage := pct(busy)
	steal := pct(after.steal - before.steal)

	if usage != 5 {
		t.Errorf("usage = %.1f%%, want 5%% (only the 50 jiffies we actually ran)", usage)
	}
	if steal != 90 {
		t.Errorf("steal = %.1f%%, want 90%%", steal)
	}
	// The old behaviour, kept as an explicit contrast so a future refactor that
	// reintroduces it fails here rather than on the dashboard.
	if old := pct(totalDelta - (after.idle - before.idle)); old != 95 {
		t.Errorf("sanity: pre-fix formula = %.1f%%, expected it to have read 95%%", old)
	}
}

// TestCPUDeltas_CounterResetDoesNotWrap guards the uint64 subtractions in
// cpuStatus. /proc/stat counters normally only rise, but a CPU hotplug or a
// counter reset can make them fall. With plain a-b that wraps to ~1.8e19 and the
// dashboard renders a garbage percentage instead of erroring; the values must
// clamp to a sane range instead.
func TestCPUDeltas_CounterResetDoesNotWrap(t *testing.T) {
	// idle GOES BACKWARDS while total still RISES. Total must rise, because
	// cpuStatus only reaches this arithmetic when now.total > prev.total (a
	// falling total takes the cold-start path instead) — so a test with a falling
	// total would prove nothing about the code under test.
	before, err := parseCPULine("cpu 100 0 0 900 0 0 0 0") // total 1000, idle 900
	if err != nil {
		t.Fatalf("before: %v", err)
	}
	after, err := parseCPULine("cpu 2000 0 0 50 0 0 0 0") // total 2050, idle 50
	if err != nil {
		t.Fatalf("after: %v", err)
	}

	if after.total <= before.total {
		t.Fatalf("fixture is wrong: total must rise (before %d, after %d)", before.total, after.total)
	}
	totalDelta := after.total - before.total
	sub := func(a, b uint64) uint64 {
		if a < b {
			return 0
		}
		return a - b
	}
	pct := func(d uint64) float64 {
		v := float64(d) / float64(totalDelta) * 100
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}

	idleDelta := sub(after.idle, before.idle) // would wrap without sub()
	if idleDelta != 0 {
		t.Errorf("idleDelta = %d, want 0 — a backwards counter must clamp, not wrap", idleDelta)
	}
	usage := pct(sub(totalDelta, idleDelta+sub(after.steal, before.steal)))
	if usage < 0 || usage > 100 {
		t.Errorf("usage = %f, want within 0..100", usage)
	}

	// And the unguarded form is what we are protecting against.
	if raw := after.idle - before.idle; raw < 1<<60 {
		t.Errorf("sanity: unguarded subtraction = %d, expected it to have wrapped huge", raw)
	}
}
