package keyseq

import "testing"

func withOptionLayer() *Processor {
	p := NewProcessor(nil)
	p.SetMacOptionInsert(true)
	return p
}

// Option+i types nothing and arms a circumflex; the next letter takes it.
//
// A keyboard normally does this itself, and a host handed a finished "û" never
// sees the chord. Ghostty under the "kitty" protocol reports Option+i as a Mega
// chord that produced no text and then reports the following "u" as a plain
// "u", so the accent is lost between them and nothing composes anywhere.
func TestADeadKeyArmsAnAccentForTheNextKey(t *testing.T) {
	p := withOptionLayer()

	text, known := p.TextForKey("M-i")
	if !known || text != "" {
		t.Fatalf("Option+i -> (%q, %v), want it to type nothing and say so",
			text, known)
	}
	if accent, pending := p.DeadKeyPending(); !pending || accent != "ˆ" {
		t.Fatalf("armed %q (%v), want the circumflex", accent, pending)
	}

	if text, known := p.TextForKey("u"); !known || text != "û" {
		t.Errorf("u after Option+i -> (%q, %v), want û", text, known)
	}
	if _, pending := p.DeadKeyPending(); pending {
		t.Error("the accent is still armed after being taken")
	}
}

// All five, over a letter each actually takes.
func TestEveryDeadKeyComposes(t *testing.T) {
	for _, c := range []struct{ chord, then, want string }{
		{"M-e", "e", "é"},
		{"M-i", "o", "ô"},
		{"M-n", "n", "ñ"},
		{"M-u", "u", "ü"},
		{"M-`", "a", "à"},
		{"M-u", "O", "Ö"}, // and the capital on the same accent
	} {
		p := withOptionLayer()
		p.TextForKey(c.chord)
		if text, known := p.TextForKey(c.then); !known || text != c.want {
			t.Errorf("%s then %q -> (%q, %v), want %q",
				c.chord, c.then, text, known, c.want)
		}
	}
}

// A pair that does not compose leaves the accent standing and lets the key
// land after it, which is what macOS puts in a document for the same gesture.
func TestAnUncomposablePairLeavesTheAccentAndTypesOn(t *testing.T) {
	p := withOptionLayer()
	p.TextForKey("M-i")

	if text, known := p.TextForKey("z"); !known || text != "ˆz" {
		t.Errorf("z after Option+i -> (%q, %v), want the bare accent and the z",
			text, known)
	}
	if _, pending := p.DeadKeyPending(); pending {
		t.Error("the accent is still armed after being spent on a pair that " +
			"could not take it")
	}
}

// A key that TYPES NOTHING is no answer to a dead key: an arrow moves the
// caret and a binding runs, and the accent belongs to the next keystroke that
// types.
func TestAKeyThatTypesNothingLeavesTheAccentArmed(t *testing.T) {
	p := withOptionLayer()
	p.TextForKey("M-i")

	if text, known := p.TextForKey("Down"); known || text != "" {
		t.Errorf("Down after Option+i -> (%q, %v), want no answer", text, known)
	}
	if _, pending := p.DeadKeyPending(); !pending {
		t.Fatal("the accent was spent on a key that types nothing")
	}
	if text, _ := p.TextForKey("u"); text != "û" {
		t.Errorf("u after the arrow -> %q, want û", text)
	}
}

// And an application that knows the keyboard has gone elsewhere can drop it.
func TestResetDropsAPendingAccent(t *testing.T) {
	p := withOptionLayer()
	p.TextForKey("M-i")
	p.ResetDeadKey()

	if _, pending := p.DeadKeyPending(); pending {
		t.Fatal("the accent survived a reset")
	}
	if text, _ := p.TextForKey("u"); text != "u" {
		t.Errorf("u after the reset -> %q, want the plain letter", text)
	}
}

// With the layer OFF these five are chords for binding and nothing else.
// Arming an accent nobody is going to see would swallow the keystroke after.
func TestNoDeadKeysWithTheLayerOff(t *testing.T) {
	p := NewProcessor(nil)

	if text, known := p.TextForKey("M-i"); known || text != "" {
		t.Errorf("Option+i with the layer off -> (%q, %v), want no answer",
			text, known)
	}
	if text, _ := p.TextForKey("u"); text != "u" {
		t.Errorf("u -> %q, want the plain letter", text)
	}
}

// What the HOST watched still wins for ordinary keys, and the accent applies to
// what it saw.
func TestAnObservedChordStillTakesTheAccent(t *testing.T) {
	p := withOptionLayer()
	p.SetKeyChordText(func(chord string) (string, bool) {
		if chord == "G-x" {
			return "e", true
		}
		return "", false
	})
	p.TextForKey("M-`")

	if text, known := p.TextForKey("G-x"); !known || text != "è" {
		t.Errorf("an observed chord after Option+` -> (%q, %v), want è", text, known)
	}
}

// The three tables that name these five keys must agree. This one is the chord
// side; kittytk's SDL platform carries the composed-text side, and the Option
// table lists the character each types on its own.
func TestTheDeadKeysAreTheOnesTheOptionTableMarks(t *testing.T) {
	for chord, accent := range deadKeyAccents {
		if ch, ok := macOptionChars[chord]; !ok || ch != accent {
			t.Errorf("%s arms %q but the Option table says %q (present=%v)",
				chord, accent, ch, ok)
		}
	}
}

// A host that WATCHED its own keyboard was there for the composition too, so
// nothing is armed on its account.
//
// macOS composes Option+i then "u" into "û" and delivers the finished
// character; the chord it reports along the way types nothing and means nothing
// more. Arming there leaves an accent waiting for a keystroke already accounted
// for, and it attaches itself to whatever is typed next — kittytk's SDL
// platform is exactly this case, and it is the reason the order matters.
func TestNothingIsArmedWhereTheHostWatchedTheKeyboard(t *testing.T) {
	p := withOptionLayer()
	p.SetKeyChordText(func(chord string) (string, bool) {
		if chord == "M-i" {
			return "", true // watched, and it typed nothing
		}
		return "", false
	})

	if text, known := p.TextForKey("M-i"); !known || text != "" {
		t.Fatalf("the watched chord -> (%q, %v), want a known nothing", text, known)
	}
	if accent, pending := p.DeadKeyPending(); pending {
		t.Fatalf("armed %q where the keyboard's own machine had already "+
			"composed; the accent would land on the next thing typed", accent)
	}
	if text, _ := p.TextForKey("u"); text != "u" {
		t.Errorf("u -> %q, want the plain letter", text)
	}
}
