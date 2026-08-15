package keyseq

import (
	"strings"
	"testing"
)

// fallbackHarness runs presses through a fresh processor over mappings and
// reports every command that fired (returned or dispatched through the
// executor) plus the final active sequence.
func fallbackHarness(t *testing.T, mappings map[string]string, presses ...string) (fired []string, active string) {
	t.Helper()
	sp := NewProcessor(func(_, cmd string) bool { fired = append(fired, cmd); return true })
	sp.SetMappings(mappings)
	for _, k := range presses {
		if r := sp.ProcessKey(k); r.Command != "" {
			fired = append(fired, r.Command)
		}
	}
	return fired, sp.GetActiveSequence()
}

// A continuation key reaches the mapping whichever spelling the map uses:
// control, uppercase and lowercase forms of a letter are equivalent within a
// control-started sequence — the mid-progress sequence completes rather than
// being abandoned for a new one.
func TestSequenceContinuationFallbacks(t *testing.T) {
	base := map[string]string{
		"^C":   "cancel_binding",
		"^B P": "window_prior",
	}
	cases := []struct {
		name    string
		mapped  string // the ^B copy binding's spelling in the map
		presses []string
	}{
		{"upper mapping, control press", "^B C", []string{"^B", "^C"}},
		{"lower mapping, control press", "^B c", []string{"^B", "^C"}},
		{"lower mapping, upper press", "^B c", []string{"^B", "C"}},
		{"control mapping, upper press", "^B ^C", []string{"^B", "C"}},
		{"control mapping, lower press", "^B ^C", []string{"^B", "c"}},
	}
	for _, tc := range cases {
		m := map[string]string{tc.mapped: "buffer_duplicate"}
		for k, v := range base {
			m[k] = v
		}
		fired, active := fallbackHarness(t, m, tc.presses...)
		if len(fired) != 1 || fired[0] != "buffer_duplicate" {
			t.Errorf("%s: fired %v, want [buffer_duplicate]", tc.name, fired)
		}
		if active != "" {
			t.Errorf("%s: sequence left active: %q", tc.name, active)
		}
	}
}

// The reported regression, end to end: with "^B c" mapped and ^C carrying its
// own binding AND starting sequences of its own, "^B ^C" must complete the ^B
// sequence — not fire ^C's binding, and not leave a "^C" sequence pending.
func TestSequenceFallbackBeatsNewSequence(t *testing.T) {
	m := map[string]string{
		"^B c":    "buffer_duplicate",
		"^C":      "nav_cancel|cancel|viewport_close",
		"^C X":    "some_c_sequence",
		"^B P":    "window_prior",
		"^B help": `"keys_buffer"`,
	}
	fired, active := fallbackHarness(t, m, "^B", "^C")
	if len(fired) != 1 || fired[0] != "buffer_duplicate" {
		t.Fatalf("mid-progress fallback must beat a new sequence; fired %v", fired)
	}
	if active != "" {
		t.Fatalf("no ^C sequence should be picked up; active %q", active)
	}
}

// The as-pressed spelling always outranks an fallback when both are mapped.
func TestSequenceFallbackPriority(t *testing.T) {
	m := map[string]string{
		"^B ^C": "exact_control",
		"^B C":  "plain_upper",
	}
	fired, _ := fallbackHarness(t, m, "^B", "^C")
	if len(fired) != 1 || fired[0] != "exact_control" {
		t.Fatalf("as-pressed must win over fallback; fired %v", fired)
	}
	fired, _ = fallbackHarness(t, m, "^B", "C")
	if len(fired) != 1 || fired[0] != "plain_upper" {
		t.Fatalf("as-pressed must win over fallback; fired %v", fired)
	}
}

// Fallbacks combine across parts: a three-part chord matches with SEVERAL parts
// spelled differently from the map at once.
func TestSequenceFallbackCrossProduct(t *testing.T) {
	m := map[string]string{
		"^O V ^C": "ruler_cursor",
		"^O A":    "other",
	}
	for _, presses := range [][]string{
		{"^O", "^V", "^C"}, // part 2 given a fallback
		{"^O", "V", "C"},   // part 3 given a fallback
		{"^O", "^V", "c"},  // parts 2 AND 3 given a fallback
	} {
		fired, active := fallbackHarness(t, m, presses...)
		if len(fired) != 1 || fired[0] != "ruler_cursor" {
			t.Errorf("%v: fired %v, want [ruler_cursor]", presses, fired)
		}
		if active != "" {
			t.Errorf("%v: sequence left active: %q", presses, active)
		}
	}
}

// An given a fallback continuation that is a PREFIX of a longer mapping keeps the
// sequence alive: "^B ^C" stays pending when "^B C X" exists, and the final
// key completes it.
func TestSequenceFallbackPrefixContinues(t *testing.T) {
	m := map[string]string{
		"^B C X": "deep_binding",
		"^C":     "cancel_binding",
	}
	sp := NewProcessor(nil)
	sp.SetMappings(m)
	sp.ProcessKey("^B")
	sp.ProcessKey("^C")
	if got := sp.GetActiveSequence(); got != "^B ^C" {
		t.Fatalf("given a fallback prefix should stay pending; active %q", got)
	}
	r := sp.ProcessKey("X")
	if r.Command != "deep_binding" {
		t.Fatalf("completing the given a fallback prefix should fire deep_binding; got %q", r.Command)
	}
}

// The control-equivalence layer stays gated to control-started sequences: a
// plain-key starter does not equate ^C with c.
func TestSequenceFallbackControlGate(t *testing.T) {
	m := map[string]string{
		"q c": "quick_thing",
	}
	fired, _ := fallbackHarness(t, m, "q", "^C")
	for _, f := range fired {
		if f == "quick_thing" {
			t.Fatal("a non-control starter must not fallback ^C to c")
		}
	}
}

// Named keys with control fallbacks resolve in both cases through the variant
// layer (Return ~ ^M ~ M ~ m within a control-started sequence). The name is
// the default fallback vocabulary's, not any one application's.
func TestSequenceFallbackNamedControl(t *testing.T) {
	m := map[string]string{
		"^K m": "mark_binding",
	}
	fired, _ := fallbackHarness(t, m, "^K", "Return")
	if len(fired) != 1 || fired[0] != "mark_binding" {
		t.Fatalf("Return should reach ^K m via ^M -> m; fired %v", fired)
	}
	if strings.Contains(strings.Join(fired, " "), "insert") {
		t.Fatalf("no fallthrough insert expected; fired %v", fired)
	}
}
