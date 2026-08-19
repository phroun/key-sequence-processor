package keyseq

import "strings"

// macOptionChars maps Mega key names back to the character macOS Option
// produces for that key (US layout).
//
// It exists because a terminal forces an either/or: the Option cap is Mega, or it
// types characters — not both. Whichever way the terminal is set, one half is
// normally lost. This table restores it from the binding side: an M- key no
// binding claimed types the character Option would have produced. So bindings
// take the combos they name and every other combo still types, and that holds
// on any platform — a Linux or Windows terminal gains mac-style Option typing
// it never had, and an SSH session from a Mac keeps it no matter what the
// remote machine runs.
//
// Deliberately not keyed off the host OS: the keyboard that matters belongs to
// whoever is typing, which a process on the far end of an SSH connection
// cannot see. Enable it with SetMacOptionInsert (see MacOptionChar).
//
// The inverse of this table — decoding the composed character back into an M-
// name so it can be bound at all — is the input decoder's job, and
// github.com/phroun/direct-key-handler carries it as macOSOptionChars. The two
// are the same 69 entries in opposite directions and must agree; they are kept
// apart because they serve different halves, and either is useful alone.

var macOptionChars = map[string]string{
	// Option+letter
	"M-a": "å", "M-b": "∫", "M-c": "ç", "M-d": "∂",
	"M-e": "´", // dead key: acute accent
	"M-f": "ƒ", "M-g": "©", "M-h": "˙",
	"M-i": "ˆ", // dead key: circumflex
	"M-j": "∆", "M-k": "˚", "M-l": "¬", "M-m": "µ",
	"M-n": "˜", // dead key: tilde
	"M-o": "ø", "M-p": "π", "M-q": "œ", "M-r": "®", "M-s": "ß",
	"M-t": "†",
	"M-u": "¨", // dead key: diaeresis
	"M-v": "√", "M-w": "∑", "M-x": "≈", "M-y": "¥", "M-z": "Ω",

	// Option+Shift+letter (E/I/N/U produce the same dead keys as lowercase,
	// so they have no distinct entries — mirroring the decode table).
	"M-A": "Å", "M-B": "ı", "M-C": "Ç", "M-D": "Î", "M-F": "Ï",
	"M-G": "˝", "M-H": "Ó", "M-J": "Ô",
	"M-K": "", // the Apple logo (private use area)
	"M-L": "Ò", "M-M": "Â", "M-O": "Ø", "M-P": "∏", "M-Q": "Œ",
	"M-R": "‰", "M-S": "Í", "M-T": "ˇ", "M-V": "◊", "M-W": "„",
	"M-X": "˛", "M-Y": "Á", "M-Z": "¸",

	// Option+number
	"M-1": "¡", "M-2": "™", "M-3": "£", "M-4": "¢", "M-5": "∞",
	"M-6": "§", "M-7": "¶", "M-8": "•", "M-9": "ª", "M-0": "º",

	// Option+symbol
	"M--": "–", "M-=": "≠", "M-[": "“", "M-]": "’",
	"M-\\": "«", "M-;": "…", "M-'": "æ", "M-,": "≤", "M-.": "≥",
	"M-/": "÷", "M-`": "`",
}

// SetMacOptionInsert enables the Option-character layer for unbound Mega keys
// (see macOptionChars). Off by default; the application decides, since the
// relevant keyboard is the user's, not the host's.
func (sp *Processor) SetMacOptionInsert(enabled bool) {
	sp.macOptionInsert = enabled
}

// SetKeyChordText installs a lookup of what the HOST watched its own keyboard
// TYPE for a chord, which TextForKey consults ahead of everything it would
// otherwise derive.
//
// The table above is one keyboard written down — the US layout, from memory. A
// host that receives the text alongside the keystroke knows what this machine
// actually produced, which is better evidence than any table, and the only
// evidence that is right when the two disagree. Pass nil to go back to the
// derivations alone.
//
// The lookup's second result means OBSERVED, not non-empty. A host that watched
// a key type nothing says so by answering "" and true, and that is a different
// answer from never having seen the key.
//
// Install it whatever the Option layer's switch says. That switch governs the
// table, which is a guess; it is no reason to withhold what was seen.
func (sp *Processor) SetKeyChordText(lookup func(chord string) (string, bool)) {
	sp.keyChordText = lookup
}

// MacOptionChar returns the character macOS Option produces for a Mega key
// name, and whether the layer applies to it: false when the layer is off, or
// when the key is not one Option composes.
//
// This is the LAYER's answer alone — the table above and its switch. What an
// unbound key types is a wider question, and TextForKey answers that one.
func (sp *Processor) MacOptionChar(key string) (string, bool) {
	if !sp.macOptionInsert {
		return "", false
	}
	ch, ok := macOptionChars[key]
	return ch, ok
}

// TextForKey returns the text an unbound key types, and whether that is known.
//
// known with EMPTY text is an answer, and a different one from not knowing: the
// key types nothing. A macOS dead key is that — Option+i arms a circumflex for
// the next keystroke and produces no character of its own — and an application
// that reads the empty string as "no answer" goes on to guess, and inserts an
// accent the keyboard never produced.
//
// The order is what the answers are worth:
//
//   - What the HOST watched this keyboard type for the chord, if it has been
//     given a lookup (SetKeyChordText). It saw both halves of the keystroke;
//     everything below derives an answer from the key's NAME, which is a good
//     guess and only a guess.
//   - A one-character name IS that character.
//   - A Glyph token carries its glyph: "G-€" types "€". The character rides in
//     the token, so no lookup is needed to unroll it.
//   - The macOS Option layer, for a Mega chord, when it is switched on.
//
// The application spells the command: the character is a keyboard fact, the
// command is the application's vocabulary.
//
//	if text, known := p.TextForKey(key); known {
//		if text == "" {
//			return ""	// the key types nothing
//		}
//		return "insert '" + escape(text) + "'"
//	}
func (sp *Processor) TextForKey(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	// Asked first, and not gated by the Option switch: that switch says "stop
	// guessing", which is no reason to refuse an answer arrived at by looking.
	if sp.keyChordText != nil {
		if text, observed := sp.keyChordText(key); observed {
			return text, true
		}
	}
	if r := []rune(key); len(r) == 1 {
		return key, true
	}
	if glyph, ok := strings.CutPrefix(key, "G-"); ok {
		if r := []rune(glyph); len(r) == 1 {
			return glyph, true
		}
	}
	if ch, ok := sp.MacOptionChar(key); ok {
		return ch, true
	}
	return "", false
}
