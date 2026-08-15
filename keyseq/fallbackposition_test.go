package keyseq

import "testing"

// posGroups are a realistic mixture: named keys with control spellings, and
// symbols with word spellings.
var posGroups = []FallbackGroup{
	{"back", "^H", "backspace"},
	{"tab", "^I"},
	{"return", "enter", "^M"},
	{"esc", "escape", "^[", "^3"},
	{"-", "minus"},
	{"=", "equals"},
}

func seqResolves(t *testing.T, bound string, keys ...string) string {
	t.Helper()
	p := NewProcessor(nil)
	p.SetFallbackGroups(posGroups)
	p.SetMappings(map[string]string{bound: "TARGET"})
	var last string
	for _, k := range keys {
		last = p.ProcessKey(k).Command
	}
	return last
}

func mustResolve(t *testing.T, bound string, keys ...string) {
	t.Helper()
	if got := seqResolves(t, bound, keys...); got != "TARGET" {
		t.Errorf("bound %q, keys %v -> %q, want TARGET", bound, keys, got)
	}
}

func mustNotResolve(t *testing.T, bound string, keys ...string) {
	t.Helper()
	if got := seqResolves(t, bound, keys...); got == "TARGET" {
		t.Errorf("bound %q, keys %v matched, and must not", bound, keys)
	}
}

// An fallback resolves wherever it sits in a chord — as the starter, in the
// middle, at the tail, and in several positions at once. Matching used to swap
// a single spelling at the tail only, so a chord bound "esc x" could not be
// typed with Escape's control form at all: the starter was not recognized, the
// prefix was never held, and the chord simply never began.
func TestFallbackResolvesInEveryPosition(t *testing.T) {
	// Starter.
	mustResolve(t, "esc x", "^[", "x")
	mustResolve(t, "esc x", "escape", "x")
	mustResolve(t, "^[ x", "esc", "x")
	mustResolve(t, "esc a b", "^[", "a", "b")
	mustResolve(t, "minus x", "-", "x")

	// Middle, and several at once.
	mustResolve(t, "^K minus x", "^K", "-", "x")
	mustResolve(t, "^K minus minus", "^K", "-", "-")
	mustResolve(t, "^K equals minus", "^K", "=", "-")

	// Tail, which always worked.
	mustResolve(t, "^K minus", "^K", "-")
	mustResolve(t, "^K return", "^K", "^M")
}

// The control/case layer is what lets a chord bound "^B M" be completed with
// Ctrl-M, or with the Return key. It must survive fallback expansion: the fallback
// group's primary must not swallow "^M" before the letter variants are tried.
func TestControlVariantSurvivesFallbackExpansion(t *testing.T) {
	mustResolve(t, "^B M", "^B", "^M")
	mustResolve(t, "^B M", "^B", "return")
	mustResolve(t, "^B m", "^B", "^M")
	mustResolve(t, "^B m", "^B", "return")
	mustResolve(t, "^B return", "^B", "^M")
	mustResolve(t, "^B I", "^B", "tab")
	mustResolve(t, "^B H", "^B", "back")
}

// ...and it stays CONTEXTUAL. A plain letter is not its control chord outside
// a control-started sequence, however the key is spelled.
func TestControlVariantStaysContextual(t *testing.T) {
	mustNotResolve(t, "M", "^M")
	mustNotResolve(t, "m", "^M")
	mustNotResolve(t, "M", "return")
	mustNotResolve(t, "M x", "^M", "x")
	mustNotResolve(t, "I", "tab")
}

// A single key still resolves through its group, which is the path a chordless
// binding takes (no parts loop runs for it).
func TestSingleKeyFallbackStillResolves(t *testing.T) {
	mustResolve(t, "return", "^M")
	mustResolve(t, "esc", "^[")
	mustResolve(t, "minus", "-")
	mustResolve(t, "-", "minus")
}

// An exact mapping outranks an given a fallback one: the as-pressed spelling enumerates
// first, so a keymap that names both spellings gets the one the user typed.
func TestExactSpellingWinsOverFallback(t *testing.T) {
	p := NewProcessor(nil)
	p.SetFallbackGroups(posGroups)
	p.SetMappings(map[string]string{
		"^K -":     "by_symbol",
		"^K minus": "by_word",
	})
	p.ProcessKey("^K")
	if got := p.ProcessKey("-").Command; got != "by_symbol" {
		t.Errorf("pressed the symbol, got %q, want the symbol's own binding", got)
	}
}

// Fallbacks add lookups; they never rewrite the event. Two spellings a modern
// terminal CAN distinguish (the kitty protocol reports Ctrl-H and Backspace
// separately, where a legacy terminal sent one byte for both) stay separately
// bindable — the fallback only fills in when the keymap left one of them unnamed.
// Nothing about a key's identity is discarded by declaring it able to fall back.
func TestFallbackesNeverDiscardADistinction(t *testing.T) {
	groups := []FallbackGroup{{"Backspace", "^H"}}
	bind := func(m map[string]string, keys ...string) string {
		p := NewProcessor(nil)
		p.SetFallbackGroups(groups)
		p.SetMappings(m)
		var last string
		for _, k := range keys {
			last = p.ProcessKey(k).Command
		}
		return last
	}

	// Named separately: each keeps its own meaning, alone and in a chord.
	both := map[string]string{"^H": "ctrl_h", "Backspace": "erase"}
	if got := bind(both, "^H"); got != "ctrl_h" {
		t.Errorf("^H -> %q, want its own binding, not the fallback's", got)
	}
	if got := bind(both, "Backspace"); got != "erase" {
		t.Errorf("Backspace -> %q, want its own binding", got)
	}
	bothSeq := map[string]string{"^K ^H": "seq_ctrl", "^K Backspace": "seq_named"}
	if got := bind(bothSeq, "^K", "^H"); got != "seq_ctrl" {
		t.Errorf("^K ^H -> %q, want its own binding inside a chord", got)
	}
	if got := bind(bothSeq, "^K", "Backspace"); got != "seq_named" {
		t.Errorf("^K Backspace -> %q, want its own binding inside a chord", got)
	}

	// Only one named: the other falls back to it, in both directions.
	if got := bind(map[string]string{"Backspace": "erase"}, "^H"); got != "erase" {
		t.Errorf("^H with only Backspace bound -> %q, want the fallback", got)
	}
	if got := bind(map[string]string{"^H": "ctrl_h"}, "Backspace"); got != "ctrl_h" {
		t.Errorf("Backspace with only ^H bound -> %q, want the fallback", got)
	}
}
