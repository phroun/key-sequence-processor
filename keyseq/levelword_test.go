package keyseq

import (
	"reflect"
	"testing"
)

// A level word is metadata about the binding, not a position in it, so it
// resolves the same wherever it is written.
func TestLevelWordsMaySitAnywhere(t *testing.T) {
	for _, raw := range []string{"(capture) ^C", "^C (capture)"} {
		sp, h := newCaptureSP(map[string]string{
			"^C": "base",
			raw:  "captured",
		})
		sp.ProcessKey("^C")
		if want := []string{"^C→captured"}; !reflect.DeepEqual(h.calls, want) {
			t.Errorf("%q dispatched %v, want %v", raw, h.calls, want)
		}
	}

	// ...including in the middle of a chord, which is the case a leading-word
	// notation could not express at all.
	sp, h := newCaptureSP(map[string]string{"^B (capture) N": "captured"})
	sp.ProcessKey("^B")
	sp.ProcessKey("N")
	if want := []string{"N→captured"}; !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — the chord is ^B N at level 1", h.calls, want)
	}
}

// A parenthesized word this reader does not know is SKIPPED, not read as a
// key. That is what lets an application layered over this one write its own
// metadata in the same parentheses: the key still resolves, and the word it
// did not understand cost it nothing.
func TestUnknownMetadataIsSkippedNotBound(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{"(mac) (capture) ^C": "captured"})
	sp.ProcessKey("^C")
	if want := []string{"^C→captured"}; !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — (mac) is not a key and not a level", h.calls, want)
	}
}

// A parenthesis is a key someone can press, so only a WHOLE "(word)" token is
// metadata. Getting this wrong would make a keymap unable to bind the key at
// all.
func TestParenthesesAreStillKeys(t *testing.T) {
	for _, tc := range []struct{ raw, press string }{
		{"(", "("},
		{")", ")"},
		{"()", "()"}, // an empty pair has no word in it
		{"^(", "^("}, // and a modified parenthesis is not a token of one
	} {
		sp, h := newCaptureSP(map[string]string{tc.raw: "punct"})
		sp.ProcessKey(tc.press)
		if want := []string{tc.press + "→punct"}; !reflect.DeepEqual(h.calls, want) {
			t.Errorf("%q dispatched %v, want %v", tc.raw, h.calls, want)
		}
	}
}

// The bare words carry no meaning any more: "capture" is the name of a key, and
// a keymap still written the old way binds a chord nobody can press rather
// than quietly raising a level. Stated here because it is the breaking half of
// the change, and a keymap that hits it deserves to be told what happened.
func TestBareWordsAreOrdinaryKeys(t *testing.T) {
	level, seq := parseRawKey("capture ^C")
	if level != 0 || seq != "capture ^C" {
		t.Errorf("parseRawKey(\"capture ^C\") = (%d, %q), want (0, \"capture ^C\")", level, seq)
	}
	if level, seq := parseRawKey("capture"); level != 0 || seq != "capture" {
		t.Errorf("parseRawKey(\"capture\") = (%d, %q), want (0, \"capture\")", level, seq)
	}
}

// Metadata with nothing left to press names no key. It must not become a
// binding on the empty string, which would claim every resolution that looked
// one up.
func TestMetadataAloneBindsNoKey(t *testing.T) {
	for _, raw := range []string{"(capture)", "(capture) (override)"} {
		if level, seq := parseRawKey(raw); level != 0 || seq != raw {
			t.Errorf("parseRawKey(%q) = (%d, %q), want (0, %q)", raw, level, seq, raw)
		}
	}

	sp := NewProcessor(nil)
	sp.SetDefaultHandler(testDefaultHandler)
	sp.SetMappings(map[string]string{"(capture)": "should_not_claim_anything"})
	if got := sp.ProcessKey("z").Command; got != "insert 'z'" {
		t.Errorf("Command = %q, want the default insert — an empty key claimed the press", got)
	}
}
