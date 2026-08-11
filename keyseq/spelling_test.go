package keyseq

import "testing"

func bindAndPress(t *testing.T, groups []AliasGroup, bound string, keys ...string) string {
	t.Helper()
	p := NewProcessor(nil)
	if groups != nil {
		p.SetAliasGroups(groups)
	}
	p.SetMappings(map[string]string{bound: "TARGET"})
	var last string
	for _, k := range keys {
		last = p.ProcessKey(k).Command
	}
	return last
}

// A word spelling names a key that has exactly one real token: nothing ever
// emits "Minus". It exists so a binding can be written without fighting the
// syntax it is written in, which is why the defaults carry them at all.
func TestDefaultSpellingsResolveBareKeys(t *testing.T) {
	cases := []struct{ word, symbol string }{
		{"Minus", "-"}, {"Plus", "+"}, {"Equals", "="},
		{"Apos", "'"}, {"Quote", "\""}, {"Tilde", "~"}, {"Wave", "~"},
		{"Backtick", "`"}, {"Backslash", "\\"}, {"Slash", "/"},
		{"Semicolon", ";"}, {"Colon", ":"}, {"Pipe", "|"},
	}
	for _, c := range cases {
		if got := bindAndPress(t, nil, c.word, c.symbol); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.word, c.symbol, got)
		}
		if got := bindAndPress(t, nil, c.symbol, c.word); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.symbol, c.word, got)
		}
	}
}

// The case the words were invented for: `-` is the modifier separator, so `M--`
// is exactly where writing the symbol hurts. A spelling that only worked on a
// bare key would miss it, since alias groups match whole tokens and `M-Minus`
// and `M--` are two unrelated strings until the modifiers are peeled off.
func TestSpellingsResolveInsideModifierChords(t *testing.T) {
	cases := []struct{ word, symbol string }{
		{"M-Minus", "M--"},
		{"S-Equals", "S-="},
		{"C-Slash", "C-/"},
		{"s-Plus", "s-+"},
		{"G-Backslash", "G-\\"},
		{"M-C-Minus", "M-C--"}, // stacked modifiers peel all the way down
	}
	for _, c := range cases {
		if got := bindAndPress(t, nil, c.word, c.symbol); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.word, c.symbol, got)
		}
		if got := bindAndPress(t, nil, c.symbol, c.word); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.symbol, c.word, got)
		}
	}
}

// ...and inside a chord, in any slot, including as the starter.
func TestSpellingsResolveThroughSequences(t *testing.T) {
	for _, c := range []struct {
		bound string
		keys  []string
	}{
		{"^K Minus", []string{"^K", "-"}},
		{"^K M-Minus", []string{"^K", "M--"}},
		{"M-Minus x", []string{"M--", "x"}},
		{"^K Minus Minus", []string{"^K", "-", "-"}},
		{"^K Equals Minus", []string{"^K", "=", "-"}},
	} {
		if got := bindAndPress(t, nil, c.bound, c.keys...); got != "TARGET" {
			t.Errorf("bound %q, pressed %v -> %q, want TARGET", c.bound, c.keys, got)
		}
	}
}

// Peeling modifiers must not invent a base out of a token that is only
// modifiers, nor treat a bare symbol as prefixed.
func TestSplitModifiers(t *testing.T) {
	for _, c := range []struct{ in, prefix, base string }{
		{"M--", "M-", "-"},
		{"M-Minus", "M-", "Minus"},
		{"M-C--", "M-C-", "-"},
		{"-", "", "-"},
		{"M-", "", "M-"}, // nothing but a modifier: left whole, base never empty
		{"^K", "^", "K"},
		{"^^", "^", "^"}, // Ctrl-caret: the last caret is the base, not a prefix
		{"C-K", "C-", "K"},
		{"Tab", "", "Tab"},
	} {
		prefix, base := splitModifiers(c.in)
		if prefix != c.prefix || base != c.base {
			t.Errorf("splitModifiers(%q) = (%q, %q), want (%q, %q)", c.in, prefix, base, c.prefix, c.base)
		}
	}
}

// The caret and "C-" are one modifier, so all four ways of writing Ctrl-minus
// reach one binding, and the caret composes with the word spellings — "^Minus"
// is a legal way to write "^-", which is the edge the caret's missing separator
// would otherwise make unwritable.
func TestControlPrefixSpellings(t *testing.T) {
	spellings := []string{"^-", "^Minus", "C--", "C-Minus"}
	for _, bound := range spellings {
		for _, pressed := range spellings {
			if got := bindAndPress(t, nil, bound, pressed); got != "TARGET" {
				t.Errorf("bound %q, pressed %q -> %q, want TARGET", bound, pressed, got)
			}
		}
	}
	// And on named keys, where only the modifier varies.
	for _, c := range []struct{ bound, pressed string }{
		{"^K", "C-K"}, {"C-K", "^K"},
		{"^Backslash", "^\\"}, {"^\\", "C-Backslash"},
		{"M-^K", "M-C-K"},
	} {
		if got := bindAndPress(t, nil, c.bound, c.pressed); got != "TARGET" {
			t.Errorf("bound %q, pressed %q -> %q, want TARGET", c.bound, c.pressed, got)
		}
	}
	// A chord opened with the letter form is still a control chord, so the
	// ladder that lets "^B M" complete on "^B ^M" applies to "C-B M" too.
	if got := bindAndPress(t, nil, "C-B M", "C-B", "^M"); got != "TARGET" {
		t.Errorf("C-B M pressed as C-B ^M -> %q, want TARGET", got)
	}
}

// The caret is also an ordinary character, and stays mappable as one. Peeling
// modifiers must not eat the whole token, and a bare caret must not be read as
// Control-of-nothing: it names the key a user types, alongside "^^" for
// Ctrl-caret and "^C" for a real control chord.
func TestBareCaretIsAKey(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"^": "caret", "^^": "ctrl_caret", "^C": "ctrl_c"})
	for _, c := range []struct{ pressed, want string }{
		{"^", "caret"}, {"^^", "ctrl_caret"}, {"^C", "ctrl_c"},
	} {
		p.ClearActiveSequence()
		if got := p.ProcessKey(c.pressed).Command; got != c.want {
			t.Errorf("%q -> %q, want %q", c.pressed, got, c.want)
		}
	}

	// It works in every slot of a chord, and under a modifier.
	for _, c := range []struct {
		bound string
		keys  []string
	}{
		{"^ x", []string{"^", "x"}},
		{"x ^", []string{"x", "^"}},
		{"^K ^", []string{"^K", "^"}},
		{"M-^", []string{"M-^"}},
	} {
		if got := bindAndPress(t, nil, c.bound, c.keys...); got != "TARGET" {
			t.Errorf("bound %q, pressed %v -> %q, want TARGET", c.bound, c.keys, got)
		}
	}

	// ...and it does NOT open a control chord, so the control/case ladder stays
	// off for what follows it: a bound "^ M" is caret-then-M and nothing else.
	for _, c := range []struct {
		bound string
		keys  []string
	}{
		{"^ M", []string{"^", "^M"}},
		{"^ m", []string{"^", "^M"}},
	} {
		if got := bindAndPress(t, nil, c.bound, c.keys...); got == "TARGET" {
			t.Errorf("bound %q matched %v; a bare caret is a character, not Control", c.bound, c.keys)
		}
	}
}

// The first slot of a chord varies exactly as the others do. It used to be
// registered from the bound spelling alone, so a chord bound on a letter could
// only be opened in that case: the prefix was never held and the chord never
// began, however the rest was typed.
func TestStarterCaseFlips(t *testing.T) {
	for _, c := range []struct {
		bound string
		keys  []string
	}{
		{"M x", []string{"m", "x"}},
		{"m x", []string{"M", "x"}},
		{"M x", []string{"m", "X"}}, // both slots at once
		{"M N o", []string{"m", "n", "O"}},
	} {
		if got := bindAndPress(t, nil, c.bound, c.keys...); got != "TARGET" {
			t.Errorf("bound %q, pressed %v -> %q, want TARGET", c.bound, c.keys, got)
		}
	}

	// Exact still wins when a keymap spells both cases.
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"M x": "upper", "m x": "lower"})
	p.ProcessKey("m")
	if got := p.ProcessKey("x").Command; got != "lower" {
		t.Errorf("pressed the lowercase starter, got %q, want its own binding", got)
	}
	p.ClearActiveSequence()
	p.ProcessKey("M")
	if got := p.ProcessKey("x").Command; got != "upper" {
		t.Errorf("pressed the uppercase starter, got %q, want its own binding", got)
	}

	// A case-flipped starter is not a control starter, so the ladder stays off:
	// "M ^M" still means a real Ctrl-M, opened in either case.
	if got := bindAndPress(t, nil, "M ^M", "m", "^M"); got != "TARGET" {
		t.Errorf("M ^M opened as m -> %q, want TARGET", got)
	}
	if got := bindAndPress(t, nil, "M ^M", "m", "M"); got == "TARGET" {
		t.Error("a letter starter invented a control form for its continuation")
	}
}

// A spelling is still a lookup, not a rewrite: naming the symbol directly wins
// over the word, and the control ladder is untouched by the modifier pass.
func TestSpellingsStayNonDestructive(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMappings(map[string]string{"^K -": "by_symbol", "^K Minus": "by_word"})
	p.ProcessKey("^K")
	if got := p.ProcessKey("-").Command; got != "by_symbol" {
		t.Errorf("pressed the symbol, got %q, want its own binding", got)
	}
	if got := bindAndPress(t, nil, "^B M", "^B", "^M"); got != "TARGET" {
		t.Errorf("the control ladder broke: ^B ^M -> %q", got)
	}
}
