package keyseq

import (
	"reflect"
	"testing"
)

// Order is not meaning. A keymap that writes "S-C-Up" names the same key as one
// that writes "C-S-Up", and either spelling of the press finds either spelling
// of the binding. Before this, matching compared whole tokens, so the two were
// unrelated strings — and the input layers disagreed about the order they
// composed, which is exactly the disagreement a keymap must not inherit.
func TestModifierOrderDoesNotMatter(t *testing.T) {
	orders := []string{"C-S-Up", "S-C-Up"}
	for _, bound := range orders {
		for _, pressed := range orders {
			if got := bindAndPress(t, nil, bound, pressed); got != "TARGET" {
				t.Errorf("bound %q, pressed %q -> %q, want TARGET", bound, pressed, got)
			}
		}
	}

	// Three deep, every ordering.
	three := []string{"C-M-S-Left", "M-S-C-Left", "S-C-M-Left", "M-C-S-Left"}
	for _, bound := range three {
		for _, pressed := range three {
			if got := bindAndPress(t, nil, bound, pressed); got != "TARGET" {
				t.Errorf("bound %q, pressed %q -> %q, want TARGET", bound, pressed, got)
			}
		}
	}

	// Through a sequence, and combined with the word spellings.
	if got := bindAndPress(t, nil, "^K S-C-Minus", "^K", "C-S--"); got != "TARGET" {
		t.Errorf("^K S-C-Minus <- ^K C-S-- -> %q, want TARGET", got)
	}
}

// The canonical order follows the order macOS renders modifiers (⌃⌥⇧⌘),
// extended with the ones a Mac keyboard has no cap for. Control's caret form
// sorts last so it lands against the base key.
func TestCanonicalOrder(t *testing.T) {
	got := canonicalizeStack([]string{"^", "H-", "s-", "S-", "m-", "M-", "G-", "C-"})
	want := []string{"C-", "G-", "M-", "m-", "S-", "s-", "H-", "^"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canonicalizeStack = %v, want %v", got, want)
	}
}

// There is no A- modifier. The key a PC puts under the Alt cap is Mega, spelled
// M-. An "A-" written in a keymap is not a modifier at all, so it stays part of
// the key name rather than being silently accepted as one.
func TestNoAModifier(t *testing.T) {
	if _, ok := modifierRank["A-"]; ok {
		t.Error("A- has a canonical rank; it is not a modifier in this vocabulary")
	}
	if got := bindAndPress(t, nil, "M-x", "A-x"); got == "TARGET" {
		t.Error("A-x reached an M-x binding; A- is not a spelling of Mega")
	}
}

// Mega and Micro are two DIFFERENT modifiers that fall back to each other: a
// terminal reports them on separate bits, and most keyboards only have the
// first. This is the ^H/Backspace shape, not the ^/C- shape — bind one and
// either reaches it, bind both and they stay apart.
func TestMegaAndMicroFallBack(t *testing.T) {
	// Either spelling reaches a binding that names only the other.
	for _, c := range []struct{ bound, pressed string }{
		{"M-x", "m-x"},
		{"m-x", "M-x"},
		{"M-S-Up", "m-S-Up"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}

	// Named separately, they keep their own meanings — nothing is discarded by
	// declaring them reachable.
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"M-x": "mega", "m-x": "micro"})
	if got := p.ProcessKey("M-x").Command; got != "mega" {
		t.Errorf("M-x -> %q, want its own binding", got)
	}
	p.ClearActiveSequence()
	if got := p.ProcessKey("m-x").Command; got != "micro" {
		t.Errorf("m-x -> %q, want its own binding", got)
	}
}

// Hyper is a real modifier — one of the four a Space Cadet keyboard had its own
// key for — and it stacks and sorts like any other.
func TestHyperIsABindableModifier(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"H-x", "H-x"},
		{"s-H-Up", "H-s-Up"},
		{"C-H-Minus", "H-C--"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}
	// Hyper is not Super: they are separate keys and neither falls back.
	if got := bindAndPress(t, nil, "H-x", "s-x"); got == "TARGET" {
		t.Error("Super matched a Hyper binding")
	}
}

// The caret keeps reaching C- through a whole stack, in either order.
func TestControlSpellingThroughStacks(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"M-^X", "C-M-X"},
		{"C-M-X", "M-^X"},
		{"M-S-^X", "C-M-S-X"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}
}
