package keyseq

import "testing"

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
