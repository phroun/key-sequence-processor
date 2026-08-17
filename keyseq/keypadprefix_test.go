package keyseq

import "testing"

// "P-" and "p-" are modifiers, so a keymap can write them at all.
//
// Before this they were not in the vocabulary, so "P-Home" split as no
// modifiers and the base name "P-Home" — a literal string that only ever
// matched itself. None of the machinery that makes a chord findable reached
// it: not the ordering, not the group fallbacks, not the word spellings.
func TestKeypadPrefixesAreModifiers(t *testing.T) {
	for _, c := range []struct{ prefix, base, token string }{
		{"P-", "Home", "P-Home"},
		{"p-", ",", "p-,"},
		{"C-P-", "Home", "C-P-Home"},
		{"P-^", "7", "P-^7"},
		{"S-P-^", "7", "S-P-^7"},
	} {
		prefix, base := splitModifiers(c.token)
		if prefix != c.prefix || base != c.base {
			t.Errorf("splitModifiers(%q) = %q + %q, want %q + %q",
				c.token, prefix, base, c.prefix, c.base)
		}
	}
}

// Order is not meaning here either: the pad prefix sorts to one place, so a
// keymap that writes "P-C-Home" names the same key as one that writes
// "C-P-Home", and neither has to know which order the input layer composed.
func TestKeypadPrefixOrderDoesNotMatter(t *testing.T) {
	orders := []string{"C-P-Home", "P-C-Home"}
	for _, bound := range orders {
		for _, pressed := range orders {
			if got := bindAndPress(t, nil, bound, pressed); got != "TARGET" {
				t.Errorf("bound %q, pressed %q -> %q, want TARGET", bound, pressed, got)
			}
		}
	}
	// And with the caret, which sorts behind the pad to land on the base key.
	for _, pressed := range []string{"P-^7", "^P-7"} {
		if got := bindAndPress(t, nil, "P-^7", pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", "P-^7", pressed, got)
		}
	}
}

// The two pad prefixes reach each other, the way Mega and Micro do. The
// lowercase form exists only because HID defines a handful of pad keys twice,
// and no keymap should have to know which of the two duplicates its terminal
// picked.
func TestKeypadPrefixesFallBackToEachOther(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"P-,", "p-,"},
		{"p-,", "P-,"},
		{"P-=", "p-="},
		{"C-P-Home", "C-p-Home"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}

	// Bound separately they stay apart, since as-pressed outranks any fallback.
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"P-,": "upper", "p-,": "lower"})
	if got := p.ProcessKey("P-,").Command; got != "upper" {
		t.Errorf("P-, -> %q, want \"upper\"", got)
	}
	if got := p.ProcessKey("p-,").Command; got != "lower" {
		t.Errorf("p-, -> %q, want \"lower\"", got)
	}
}

// A keypad key falls back to the plain binding for the action it duplicates.
//
// The pad's Home and the main cluster's are one action struck in two places, so
// a keymap that binds "Home" means both — and a keymap written before the pad
// was ever surfaced keeps working the day a pad key first arrives, which is
// every keymap in existence at the moment this prefix was added.
func TestAKeypadKeyReachesThePlainBinding(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"Home", "P-Home"},
		{"Home", "p-Home"},
		{"C-Home", "C-P-Home"},
		{"Enter", "P-Enter"},
		{"7", "P-7"},
		{"^7", "P-^7"},
		{",", "p-,"},
		// And the drop composes with the other fallbacks rather than
		// competing with them: "^" reaches a bound "C-" through the pad.
		{"C-7", "P-^7"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}
}

// The drop goes one way only. A keymap that has bound the pad key on purpose
// has said the two are different, and the plain key must not be swept into it
// — that would make the prefix worse than useless, since surfacing the keypad
// would then STEAL keys from the main cluster.
func TestThePlainKeyDoesNotReachAKeypadBinding(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"P-Home", "Home"},
		{"p-,", ","},
		{"P-^7", "^7"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got == "TARGET" {
			t.Errorf("pressed %q reached a binding written %q; the plain key was "+
				"taken over by the keypad's", c.pressed, c.bound)
		}
	}
}

// Binding either pad form takes the pad key back out of the plain binding's
// reach, because the sequence as pressed and its sibling spellings are both
// tried before anything is dropped. That is what "falls back when neither is
// defined" has to mean to be worth anything.
func TestAPadBindingWinsOverThePlainOne(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"Home": "plain", "P-Home": "pad"})
	if got := p.ProcessKey("P-Home").Command; got != "pad" {
		t.Errorf("P-Home -> %q, want \"pad\"", got)
	}
	if got := p.ProcessKey("Home").Command; got != "plain" {
		t.Errorf("Home -> %q, want \"plain\"", got)
	}

	// The lowercase twin counts as defined for this purpose: it is the same
	// key by fallback, so it is preferred over dropping the pad entirely.
	q := NewProcessor(nil)
	q.SetMappings(map[string]string{",": "plain", "p-,": "pad"})
	if got := q.ProcessKey("P-,").Command; got != "pad" {
		t.Errorf("P-, -> %q, want \"pad\" (via p-,), got the plain binding", got)
	}
}

// Only the pad prefixes drop. Every other modifier says what was HELD, and a
// held modifier is part of which chord was struck — quietly ignoring one would
// send Ctrl-Home to a binding written for Home.
func TestOtherModifiersDoNotDrop(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"Home", "C-Home"},
		{"Home", "S-Home"},
		{"Home", "M-Home"},
		{"Home", "s-Home"},
		{"7", "^7"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got == "TARGET" {
			t.Errorf("pressed %q reached a binding written %q; a held modifier "+
				"was discarded", c.pressed, c.bound)
		}
	}
}

// The pad prefix survives a sequence, at any position.
func TestKeypadPrefixInASequence(t *testing.T) {
	if got := bindAndPress(t, nil, "^K Home", "^K", "P-Home"); got != "TARGET" {
		t.Errorf("^K Home <- ^K P-Home -> %q, want TARGET", got)
	}
	if got := bindAndPress(t, nil, "^K P-,", "^K", "p-,"); got != "TARGET" {
		t.Errorf("^K P-, <- ^K p-, -> %q, want TARGET", got)
	}
}

// A word spelling reaches through the pad prefix, as it does through every
// other one — the reason the spellings exist is that "-" is the separator, and
// "P--" is exactly the shape that makes trouble.
func TestWordSpellingsReachThroughThePadPrefix(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"P-Minus", "P--"},
		{"P--", "P-Minus"},
		{"Minus", "P--"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}
}
