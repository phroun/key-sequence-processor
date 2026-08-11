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

// A binding on a Meta key beats the Option layer: the whole point is that
// bindings take the combos they name while everything else still types.
func TestBindingBeatsMacOptionLayer(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{"M-d": "delete_word"})
	sp.SetMacOptionInsert(true)
	sp.ProcessKey("M-d")
	if want := []string{"M-d→delete_word"}; !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — a binding must outrank the Option character", h.calls, want)
	}
}

// And an UNBOUND Meta key reaches the default handler, which asks the layer
// what it composes and spells the command in its own vocabulary.
func TestUnboundMetaKeyTypesItsOptionCharacter(t *testing.T) {
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
