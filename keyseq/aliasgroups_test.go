package keyseq

import (
	"strings"
	"testing"
	"unicode"
)

// The defaults speak direct-key-handler's vocabulary, so a binding written
// against a control character reaches the named key it arrives as, and the
// other way round.
func TestDefaultAliasGroupsUseHandlerVocabulary(t *testing.T) {
	cases := []struct{ bound, pressed string }{
		{"^I", "Tab"},
		{"Tab", "^I"},
		{"^M", "Return"},
		{"Return", "^M"},
		{"^[", "Escape"},
		{"Escape", "^["},
		{"^H", "Backspace"},
		{"Backspace", "^H"},
	}
	for _, c := range cases {
		sp := NewProcessor(nil)
		sp.SetMappings(map[string]string{c.bound: "target"})
		if got := sp.ProcessKey(c.pressed).Command; got != "target" {
			t.Errorf("bound %q, pressed %q -> %q, want target", c.bound, c.pressed, got)
		}
	}
}

// Ctrl+Space is the case where a default group has to speak the vocabulary
// exactly, because the name carries a modifier in front of it.
//
// NUL is what Ctrl+@ and Ctrl+Space both send, so the byte wire cannot tell
// them apart and the three spellings name one input. A binding is written
// "^Space" — the modifier on this package's name for the key — while the byte
// arrives as "^@", and the group is what joins them.
//
// It was spelled "^space", which joined nothing anyone would write: the
// lowercase name is not in this vocabulary. The miss was silent, because a
// binding that matches nothing simply never fires.
func TestDefaultAliasGroupsSpellCtrlSpaceInVocabulary(t *testing.T) {
	for _, c := range []struct{ bound, pressed string }{
		{"^Space", "^@"}, // written by name, arrives as the byte
		{"^Space", "^2"},
		{"^@", "^Space"}, // and every direction among the three
		{"^2", "^Space"},
		{"^@", "^2"},
	} {
		sp := NewProcessor(nil)
		sp.SetMappings(map[string]string{c.bound: "target"})
		if got := sp.ProcessKey(c.pressed).Command; got != "target" {
			t.Errorf("bound %q, pressed %q -> %q, want target", c.bound, c.pressed, got)
		}
	}

	// The modifier is part of the identity: the bare spacebar is another key.
	sp := NewProcessor(nil)
	sp.SetMappings(map[string]string{"Space": "bare", "^Space": "ctrl"})
	if got := sp.ProcessKey("Space").Command; got != "bare" {
		t.Errorf("Space -> %q, want bare", got)
	}
	sp.ClearActiveSequence()
	if got := sp.ProcessKey("^@").Command; got != "ctrl" {
		t.Errorf("^@ -> %q, want ctrl", got)
	}
}

// Every default group is written in this package's own vocabulary, so no
// member may be a name the vocabulary never produces. Lowercase is the tell:
// key names here are capitalised, so a lowercase base is a spelling nothing
// emits and nothing would think to bind.
func TestDefaultAliasGroupsAreAllInVocabulary(t *testing.T) {
	for _, g := range DefaultAliasGroups() {
		for _, name := range g {
			base := name
			for _, p := range []string{"^", "C-", "M-", "m-", "S-", "s-", "H-", "G-"} {
				base = strings.TrimPrefix(base, p)
			}
			if base == "" || !unicode.IsLetter(rune(base[0])) {
				continue // a control or punctuation spelling, not a key name
			}
			if strings.ToLower(base) == base {
				t.Errorf("group %v names %q, whose base %q is lowercase; key "+
					"names in this vocabulary are capitalised, so nothing emits it",
					g, name, base)
			}
		}
	}
}

// Return and Enter are two physical keys and are NOT aliased by default:
// folding them is an application's choice, not a default that quietly
// discards the distinction.
func TestDefaultAliasGroupsKeepReturnAndEnterApart(t *testing.T) {
	sp := NewProcessor(nil)
	sp.SetMappings(map[string]string{"Return": "submit"})
	if got := sp.ProcessKey("Enter").Command; got == "submit" {
		t.Error("the keypad's Enter matched a Return binding; the two must stay distinct by default")
	}
}

// An application with its own key names supplies its own groups.
func TestSetAliasGroups(t *testing.T) {
	sp := NewProcessor(nil)
	sp.SetAliasGroups([]AliasGroup{
		{"esc", "escape", "^["},
		{"back", "^H", "backspace"},
	})
	sp.SetMappings(map[string]string{"esc": "cancel", "back": "erase"})

	for _, c := range []struct{ pressed, want string }{
		{"esc", "cancel"},
		{"escape", "cancel"},
		{"^[", "cancel"},
		{"back", "erase"},
		{"^H", "erase"},
		{"backspace", "erase"},
	} {
		if got := sp.ProcessKey(c.pressed).Command; got != c.want {
			t.Errorf("%q -> %q, want %q", c.pressed, got, c.want)
		}
	}

	// The replaced defaults no longer apply.
	sp.SetMappings(map[string]string{"Tab": "indent"})
	if got := sp.ProcessKey("^I").Command; got == "indent" {
		t.Error("a default alias survived SetAliasGroups")
	}
}

// nil drops aliasing entirely, for an application whose names carry no such
// ambiguity and wants no fallbacks invented for it.
func TestSetAliasGroupsNilDisablesAliasing(t *testing.T) {
	sp := NewProcessor(nil)
	sp.SetAliasGroups(nil)
	sp.SetMappings(map[string]string{"Tab": "indent"})
	if got := sp.ProcessKey("^I").Command; got == "indent" {
		t.Errorf("^I still aliased to Tab after aliasing was dropped (got %q)", got)
	}
	if got := sp.ProcessKey("Tab").Command; got != "indent" {
		t.Errorf("the binding itself broke: Tab -> %q, want indent", got)
	}
}
