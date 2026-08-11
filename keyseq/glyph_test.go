package keyseq

import (
	"reflect"
	"testing"
)

// A key the application's default handler claims is dispatched through it — a
// Glyph chord (the AltGr/Level3 modifier, prefix "G-") here, so an
// international user's AltGr keystrokes still do something when unbound.
func TestUnboundKeyReachesDefaultHandler(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{})
	sp.ProcessKey("G-€")
	want := []string{"G-€→insert '€'"}
	if !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — an unbound key must reach the default handler", h.calls, want)
	}
}

// A binding outranks the default handler, so a user can always take a key back
// from whatever the application does with it by default.
func TestBindingOutranksDefaultHandler(t *testing.T) {
	sp, h := newCaptureSP(map[string]string{
		"G-€": "insert 'EUR'",
	})
	sp.ProcessKey("G-€")
	want := []string{"G-€→insert 'EUR'"}
	if !reflect.DeepEqual(h.calls, want) {
		t.Errorf("dispatched %v, want %v — a binding must outrank the default handler", h.calls, want)
	}
}

// With no handler installed the processor invents no policy of its own: an
// unbound key resolves to no command at all.
//
// Handled stays true, which reads oddly for a key nothing acted on: the
// processor reports that it CONSUMED the event, not that something came of it.
// That is the behavior the resolution rules have always had and applications
// depend on, so extracting the policy left it alone.
func TestNoDefaultHandlerYieldsNoCommand(t *testing.T) {
	sp := NewProcessor(nil)
	if r := sp.ProcessKey("q"); r.Command != "" {
		t.Errorf("unbound key with no handler: Command = %q, want empty", r.Command)
	}
}

// An executing processor with no handler simply dispatches nothing.
func TestNoDefaultHandlerDispatchesNothing(t *testing.T) {
	h := &captureHarness{decline: map[string]bool{}}
	sp := NewProcessor(h.exec)
	sp.ProcessKey("q")
	if len(h.calls) != 0 {
		t.Errorf("dispatched %v, want nothing without a default handler", h.calls)
	}
}
