package keyseq

import "testing"

// A letter's case is not part of which key it is, and a modifier prefix does
// not make it so: "s-M" and "s-m" name one keystroke, exactly as "M" and "m"
// do. Whichever spelling the keymap used, the other one reaches it.
func TestCaseFlipReachesThroughModifiers(t *testing.T) {
	cases := []struct{ bound, pressed string }{
		{"s-M", "s-m"},
		{"s-m", "s-M"},
		{"M-a", "M-A"},
		{"M-A", "M-a"},
		{"C-S-z", "C-S-Z"},
		{"H-q", "H-Q"},
		// The two spellings of Control meet here too: the caret form writes
		// its letter uppercase, so a lowercase C- form needs the flip AND the
		// prefix alias applied together.
		{"^Q", "C-q"},
		{"C-q", "^Q"},
		{"^Q", "^q"},
	}
	for _, c := range cases {
		p := NewProcessor(nil)
		p.SetMappings(map[string]string{c.bound: "act"})
		if got := p.ProcessKey(c.pressed).Command; got != "act" {
			t.Errorf("bound %q, pressed %q -> %q, want \"act\"", c.bound, c.pressed, got)
		}
	}
}

// Binding BOTH spellings is what makes them differ, and no special rule is
// needed for it: the sequence as pressed is tried before any alias.
func TestBothCasesBoundStayDistinct(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{
		"s-M": "upper",
		"s-m": "lower",
	})
	if got := p.ProcessKey("s-M").Command; got != "upper" {
		t.Errorf("s-M -> %q, want \"upper\"", got)
	}
	if got := p.ProcessKey("s-m").Command; got != "lower" {
		t.Errorf("s-m -> %q, want \"lower\"", got)
	}
}

// Only LETTERS have a case. A named key is a name, not a letter, so nothing
// invents an upper-cased spelling of it.
func TestNamedKeysAreNotCaseFlipped(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"M-Enter": "act"})
	if got := p.ProcessKey("M-ENTER").Command; got != "" {
		t.Errorf("M-ENTER -> %q, want no match", got)
	}
}

// A continuation key inside a chord gets the same treatment.
func TestCaseFlipThroughModifiersMidSequence(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"^X s-M": "act"})
	if got := p.ProcessKey("^X").Command; got != "" {
		t.Fatalf("^X should hold the prefix, got %q", got)
	}
	if got := p.ProcessKey("s-m").Command; got != "act" {
		t.Errorf("^X s-m -> %q, want \"act\"", got)
	}
}
