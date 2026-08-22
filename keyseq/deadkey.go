package keyseq

// Dead keys: the five Option chords that arm an accent for the next keystroke
// instead of typing a character of their own.
//
// A keyboard normally does this itself. macOS composes Option+i then "u" into
// "û" and hands the finished character over, and a host that receives one has
// nothing to do — kittytk's SDL platform knows these five chords only so it can
// tell "this typed nothing" from "this was not seen", and never combines
// anything.
//
// A terminal in the middle can lose it. Ghostty under the "kitty" keyboard
// protocol reports Option+i as an ordinary Mega chord that produced no text —
// correctly, as far as it goes — and then reports the following "u" as having
// typed a plain "u". The accent is dropped between the two, so the composition
// never happens anywhere and the keystroke is simply gone.
//
// Which is what this is for: when nothing upstream composed, compose here. The
// layer that already answers "what does an unbound Option chord type" is the
// natural place, because these five are exactly the entries of that table that
// answer "nothing, yet".

// deadKeyAccents maps the five dead-key chords to the accent each arms, which
// is also the character it types when nothing follows that combines.
//
// The same five, and the same characters, as macOptionChars lists for them and
// as kittytk's SDL platform carries in macOSDeadKeys. Three copies exist
// because they are reached by different routes — a chord name here, composed
// text there — and all three must agree.
var deadKeyAccents = map[string]string{
	"M-e": "´", // acute
	"M-i": "ˆ", // circumflex
	"M-n": "˜", // tilde
	"M-u": "¨", // diaeresis
	"M-`": "`", // grave
}

// composed maps an armed accent and the letter that follows to the single
// character macOS would have produced.
//
// Written out rather than derived by combining and normalising, because that
// would mean a Unicode tables dependency in a package that has none, and the
// set is small and fixed: five accents over the letters each actually takes on
// a US layout. A pair that is not here is one macOS does not compose either —
// there is no "ˆz" — and it falls back to the accent followed by the letter,
// which is what macOS does with it too.
var composed = map[string]string{
	// acute
	"´a": "á", "´e": "é", "´i": "í", "´o": "ó", "´u": "ú", "´y": "ý",
	"´A": "Á", "´E": "É", "´I": "Í", "´O": "Ó", "´U": "Ú", "´Y": "Ý",
	"´c": "ć", "´n": "ń", "´s": "ś", "´z": "ź",
	"´C": "Ć", "´N": "Ń", "´S": "Ś", "´Z": "Ź",

	// circumflex
	"ˆa": "â", "ˆe": "ê", "ˆi": "î", "ˆo": "ô", "ˆu": "û",
	"ˆA": "Â", "ˆE": "Ê", "ˆI": "Î", "ˆO": "Ô", "ˆU": "Û",
	"ˆc": "ĉ", "ˆg": "ĝ", "ˆh": "ĥ", "ˆj": "ĵ", "ˆs": "ŝ", "ˆw": "ŵ", "ˆy": "ŷ",
	"ˆC": "Ĉ", "ˆG": "Ĝ", "ˆH": "Ĥ", "ˆJ": "Ĵ", "ˆS": "Ŝ", "ˆW": "Ŵ", "ˆY": "Ŷ",

	// tilde
	"˜a": "ã", "˜n": "ñ", "˜o": "õ",
	"˜A": "Ã", "˜N": "Ñ", "˜O": "Õ",
	"˜i": "ĩ", "˜u": "ũ", "˜e": "ẽ", "˜y": "ỹ",
	"˜I": "Ĩ", "˜U": "Ũ", "˜E": "Ẽ", "˜Y": "Ỹ",

	// diaeresis
	"¨a": "ä", "¨e": "ë", "¨i": "ï", "¨o": "ö", "¨u": "ü", "¨y": "ÿ",
	"¨A": "Ä", "¨E": "Ë", "¨I": "Ï", "¨O": "Ö", "¨U": "Ü", "¨Y": "Ÿ",

	// grave
	"`a": "à", "`e": "è", "`i": "ì", "`o": "ò", "`u": "ù",
	"`A": "À", "`E": "È", "`I": "Ì", "`O": "Ò", "`U": "Ù",
	"`n": "ǹ", "`w": "ẁ", "`y": "ỳ",
	"`N": "Ǹ", "`W": "Ẁ", "`Y": "Ỳ",
}

// armDeadKey records the accent a dead-key chord opened, and reports whether
// the chord was one. Only while the Option layer is on: with it off these
// chords are for binding and nothing else, and arming an accent nobody is going
// to see would swallow the keystroke after.
func (sp *Processor) armDeadKey(key string) bool {
	if !sp.macOptionInsert {
		return false
	}
	accent, ok := deadKeyAccents[key]
	if !ok {
		return false
	}
	sp.deadKey = accent
	return true
}

// takeDeadKey applies a pending accent to the text a key typed, and reports
// whether one was pending.
//
// A pair that composes becomes the single character. One that does not becomes
// the accent followed by the text, which is what macOS puts in a document when
// a dead key is answered with something it cannot take — the accent stands on
// its own and the keystroke lands after it, unchanged.
//
// Spent either way. An accent answers the next keystroke that types, and one
// only.
func (sp *Processor) takeDeadKey(text string) (string, bool) {
	accent := sp.deadKey
	if accent == "" {
		return text, false
	}
	sp.deadKey = ""
	if whole, ok := composed[accent+text]; ok {
		return whole, true
	}
	return accent + text, true
}

// ResetDeadKey drops a pending accent without typing it.
//
// For an application that knows the keyboard has gone elsewhere — focus lost, a
// buffer closed — where an accent left armed would attach itself to whatever is
// typed next, whenever that is and wherever it lands.
func (sp *Processor) ResetDeadKey() {
	sp.deadKey = ""
}

// DeadKeyPending reports the accent waiting for the next keystroke, and whether
// one is. For an application that shows the user what the keyboard is holding.
func (sp *Processor) DeadKeyPending() (string, bool) {
	return sp.deadKey, sp.deadKey != ""
}
