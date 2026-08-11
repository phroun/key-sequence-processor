package keyseq

// macOptionChars maps Meta key names back to the character macOS Option
// produces for that key (US layout).
//
// It exists because a terminal forces an either/or: Option is Meta, or Option
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

// SetMacOptionInsert enables the Option-character layer for unbound Meta keys
// (see macOptionChars). Off by default; the application decides, since the
// relevant keyboard is the user's, not the host's.
func (sp *Processor) SetMacOptionInsert(enabled bool) {
	sp.macOptionInsert = enabled
}

// MacOptionChar returns the character macOS Option produces for a Meta key
// name, and whether the layer applies to it: false when the layer is off, or
// when the key is not one Option composes.
//
// A default handler consults this to decide what an unbound M- key types, and
// spells the command itself — the character is a keyboard fact, the command is
// the application's vocabulary:
//
//	if ch, ok := p.MacOptionChar(key); ok {
//		return "insert '" + escape(ch) + "'"
//	}
func (sp *Processor) MacOptionChar(key string) (string, bool) {
	if !sp.macOptionInsert {
		return "", false
	}
	ch, ok := macOptionChars[key]
	return ch, ok
}
