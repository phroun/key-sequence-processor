package keyseq

import (
	"reflect"
	"testing"
)

// The Option layer is a user setting, never a property of the machine the
// process happens to run on: someone typing on a Mac keyboard through an SSH
// session to a Linux box wants it, and the far end cannot see their keyboard.
// Nothing here consults the host OS.
func TestMacOptionCharIsSettingNotPlatform(t *testing.T) {
	sp := NewProcessor(nil)

	if _, ok := sp.MacOptionChar("M-d"); ok {
		t.Error("the Option layer applied while disabled; it must be off until asked for")
	}

	sp.SetMacOptionInsert(true)
	if ch, ok := sp.MacOptionChar("M-d"); !ok || ch != "∂" {
		t.Errorf("M-d -> %q (%v), want ∂ — enabling the layer must work on any platform", ch, ok)
	}
	if ch, ok := sp.MacOptionChar("M-e"); !ok || ch != "´" {
		t.Errorf("M-e -> %q (%v), want the acute accent dead key", ch, ok)
	}

	sp.SetMacOptionInsert(false)
	if _, ok := sp.MacOptionChar("M-d"); ok {
		t.Error("the layer stayed on after being switched off")
	}
}

// A key Option does not compose is not the layer's business, even when it is on.
func TestMacOptionCharIgnoresOtherKeys(t *testing.T) {
	sp := NewProcessor(nil)
	sp.SetMacOptionInsert(true)
	for _, key := range []string{"q", "^K", "pgup", "M-F1", "G-€"} {
		if ch, ok := sp.MacOptionChar(key); ok {
			t.Errorf("%q claimed by the Option layer as %q", key, ch)
		}
	}
}

// A binding on a Mega key beats the Option layer: the whole point is that
// bindings take the combos they name while everything else still types.
func TestBindingBeatsMacOptionLayer(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{"M-d": "delete_word"})
	sp.SetMacOptionInsert(true)
	sp.ProcessKey("M-d")
	if want := []string{"M-d→delete_word"}; !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — a binding must outrank the Option character", h.calls, want)
	}
}

// And an UNBOUND Mega key reaches the default handler, which asks the layer
// what it composes and spells the command in its own vocabulary.
func TestUnboundMegaKeyTypesItsOptionCharacter(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{})
	sp.SetMacOptionInsert(true)
	sp.SetDefaultHandler(func(key string) string {
		if ch, ok := sp.MacOptionChar(key); ok {
			return "insert '" + ch + "'"
		}
		return ""
	})
	sp.ProcessKey("M-d")
	if want := []string{"M-d→insert '∂'"}; !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — an unbound Option combo must still type", h.calls, want)
	}
}

// What the host WATCHED its keyboard type outranks every derivation.
//
// A table is one keyboard written down and a name is a guess that a key types
// what it is called; an observation is the machine being typed on. Where they
// disagree the observation is right, and where there is none the derivations
// still answer.
func TestWhatAnUnboundKeyTypes(t *testing.T) {
	p := NewProcessor(nil)
	p.SetMacOptionInsert(true)

	// With nothing watching, each derivation answers in turn.
	for _, c := range []struct {
		key, want, what string
	}{
		{"a", "a", "a one-character name is its character"},
		{"G-€", "€", "a Glyph token carries its glyph"},
		{"M-a", "å", "and the Option layer speaks for a Mega chord"},
	} {
		if text, known := p.TextForKey(c.key); !known || text != c.want {
			t.Errorf("%s: %s = %q known=%v, want %q", c.what, c.key, text, known, c.want)
		}
	}
	if _, known := p.TextForKey("F1"); known {
		t.Error("a key nothing can answer for came back known")
	}

	p.SetKeyChordText(func(chord string) (string, bool) {
		switch chord {
		case "M-a":
			return "ä", true // this keyboard disagrees with the table
		case "a":
			return "ä", true // and with the name
		case "M-i":
			return "", true // a dead key: it types NOTHING
		}
		return "", false
	})

	for _, c := range []struct{ key, want string }{
		{"M-a", "ä"},
		{"a", "ä"},
	} {
		if text, known := p.TextForKey(c.key); !known || text != c.want {
			t.Errorf("%s = %q known=%v, want the observed %q", c.key, text, known, c.want)
		}
	}
	// Observed and empty is an ANSWER, and not the same as not knowing. Read as
	// "no answer", the table below would be asked and would insert the accent
	// this key only armed.
	if text, known := p.TextForKey("M-i"); !known || text != "" {
		t.Errorf("M-i = %q known=%v, want a known nothing", text, known)
	}
	// Unobserved keys still fall to the derivations.
	if text, known := p.TextForKey("M-x"); !known || text != "≈" {
		t.Errorf("M-x = %q known=%v, want the table's ≈", text, known)
	}

	// The switch governs the TABLE, which is the guess. It is no reason to
	// refuse an answer arrived at by looking.
	p.SetMacOptionInsert(false)
	if ch, ok := p.MacOptionChar("M-a"); ok {
		t.Errorf("the layer answered %q with its switch off", ch)
	}
	if text, known := p.TextForKey("M-a"); !known || text != "ä" {
		t.Errorf("M-a = %q known=%v with the layer off, want the observation", text, known)
	}
	if _, known := p.TextForKey("M-x"); known {
		t.Error("an unobserved Mega chord answered with the layer off")
	}
}
