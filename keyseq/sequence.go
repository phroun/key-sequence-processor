// Package keyseq resolves key events into application commands: multi-key
// sequences (WordStar/JOE-style chords such as ^K X), precedence levels,
// per-level wildcards, key aliases, and context-sensitive help topics.
//
// The processor owns the resolution rules and nothing else. Commands are
// opaque strings it hands back to the application, key names are whatever
// vocabulary the application feeds it, and what an unbound key does is the
// application's to answer (see DefaultHandler) — so it carries no assumptions
// about the program it drives, and depends only on the standard library.
package keyseq

import (
	"sort"
	"strings"
	"unicode"
)

// virtualHelpKey is the reserved pseudo-key whose mapping names the Quick Help
// topic for a prefix (see HelpTopic). It is not a real key and is filtered out
// of key completions.
const virtualHelpKey = "help"

// wildcardKey is the mapping key that matches ANY single key event at its
// level. It never participates in sequences and never appears in completions:
// it is a level's answer for the keys the level does not name.
const wildcardKey = "*"

// ProcessResult represents the result of processing a key.
type ProcessResult struct {
	Command string // Command resolved (resolution-only mode; empty when the processor executed)
	Handled bool   // Whether the key was handled
}

// CommandExecutor executes one command on behalf of the processor and reports
// whether it HANDLED the key. Only a clean false status means "not mine": that
// is a binding's way of declining the key, dropping resolution to the next
// level down. Anything else — success, an async suspension, an error — holds
// the key, so a command that merely went wrong can never volunteer its
// keystroke to a layer below it.
//
// key is the key event being resolved (the final key, for a sequence match).
//
// A nil executor puts the processor in resolution-only mode: ProcessKey
// reports the top-precedence command in ProcessResult.Command instead of
// executing, and never descends, because there is no status to descend on.
type CommandExecutor func(key, command string) bool

// DefaultHandler supplies the command for a key no binding claimed — what a
// plain "q" does when nothing maps it, what Backspace does out of the box.
//
// This is deliberately the application's to answer, because the answer is
// written in the application's command vocabulary: one editor spells it
// "insert 'q'", another "self-insert". Returning "" leaves the key unhandled.
//
// A binding always outranks it: the handler is consulted only after every
// precedence level has declined, so a user who maps a key still wins.
type DefaultHandler func(key string) string

// levelBindings is one precedence level's slice of the keymap: its named
// sequences, and its wildcard if it declared one.
type levelBindings struct {
	specific map[string]string // sequence -> command
	wildcard string
	hasWild  bool
}

// candidate is one level's claim on a key event.
type candidate struct {
	level           int
	command         string
	wildcard        bool
	matchedSequence string // the spelling that matched (aliases; the pending machinery reads it)
	isFallback      bool
}

// Processor handles key sequence detection and processing.
// It supports multi-key sequences like ^K X (Ctrl-K followed by X),
// with disambiguation for sequences that could be prefixes of longer
// sequences.
//
// Bindings carry a precedence LEVEL, written as parenthesized "(capture)" (+1)
// / "(override)" (+2) words on the mapping key — `(capture) * = tinput_key` —
// and resolution runs from the highest level down:
//
//  1. The longest live sequence possibility wins, across all levels: a key
//     that could begin (or extend) a mapped sequence is held, wildcards
//     notwithstanding. Nothing single-key outranks a chord in progress.
//  2. Among matches of equal length, the highest level is considered first.
//  3. Within a level, a specific binding SHADOWS the wildcard — each level
//     contributes at most one candidate, so `(capture) ^C = false` really does
//     drop ^C past the whole capture level, not into its own wildcard.
//  4. A candidate declining (clean false status) drops consideration one
//     level and re-runs. Exhausted candidates fall to the default handling
//     floor — what an unbound key would have done all along.
//  5. A sequence disqualified by an invalid continuation unwinds: the starter
//     is RELEASED (offered to the wildcards, then its literal self), and the
//     rest replay as ordinary keys.
type Processor struct {
	// rawMap holds bindings exactly as configured, level words included.
	// It is the API surface (MapKey/GetMapping/GetAllMappings) so config
	// merging and provenance see the same spellings the user wrote; the
	// parsed view below is derived from it.
	rawMap   map[string]string
	executor CommandExecutor

	// Parsed view: per-level bindings, levels in descending order, and the
	// union of every level's specific sequences (starter/prefix/completion
	// checks care only that a sequence exists somewhere).
	levels     map[int]*levelBindings
	levelOrder []int
	allKeys    map[string]bool

	// Key alias mappings for fallbacks
	keyAliases     map[string]string
	simpleControls map[string]string

	// aliasGroupOf maps every spelling to the full group it belongs to, so a
	// token can be expanded to its equivalents wherever it sits in a sequence
	// — not only at the tail, and not only as a continuation key.
	aliasGroupOf map[string][]string

	// defaultHandler supplies the command for an unbound key.
	defaultHandler DefaultHandler

	// macOptionInsert enables the Option-character layer (see MacOptionChar).
	macOptionInsert bool

	// Sequence tracking
	sequenceStarters        map[string]bool
	controlSequenceStarters map[string]bool

	// Current state
	activeSequence       string
	pendingShortMatch    string
	pendingFallbackMatch string
	keyBuffer            []string
	isReprocessing       bool

	debug bool
}

// NewProcessor creates a new key sequence processor.
func NewProcessor(executor CommandExecutor) *Processor {
	sp := &Processor{
		rawMap:                  make(map[string]string),
		executor:                executor,
		keyAliases:              make(map[string]string),
		simpleControls:          make(map[string]string),
		sequenceStarters:        make(map[string]bool),
		controlSequenceStarters: make(map[string]bool),
		keyBuffer:               make([]string, 0),
	}
	sp.applyAliasGroups(DefaultAliasGroups())
	sp.rebuild()
	return sp
}

// AliasGroup is a set of interchangeable spellings for one key. The FIRST
// entry is the primary — the name a key actually arrives under — and the rest
// are spellings a binding may use for it, so `^I` and `Tab` reach the same
// binding whichever one the keymap wrote.
type AliasGroup []string

// DefaultAliasGroups returns the aliases a Processor starts with: the control
// characters a terminal cannot distinguish from a named key, spelled in
// github.com/phroun/direct-key-handler's vocabulary, which is the vocabulary
// this package documents its examples in.
//
// An application with its own key names supplies its own groups
// (SetAliasGroups) — these are a sensible default, not an assumption.
//
// Return and Enter are deliberately NOT aliased. They are two physical keys
// (the home row's and the keypad's); an application that wants them
// interchangeable says so, rather than losing the distinction by default.
func DefaultAliasGroups() []AliasGroup {
	return []AliasGroup{
		// A terminal sends the same byte for these as for the control chord,
		// so the two spellings name one key.
		{"Backspace", "^H", "^8"}, // ^8 is DEL (127), which arrives as Backspace
		{"Tab", "^I"},
		{"Return", "^M"},
		{"Escape", "^[", "^3", "Esc"},

		// Control-number spellings for the control characters that have no
		// letter: a keyboard produces these with Ctrl and a digit.
		// NUL, which Ctrl+@ and Ctrl+Space both send — indistinguishable on the
		// byte wire, so the three spellings name one input. "Space" is this
		// package's name for the key (direct-key-handler's KeySpace), and the
		// modifier is what makes the token: a lowercase "^space" here was a
		// name nothing emits, which left a correctly-spelled "^Space" binding
		// unreachable from the byte that arrives.
		{"^@", "^2", "^Space"},
		{"^\\", "^4"},
		{"^]", "^5"},
		{"^^", "^6"},
		{"^_", "^7"},

		// Word spellings for punctuation. These are not equivalences between
		// two keys — nothing ever emits "Minus"; the word exists only so a
		// binding can be WRITTEN without fighting the syntax it is written in.
		// "-" is the modifier separator, so "M--" reads badly and parses worse;
		// "M-Minus" says the same thing plainly. (The processor already relies
		// on this for the spacebar, which cannot be spelled literally at all,
		// since a key name may not contain a space.)
		//
		// For some of these the word is not a convenience but the only way in.
		// A keymap is usually written in a config file, and a config file's own
		// metacharacters are exactly the keys that cannot be spelled literally
		// on the left of a binding: a line starting with ";" or "#" is a
		// comment, "=" separates key from command, and a comma reads as a list.
		// Semicolon, Octothorpe, Equals and Comma are how those keys get bound
		// at all.
		{"-", "Minus"},
		{"+", "Plus"},
		{"=", "Equals"},
		{"'", "Apos"},
		{"\"", "Quote"},
		{"~", "Tilde", "Wave"},
		{"`", "Backtick"},
		{"\\", "Backslash"},
		{"/", "Slash"},
		{";", "Semicolon"},
		{":", "Colon"},
		{"|", "Pipe"},
		{",", "Comma"},
		{".", "Period", "Dot"},
		{"#", "Octothorpe"},

		// Short spellings for the named keys, which are what a user reaches for
		// when writing a keymap. Spellings again, not equivalences: nothing
		// emits "PgUp", so there is no distinction to lose. (Escape's "Esc" is
		// in its group above, where the control forms already live.)
		//
		// Delete is deliberately absent. Del and Delete are not one key under
		// two names: on a PC, Del is forward delete, while the key a Mac labels
		// "delete" is Backspace. Folding them would silently bind the wrong key
		// on one platform or the other — the one case here where an abbreviation
		// means something different from the word it abbreviates.
		{"PageUp", "PgUp"},
		{"PageDown", "PgDn", "PgDown"},
		{"Insert", "Ins"},
		{"PrintScreen", "PrtSc"},
	}
}

// SetAliasGroups replaces the alias groups (see AliasGroup). Pass nil to drop
// aliasing entirely — an application whose key names carry no such ambiguity
// wants no fallbacks invented for it.
//
// Call before mapping keys: the parsed keymap is rebuilt from the new groups.
func (sp *Processor) SetAliasGroups(groups []AliasGroup) {
	sp.keyAliases = make(map[string]string)
	sp.simpleControls = make(map[string]string)
	sp.aliasGroupOf = make(map[string][]string)
	sp.applyAliasGroups(groups)
	sp.rebuild()
}

// applyAliasGroups indexes groups into the lookup maps resolution uses.
func (sp *Processor) applyAliasGroups(groups []AliasGroup) {
	if sp.aliasGroupOf == nil {
		sp.aliasGroupOf = make(map[string][]string)
	}
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		members := append([]string(nil), group...)
		for _, m := range members {
			sp.aliasGroupOf[m] = members
		}
		primary := group[0]
		for _, alias := range group[1:] {
			if len(alias) == 2 && alias[0] == '^' {
				sp.simpleControls[primary] = alias
			}
			sp.keyAliases[alias] = primary
		}
	}
}

// parseRawKey splits a raw mapping key into its precedence level and the
// physical key sequence.
//
// Everything a mapping key says ABOUT itself is written in parentheses, and
// everything else is keys you press. "(capture)" adds one level and
// "(override)" adds two; they compound, so "(capture) (capture) ^C" sits at
// level 2 and a layer can always outbid another by writing one more word. What
// remains is the sequence; a remainder of exactly "*" is that level's
// single-key wildcard.
//
// The words may appear ANYWHERE in the key — "^C (capture)" and "(capture) ^C"
// are the same binding — because a level is a property of the binding rather
// than a position in it.
//
// A parenthesized token this processor does not recognize is SKIPPED, not read
// as a key, and that is the point of the notation rather than an oversight. An
// application layered over this one writes its own metadata in the same
// parentheses — which platform a binding is for, say — and each reader takes
// the words it knows and passes over the rest. Bare words could not do that: a
// reader that had never heard of "capture" would have no way to tell it from
// the name of a key, and would quietly bind a key nobody can press.
//
// A parenthesis is itself a pressable key, so only a WHOLE token of the form
// "(word)" is metadata: "(", ")", "()" and "^(" are keys like any other.
func parseRawKey(raw string) (level int, seq string) {
	var keys []string
	for _, tok := range strings.Fields(raw) {
		switch metaWord(tok) {
		case "":
			keys = append(keys, tok)
		case "capture":
			level++
		case "override":
			level += 2
		}
	}
	if len(keys) == 0 {
		// Metadata and nothing to press. Rather than bind the empty key, keep
		// the line as it was written and let it be the dead binding it is.
		return 0, raw
	}
	return level, strings.Join(keys, " ")
}

// metaWord returns the word inside a whole parenthesized metadata token, or ""
// for a token that is a key. "()" is a key: a metadata token has something
// between its parentheses.
func metaWord(tok string) string {
	if len(tok) > 2 && tok[0] == '(' && tok[len(tok)-1] == ')' {
		return tok[1 : len(tok)-1]
	}
	return ""
}

// DisplayKey returns the physical key sequence a raw mapping key names, with
// any (capture)/(override) level word stripped — the spelling to SHOW a user,
// since the keys they press are the same at every level. Reports false for a
// wildcard binding, which names no pressable key at all.
func DisplayKey(raw string) (string, bool) {
	_, seq := parseRawKey(raw)
	if seq == wildcardKey {
		return "", false
	}
	return seq, true
}

// rebuild derives the per-level view from rawMap. Called on every mutation;
// keymaps are small and switches are rare (focus changes), so clarity wins
// over incremental bookkeeping.
func (sp *Processor) rebuild() {
	sp.levels = make(map[int]*levelBindings)
	sp.allKeys = make(map[string]bool)

	// Deterministic on duplicate (level, sequence) pairs spelled differently
	// ("(capture) (capture) X" vs "(override) X"): process raw keys in sorted
	// order, so the lexicographically-last spelling wins, every time.
	raws := make([]string, 0, len(sp.rawMap))
	for raw := range sp.rawMap {
		raws = append(raws, raw)
	}
	sort.Strings(raws)

	for _, raw := range raws {
		level, seq := parseRawKey(raw)
		lb := sp.levels[level]
		if lb == nil {
			lb = &levelBindings{specific: make(map[string]string)}
			sp.levels[level] = lb
		}
		if seq == wildcardKey {
			lb.wildcard = sp.rawMap[raw]
			lb.hasWild = true
			continue
		}
		lb.specific[seq] = sp.rawMap[raw]
		sp.allKeys[seq] = true
	}

	sp.levelOrder = sp.levelOrder[:0]
	for level := range sp.levels {
		sp.levelOrder = append(sp.levelOrder, level)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sp.levelOrder)))

	sp.applyNamedCaptureSuppression()
	sp.updateSequenceStarters()
}

// applyNamedCaptureSuppression removes from allKeys every sequence whose
// STARTER a higher level claims BY NAME, which is the one exception to rule 1.
//
// Naming a key at a capture level is a claim on the key itself, not on "the
// key unless something below happens to build a chord out of it": `(capture) esc
// = tinput_key` in a terminal's keymap means Escape belongs to the child, and
// it would read very strangely for a lower level's esc-chords to quietly
// outrank it. So
// a named single-key binding suppresses lower levels' sequences that start
// with it, and Escape stops being a prefix in that scope entirely.
//
// The WILDCARD deliberately does not suppress. It is a level's answer for the
// keys it did not name — the "everything else" net — and if it suppressed,
// `(capture) *` would silently kill ^B N and every other chord the moment a
// terminal took focus. Rule 1 keeps chords intact under the wildcard; only an
// explicit name overrides them.
//
// Same level does not suppress: a level's own chords are its business, and
// the pending-short-match machinery already handles a key that is both a
// binding and a prefix.
func (sp *Processor) applyNamedCaptureSuppression() {
	// The highest level at which each key is claimed by name as a single key.
	named := make(map[string]int)
	for level, lb := range sp.levels {
		for seq := range lb.specific {
			if strings.Contains(seq, " ") {
				continue
			}
			if cur, ok := named[seq]; !ok || level > cur {
				named[seq] = level
			}
		}
	}
	if len(named) == 0 {
		return
	}
	// Rebuild rather than delete: the same sequence can be bound at several
	// levels, and it survives if ANY of them outranks the claim.
	sp.allKeys = make(map[string]bool, len(sp.allKeys))
	for level, lb := range sp.levels {
		for seq := range lb.specific {
			if i := strings.IndexByte(seq, ' '); i >= 0 {
				if claimed, ok := named[seq[:i]]; ok && level < claimed {
					continue
				}
			}
			sp.allKeys[seq] = true
		}
	}
}

// MapKey maps a key sequence to a command.
func (sp *Processor) MapKey(keySequence, command string) {
	sp.rawMap[keySequence] = command
	sp.rebuild()
}

// UnmapKey removes a key mapping.
func (sp *Processor) UnmapKey(keySequence string) {
	delete(sp.rawMap, keySequence)
	sp.rebuild()
}

// GetMapping returns the command mapped to a key sequence (in its raw
// spelling, level words included), or empty string if not found.
func (sp *Processor) GetMapping(keySequence string) string {
	return sp.rawMap[keySequence]
}

// HelpTopic resolves the context-sensitive help topic for a key prefix: the
// value of the deepest "help" virtual binding matching activeSequence. It walks
// the prefix from full length down to the root, returning the first "<prefix>
// help" mapping found ("help" at the root); levels are searched highest first.
// Empty when no help binding applies. The "help" pseudo-key never fires as a
// command — it exists only to be looked up here — so it is excluded from
// completions.
func (sp *Processor) HelpTopic(activeSequence string) string {
	var parts []string
	if activeSequence != "" {
		parts = strings.Split(activeSequence, " ")
	}
	for i := len(parts); i >= 0; i-- {
		var key string
		if i == 0 {
			key = "help"
		} else {
			key = strings.Join(parts[:i], " ") + " help"
		}
		if v, ok := sp.lookupSeqDirect(key); ok && v != "" {
			return unquoteMapping(v)
		}
	}
	return ""
}

// lookupSeqDirect finds a sequence's command by exact spelling, highest level
// first.
func (sp *Processor) lookupSeqDirect(seq string) (string, bool) {
	for _, level := range sp.levelOrder {
		if lb := sp.levels[level]; lb != nil {
			if v, ok := lb.specific[seq]; ok {
				return v, true
			}
		}
	}
	return "", false
}

// unquoteMapping strips one layer of surrounding double quotes from a mapping
// value. Mapping values are usually written bare (window_prior), but a topic is
// naturally quoted (help ="keys"), and the config parser keeps the quotes — so
// the help topic would otherwise be the literal `"keys"`, not keys.
func unquoteMapping(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// SetMappings replaces the entire keymap with a copy of m (used to switch the
// active mapping set when the focused window changes).
func (sp *Processor) SetMappings(m map[string]string) {
	sp.rawMap = make(map[string]string, len(m))
	for k, v := range m {
		sp.rawMap[k] = v
	}
	sp.rebuild()
}

// GetAllMappings returns a copy of all key mappings, keyed by their raw
// spellings (level words included — see DisplayKey for the user-facing
// form).
func (sp *Processor) GetAllMappings() map[string]string {
	result := make(map[string]string, len(sp.rawMap))
	for k, v := range sp.rawMap {
		result[k] = v
	}
	return result
}

// updateSequenceStarters rebuilds the set of sequence starters from the
// parsed keymap. Starters are drawn from every level: a "(capture) ^B N" makes
// ^B a starter exactly as a level-0 "^B N" does.
func (sp *Processor) updateSequenceStarters() {
	sp.sequenceStarters = make(map[string]bool)
	sp.controlSequenceStarters = make(map[string]bool)

	for key := range sp.allKeys {
		if !strings.Contains(key, " ") {
			continue
		}
		firstPart := strings.Split(key, " ")[0]
		control := isControlSpelling(firstPart)

		// Every spelling matching can produce for the first slot must open the
		// sequence, or the chord never begins and the later slots never get
		// their chance. partVariants is exactly that set, so registering from it
		// keeps the two sides from drifting: a chord bound "M x" opens on "m"
		// for the same reason its tail completes on either case, and one bound
		// "esc x" opens on Escape's control form.
		//
		// Control-ness follows the BOUND spelling, so an alias cannot silently
		// turn a named chord into a control one.
		for _, sp2 := range sp.partVariants(firstPart, false) {
			sp.sequenceStarters[sp2] = true
			if control {
				sp.controlSequenceStarters[sp2] = true
			}
		}
	}
}

// isControlStarter reports whether a sequence beginning with this key is a
// control chord, considering every spelling of the key: the answer must not
// change with which equivalent name the user happened to press.
func (sp *Processor) isControlStarter(key string) bool {
	if sp.controlSequenceStarters[key] {
		return true
	}
	for _, sib := range sp.aliasSiblings(key) {
		if sp.controlSequenceStarters[sib] {
			return true
		}
	}
	return false
}

// modifierPrefixes are the modifier spellings that stack in front of a base key
// name, matching direct-key-handler's vocabulary, plus the caret that spells
// Control. The caret takes no separator, which is the whole reason it is listed
// separately: "^-" and "^Minus" are the same key, and a reader writing the
// second should not have to know that the first is spelled without a dash.
// Order matters only for matching the longest spelling first; the canonical
// ORDER a stack is written in is modifierRank below.
var modifierPrefixes = []string{"S-", "M-", "m-", "C-", "s-", "H-", "G-", "^"}

// modifierRank is the canonical order a stack of modifiers is written in. Two
// keymaps that name the same chord in different orders name the same key, so
// matching sorts the stack before comparing — order is not meaning.
//
// The sequence follows the order macOS renders modifiers (⌃⌥⇧⌘), extended with
// the ones a terminal can report that a Mac keyboard has no cap for. Control's
// caret form sorts LAST so it lands against the base key, which is where a
// reader expects to find it: "M-S-^X", not "^M-S-X".
var modifierRank = map[string]int{
	"C-": 0, // Control
	"G-": 1, // Glyph (AltGr / ISO_Level3_Shift; private)
	"M-": 2, // Meta, as induced by the PC Alt key
	"m-": 3, // Meta proper (the modifier a Space Cadet keyboard had its own key for)
	"S-": 4, // Shift
	"s-": 5, // Super / Command
	"H-": 6, // Hyper
	"^":  7, // Control again, hugging the base key
}

// prefixAliasGroups are modifier prefixes that reach each other. Two shapes
// live here, and the difference is real even though the machinery is the same:
//
//   - "^" and "C-" are two SPELLINGS of one modifier. Nothing can tell them
//     apart, because there is nothing to tell apart.
//   - "M-" and "m-" are two DIFFERENT modifiers that fall back to each other.
//     A terminal reports the PC Alt key and a true Meta key on separate bits,
//     and most keyboards only have the first. Binding one catches either;
//     binding both keeps them apart, since the spelling as pressed always
//     enumerates first.
//
// There is no "A-". The PC Alt key induces Meta, and "M-" is what that is
// called here — a separate Alt modifier would be a distinction no keyboard in
// this vocabulary can make.
var prefixAliasGroups = [][]string{
	{"^", "C-"},
	{"M-", "m-"},
}

// prefixSiblings maps each modifier spelling to its whole group.
var prefixSiblings = func() map[string][]string {
	m := make(map[string][]string)
	for _, g := range prefixAliasGroups {
		for _, p := range g {
			m[p] = g
		}
	}
	return m
}()

// isControlSpelling reports whether a key token names a Control chord, under
// either spelling of the modifier.
//
// A modifier needs something to modify: a bare "^" is the caret CHARACTER, a
// key a user can type and map like any other, not Control-of-nothing. Reading
// it as a control chord would switch the control/case ladder on for whatever
// followed it, so a sequence bound "^ M" would also answer to caret then
// Ctrl-M. The same goes for a bare "C-".
func isControlSpelling(key string) bool {
	return (strings.HasPrefix(key, "^") && len(key) > 1) ||
		(strings.HasPrefix(key, "C-") && len(key) > 2)
}

// splitModifierStack breaks a modifier prefix into its component spellings.
// A remainder that is not a known modifier comes back as the trailing rest,
// carried through verbatim rather than dropped.
func splitModifierStack(prefix string) (mods []string, rest string) {
	for prefix != "" {
		comp := ""
		for _, p := range modifierPrefixes {
			if strings.HasPrefix(prefix, p) {
				comp = p
				break
			}
		}
		if comp == "" {
			return mods, prefix
		}
		mods = append(mods, comp)
		prefix = prefix[len(comp):]
	}
	return mods, ""
}

// canonicalizeStack sorts a stack of modifier spellings into modifierRank
// order. The sort is stable, so two spellings that share a rank (M- and its
// A- alias) keep the order they were written in rather than swapping.
func canonicalizeStack(mods []string) []string {
	out := append([]string(nil), mods...)
	sort.SliceStable(out, func(i, j int) bool {
		return modifierRank[out[i]] < modifierRank[out[j]]
	})
	return out
}

// permuteStack returns every ordering of a modifier stack, canonical order
// first so it is preferred among the fallbacks.
func permuteStack(mods []string) [][]string {
	var out [][]string
	var walk func(cur, rest []string)
	walk = func(cur, rest []string) {
		if len(rest) == 0 {
			out = append(out, append([]string(nil), cur...))
			return
		}
		for i := range rest {
			next := make([]string, 0, len(rest)-1)
			next = append(next, rest[:i]...)
			next = append(next, rest[i+1:]...)
			walk(append(cur, rest[i]), next)
		}
	}
	walk(nil, canonicalizeStack(mods))
	return out
}

// prefixSpellings returns every way a stack of modifier prefixes can be
// written: each component varied across its alias group, and the whole stack
// put in canonical order. Order is not meaning, so a keymap that writes
// "S-C-Up" is naming the same key as one that writes "C-S-Up".
func prefixSpellings(prefix string) []string {
	mods, rest := splitModifierStack(prefix)
	if len(mods) == 0 {
		return []string{prefix}
	}
	// Order is not meaning, and the keymap may have written its order either
	// way round, so every ordering has to be reachable — canonicalizing only
	// the pressed side would still miss a binding spelled "S-C-Up". Real chords
	// stack one to three deep; beyond that only the as-written and canonical
	// orders are generated, which keeps a pathological stack from exploding.
	stacks := [][]string{mods}
	if len(mods) > 1 && len(mods) <= 3 {
		stacks = permuteStack(mods)
	} else if len(mods) > 3 {
		stacks = append(stacks, canonicalizeStack(mods))
	}
	seen := make(map[string]bool)
	out := []string{}
	for _, stack := range stacks {
		combos := []string{""}
		for _, comp := range stack {
			variants := prefixSiblings[comp]
			if len(variants) == 0 {
				variants = []string{comp}
			}
			next := make([]string, 0, len(combos)*len(variants))
			for _, sofar := range combos {
				for _, v := range variants {
					next = append(next, sofar+v)
				}
			}
			combos = next
			if len(combos) > 64 {
				combos = combos[:64] // runaway guard; real chords stack 1-3 deep
			}
		}
		for _, c := range combos {
			if s := c + rest; !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// splitModifiers peels any stack of modifier prefixes off a key token and
// returns them alongside the base name. A token that is nothing but modifiers
// is left whole, so the base is never empty.
func splitModifiers(token string) (prefix, base string) {
	base = token
	for {
		matched := false
		for _, p := range modifierPrefixes {
			if len(base) > len(p) && strings.HasPrefix(base, p) {
				prefix, base = prefix+p, base[len(p):]
				matched = true
				break
			}
		}
		if !matched {
			return prefix, base
		}
	}
}

// aliasSiblings returns the other spellings of one key token: its own alias
// group, plus — for a modified key — the group of its base name with the
// modifiers put back on. The token itself is not included.
//
// The modifier pass is what makes a *spelling* (as opposed to an equivalence
// between two keys a decoder can both emit) earn its keep. A word like `minus`
// exists because `-` is the modifier separator, so `M--` is awkward to read and
// awkward to parse; a word that only resolved on a bare key would miss the very
// case it was invented for. Peeling the prefix off and putting it back lets
// `M-minus` and `M--` name one key, exactly as `minus` and `-` do.
//
// A letter's CASE rides along the same pass. Case is not part of which key a
// letter is — `M` and `m` are one keystroke, and hanging a modifier off it
// does not change that — so `s-M` and `s-m` name one key too. They differ only
// when BOTH are bound, which needs no rule here: the sequence as pressed is
// tried before any alias, so an exact binding always wins.
func (sp *Processor) aliasSiblings(token string) []string {
	var out []string
	add := func(s string) {
		if s == token {
			return
		}
		for _, e := range out {
			if e == s {
				return
			}
		}
		out = append(out, s)
	}
	for _, sib := range sp.aliasGroupOf[token] {
		add(sib)
	}
	if prefix, base := splitModifiers(token); prefix != "" {
		bases := append([]string{base}, sp.aliasGroupOf[base]...)
		// The case flip is a base spelling like any other, so it goes through
		// the same prefix pass. That is also what lets the two spellings of
		// Control meet: the caret form writes its letter uppercase, so `C-q`
		// only reaches a bound `^Q` if the flip and the prefix are applied
		// together.
		if flip := caseFlip(base); flip != "" {
			bases = append(bases, flip)
			bases = append(bases, sp.aliasGroupOf[flip]...)
		}
		for _, p := range prefixSpellings(prefix) {
			for _, b := range bases {
				add(p + b)
			}
		}
	}
	return out
}

// caseFlip returns a single letter with its case swapped, or "" for anything
// that is not one letter. Named keys and punctuation have no case to flip:
// `Enter` is a name, not a letter, and upper-casing it would invent a spelling
// nobody wrote.
func caseFlip(base string) string {
	r := []rune(base)
	if len(r) != 1 || !unicode.IsLetter(r[0]) {
		return ""
	}
	if unicode.IsUpper(r[0]) {
		return strings.ToLower(base)
	}
	return strings.ToUpper(base)
}

// GetActiveSequence returns the current active sequence.
func (sp *Processor) GetActiveSequence() string {
	return sp.activeSequence
}

// ClearActiveSequence clears the current sequence state.
func (sp *Processor) ClearActiveSequence() {
	sp.activeSequence = ""
	sp.pendingShortMatch = ""
	sp.pendingFallbackMatch = ""
}

// ProcessKey processes a key input and returns the result.
//
// With an executor installed, the processor RUNS the resolved bindings itself
// — a key can resolve to a stack of candidates across levels, tried in
// precedence order until one takes it — and ProcessResult.Command is empty.
// With a nil executor it reports the top candidate instead (resolution-only
// mode; there is no status to descend on).
func (sp *Processor) ProcessKey(key string) ProcessResult {
	// Don't process empty keys (likely timeout signals)
	if key == "" {
		return ProcessResult{Command: "", Handled: true}
	}

	// If we have an active sequence, handle accordingly
	if sp.activeSequence != "" {
		return sp.handleKeyWithActiveSequence(key)
	}

	// Rule 1: a key that could begin a mapped sequence is HELD, at any level,
	// wildcards notwithstanding — the chord in progress outranks every
	// single-key claim, and the wildcard only sees the starter if the
	// sequence later disqualifies (releaseKey).
	if sp.isSequenceStarter(key) {
		sp.activeSequence = key
		return ProcessResult{Command: "", Handled: true}
	}

	return sp.resolveKeyEvent(key)
}

// resolveKeyEvent runs one ordinary key (no sequence in play) through the
// level stack: candidates from the highest level down, then the default
// handling floor. The floor is load-bearing for the reclaim idiom — a
// `(capture) ^C = false` declines the capture so ^C falls through the levels
// and lands exactly where an uncaptured ^C always landed.
func (sp *Processor) resolveKeyEvent(key string) ProcessResult {
	cands := sp.getCandidates(key)

	if sp.executor == nil {
		if len(cands) > 0 {
			return ProcessResult{Command: cands[0].command, Handled: true}
		}
		return ProcessResult{Command: sp.defaultCommand(key), Handled: true}
	}

	if !sp.runCandidates(key, cands) {
		if command := sp.defaultCommand(key); command != "" {
			sp.executor(key, command)
		}
	}
	return ProcessResult{Handled: true}
}

// getCandidates builds the precedence-ordered claims on a key event: one
// candidate per level, highest level first. Within a level the specific
// binding SHADOWS the wildcard — the wildcard is a level's answer only for
// the keys that level does not name — so declining a specific `(capture) ^C`
// drops past the capture level entirely rather than into its own net.
// Wildcards claim single keys only; sequences are always named.
func (sp *Processor) getCandidates(seq string) []candidate {
	var cands []candidate
	single := !strings.Contains(seq, " ")

	for _, level := range sp.levelOrder {
		lb := sp.levels[level]
		if lb == nil {
			continue
		}
		if cmd, ok := lb.specific[seq]; ok {
			cands = append(cands, candidate{level: level, command: cmd, matchedSequence: seq})
			continue
		}
		found := false
		for _, fallback := range sp.getKeyFallbacks(seq) {
			if fallback == seq {
				continue
			}
			if cmd, ok := lb.specific[fallback]; ok {
				cands = append(cands, candidate{level: level, command: cmd, matchedSequence: fallback, isFallback: true})
				found = true
				break
			}
		}
		if found {
			continue
		}
		if single && lb.hasWild {
			cands = append(cands, candidate{level: level, command: lb.wildcard, wildcard: true, matchedSequence: seq})
		}
	}
	return cands
}

// primaryCandidate returns the index of the first specific (non-wildcard)
// candidate, or -1. The pending-match machinery keys on the specific match's
// spelling; a wildcard can neither begin nor extend a sequence.
func primaryCandidate(cands []candidate) int {
	for i, c := range cands {
		if !c.wildcard {
			return i
		}
	}
	return -1
}

// runCandidates executes candidates in order until one reports the key
// handled. Reports whether any did. A nil executor handles nothing.
func (sp *Processor) runCandidates(key string, cands []candidate) bool {
	if sp.executor == nil {
		return false
	}
	for _, c := range cands {
		if sp.executor(key, c.command) {
			return true
		}
	}
	return false
}

// getKeyFallbacks generates alternative sequences to try.
func (sp *Processor) getKeyFallbacks(sequence string) []string {
	fallbacks := []string{sequence}

	// Single character case variants
	if len(sequence) == 1 {
		r := rune(sequence[0])
		if unicode.IsLetter(r) {
			if unicode.IsUpper(r) {
				fallbacks = append(fallbacks, strings.ToLower(sequence))
			} else {
				fallbacks = append(fallbacks, strings.ToUpper(sequence))
			}
		}
	}

	// Every token admits its equivalent spellings, combined across ALL parts at
	// once — a key must reach its mapping whichever spelling the map uses,
	// wherever in the chord it sits and however many parts are aliased at the
	// same time ("^O ^V C" for a mapped "^O V ^C", "^K minus minus" for
	// "^K - -", "^[ x" for "esc x"). The as-pressed sequence enumerates first,
	// so an exact mapping always wins over an alias.
	if parts := strings.Split(sequence, " "); len(parts) > 1 {
		// Control-ness is a property of the chord being typed, decided by the
		// starter as pressed, and it applies only to CONTINUATION keys: a bare
		// "M" must never match a bound "^M".
		isControlStarter := sp.isControlStarter(parts[0])

		seqs := []string{}
		for i, part := range parts {
			variants := sp.partVariants(part, isControlStarter && i > 0)
			if i == 0 {
				seqs = variants
				continue
			}
			next := make([]string, 0, len(seqs)*len(variants))
			for _, prefix := range seqs {
				for _, v := range variants {
					next = append(next, prefix+" "+v)
				}
			}
			seqs = next
			if len(seqs) > 1024 {
				seqs = seqs[:1024] // runaway guard; real chords are 2-4 parts
			}
		}
		for _, s := range seqs {
			if s != sequence {
				fallbacks = append(fallbacks, s)
			}
		}
	}

	// A single key has no parts loop above, so expand its group here.
	if !strings.Contains(sequence, " ") {
		fallbacks = append(fallbacks, sp.aliasSiblings(sequence)...)
	}

	return fallbacks
}

// partVariants returns the equivalent spellings of one continuation key token
// within a sequence, the token itself always first (so as-pressed outranks any
// alias). A single letter admits its case flip; within a control-started
// sequence it also admits its control form (c/C ~ ^C), and a control token or
// a named key with a control alias admits its plain letter in both cases
// (^C ~ C ~ c, return ~ ^M ~ M ~ m). This is what lets "^B C" entered as
// "^B ^C" complete the mapping mid-sequence instead of abandoning the prefix.
// Alias-group spellings (esc/^[, minus/-, M-minus/M--) come from aliasSiblings
// and apply at any position, in or out of a control chord.
func (sp *Processor) partVariants(part string, controlSeq bool) []string {
	out := []string{part}
	add := func(s string) {
		for _, e := range out {
			if e == s {
				return
			}
		}
		out = append(out, s)
	}
	if r := []rune(part); len(r) == 1 && unicode.IsLetter(r[0]) {
		if unicode.IsUpper(r[0]) {
			add(strings.ToLower(part))
		} else {
			add(strings.ToUpper(part))
		}
		if controlSeq {
			add("^" + strings.ToUpper(part)) // control keys are canonically ^X
		}
	}
	// Alias-group siblings apply at ANY position and in any context: esc/^[ or
	// minus/- name one key wherever they appear, unlike the control/case layer
	// below, which is only meaningful for a continuation key.
	for _, sib := range sp.aliasSiblings(part) {
		add(sib)
	}
	if controlSeq {
		if cv := sp.getSimpleControl(part); cv != "" {
			add(cv)
			if len(cv) == 2 && cv[0] == '^' {
				letter := string(cv[1])
				add(letter)
				add(strings.ToLower(letter))
			}
		}
	}
	return out
}

// getSimpleControl converts a key to its control character equivalent.
func (sp *Processor) getSimpleControl(key string) string {
	if ctrl, ok := sp.simpleControls[key]; ok {
		return ctrl
	}
	if len(key) == 2 && key[0] == '^' {
		return key
	}
	return ""
}

// isSequenceStarter checks if a key starts a multi-key sequence.
func (sp *Processor) isSequenceStarter(key string) bool {
	if sp.sequenceStarters[key] {
		return true
	}

	// Check if any mapped sequence starts with this key plus a space
	searchPattern := key + " "
	for mappedKey := range sp.allKeys {
		if strings.HasPrefix(mappedKey, searchPattern) {
			sp.sequenceStarters[key] = true
			return true
		}
	}

	return false
}

// isPotentialSequenceMatch checks if a sequence could match a longer sequence.
// The prefix is matched as WHOLE KEYS, never as a substring: a mapped key
// extends `sequence` only when it equals it or continues with a space and
// another key. Without the token boundary the letter "r" would match the key
// name "return" (and "t" would match "tab"), holding the key for a chord that
// can never arrive.
func (sp *Processor) isPotentialSequenceMatch(sequence string) bool {
	// Check direct prefix matches
	if sp.hasTokenPrefix(sequence) {
		return true
	}

	// Check fallback prefixes
	for _, fallback := range sp.getKeyFallbacks(sequence) {
		if fallback == sequence {
			continue
		}
		if sp.hasTokenPrefix(fallback) {
			return true
		}
	}

	return false
}

// hasTokenPrefix reports whether any mapped key equals `seq` or continues it
// with another whole key (`seq` + " " + ...). Whole-token, never a substring.
func (sp *Processor) hasTokenPrefix(seq string) bool {
	withSep := seq + " "
	for mappedKey := range sp.allKeys {
		if mappedKey == seq || strings.HasPrefix(mappedKey, withSep) {
			return true
		}
	}
	return false
}

// hasLongerPotentialMatches checks if there are longer matches possible.
func (sp *Processor) hasLongerPotentialMatches(sequence, matchedSequence string) bool {
	searchPrefix := sequence + " "
	for mappedKey := range sp.allKeys {
		if strings.HasPrefix(mappedKey, searchPrefix) {
			return true
		}
	}

	if matchedSequence != "" && matchedSequence != sequence {
		fallbackPrefix := matchedSequence + " "
		for mappedKey := range sp.allKeys {
			if strings.HasPrefix(mappedKey, fallbackPrefix) {
				return true
			}
		}
	}

	for _, fallback := range sp.getKeyFallbacks(sequence) {
		if fallback == sequence {
			continue
		}
		fallbackPrefix := fallback + " "
		for mappedKey := range sp.allKeys {
			if strings.HasPrefix(mappedKey, fallbackPrefix) {
				return true
			}
		}
	}

	return false
}

// handleKeyWithActiveSequence processes a key when there's an active sequence.
func (sp *Processor) handleKeyWithActiveSequence(key string) ProcessResult {
	// Handle pending match disambiguation
	if sp.pendingShortMatch != "" {
		sp.keyBuffer = append(sp.keyBuffer, key)

		fullSequence := sp.activeSequence + " " + key
		isPotentialMatch := sp.isPotentialSequenceMatch(fullSequence)

		if !isPotentialMatch && sp.pendingFallbackMatch != "" {
			fallbackFullSeq := sp.pendingFallbackMatch + " " + key
			isPotentialMatch = sp.isPotentialSequenceMatch(fallbackFullSeq)
		}

		if !isPotentialMatch {
			sp.processPendingMatch()
			return ProcessResult{Command: "", Handled: true}
		}
	}

	fullSequence := sp.activeSequence + " " + key

	// Check for command match
	if cands := sp.getCandidates(fullSequence); len(cands) > 0 {
		matched := fullSequence
		isFallback := false
		if pi := primaryCandidate(cands); pi >= 0 {
			matched = cands[pi].matchedSequence
			isFallback = cands[pi].isFallback
		}
		if sp.hasLongerPotentialMatches(fullSequence, matched) {
			// Store as pending and wait for more input
			sp.pendingShortMatch = fullSequence
			if isFallback {
				sp.pendingFallbackMatch = matched
			} else {
				sp.pendingFallbackMatch = ""
			}
			sp.keyBuffer = nil
			sp.activeSequence = fullSequence
			return ProcessResult{Command: "", Handled: true}
		}

		// No longer matches possible: resolve now.
		sp.ClearActiveSequence()
		if sp.executor == nil {
			return ProcessResult{Command: cands[0].command, Handled: true}
		}
		// A MATCHED sequence is consumed whether or not a candidate takes it:
		// declining descends through the levels, but a sequence every level
		// declined does not unwind into the child or the buffer — ^B N
		// failing must not type "N".
		sp.runCandidates(key, cands)
		return ProcessResult{Command: "", Handled: true}
	}

	// Check if this could be a prefix
	if sp.isPotentialSequenceMatch(fullSequence) {
		sp.activeSequence = fullSequence
		return ProcessResult{Command: "", Handled: true}
	}

	// Handle pending short match if sequence invalid
	if sp.pendingShortMatch != "" {
		sp.processPendingMatch()
		return ProcessResult{Command: "", Handled: true}
	}

	// Invalid continuation: unwind. The starter is RELEASED — its sequence
	// came to nothing, so the capture layers get it (this is how a held ^B
	// reaches a hosted shell), else its literal self — and the middle keys
	// plus this one replay as ordinary keys.
	parts := strings.Split(sp.activeSequence, " ")
	sp.ClearActiveSequence()
	sp.releaseKey(parts[0])
	for i := 1; i < len(parts); i++ {
		sp.handleSingleKey(parts[i])
	}
	sp.handleSingleKey(key)
	return ProcessResult{Command: "", Handled: true}
}

// releaseKey lets go of a sequence starter whose sequence disqualified. The
// key was never an ordinary keystroke — it was held as a possible prefix — so
// it does not re-enter sequence consideration and its own specific bindings
// (which lost to the sequence the moment it was held) do not fire. The
// wildcards get first refusal, highest level down: for a hosted terminal that
// is `(capture) * = tinput_key`, sending the starter to the child as-is. Failing
// those it falls to the default handler, exactly as an unbound key would.
func (sp *Processor) releaseKey(key string) {
	if sp.executor != nil {
		for _, level := range sp.levelOrder {
			if lb := sp.levels[level]; lb != nil && lb.hasWild {
				if sp.executor(key, lb.wildcard) {
					return
				}
			}
		}
	}
	if sp.executor != nil {
		if cmd := sp.defaultCommand(key); cmd != "" {
			sp.executor(key, cmd)
		}
	}
}

// processPendingMatch executes a pending match and reprocesses buffered keys.
func (sp *Processor) processPendingMatch() {
	if sp.pendingShortMatch == "" {
		return
	}

	cands := sp.getCandidates(sp.pendingShortMatch)
	keysToReprocess := sp.keyBuffer
	parts := strings.Split(sp.pendingShortMatch, " ")
	lastKey := parts[len(parts)-1]

	sp.pendingShortMatch = ""
	sp.pendingFallbackMatch = ""
	sp.keyBuffer = nil
	sp.ClearActiveSequence()

	if len(cands) > 0 && sp.executor != nil {
		sp.runCandidates(lastKey, cands)

		sp.isReprocessing = true
		for _, k := range keysToReprocess {
			sp.handleSingleKey(k)
		}
		sp.isReprocessing = false
	}
}

// handleSingleKey processes a single key outside of sequence tracking.
func (sp *Processor) handleSingleKey(key string) {
	// Check if sequence starter
	if sp.isSequenceStarter(key) {
		sp.activeSequence = key
		return
	}

	sp.resolveKeyEvent(key)
}

// defaultCommand asks the application what an unbound key should do. No
// handler means unbound keys do nothing.
func (sp *Processor) defaultCommand(key string) string {
	if sp.defaultHandler == nil {
		return ""
	}
	return sp.defaultHandler(key)
}

// SetDefaultHandler installs the handler consulted for a key no binding
// claimed (see DefaultHandler).
func (sp *Processor) SetDefaultHandler(h DefaultHandler) {
	sp.defaultHandler = h
}

// GetPossibleCompletions returns possible completions for the current sequence.
// Returns completions sorted alphabetically to match TypeScript behavior.
func (sp *Processor) GetPossibleCompletions() []string {
	if sp.activeSequence == "" {
		return nil
	}

	completions := make(map[string]bool)
	prefix := sp.activeSequence + " "

	for mappedKey := range sp.allKeys {
		if strings.HasPrefix(mappedKey, prefix) {
			nextPart := strings.TrimPrefix(mappedKey, prefix)
			nextKey := strings.Split(nextPart, " ")[0]
			completions[nextKey] = true
		}
	}

	// Check fallbacks too
	for _, fallback := range sp.getKeyFallbacks(sp.activeSequence) {
		if fallback == sp.activeSequence {
			continue
		}
		fallbackPrefix := fallback + " "
		for mappedKey := range sp.allKeys {
			if strings.HasPrefix(mappedKey, fallbackPrefix) {
				nextPart := strings.TrimPrefix(mappedKey, fallbackPrefix)
				nextKey := strings.Split(nextPart, " ")[0]
				completions[nextKey] = true
			}
		}
	}

	// "help" is a virtual binding (it names the Quick Help topic for this
	// prefix, resolved by HelpTopic), not a key the user can press — never
	// offer it as a completion.
	delete(completions, virtualHelpKey)

	result := make([]string, 0, len(completions))
	for k := range completions {
		result = append(result, k)
	}

	// Sort for deterministic output (matches TypeScript behavior)
	sort.Strings(result)

	return result
}

// DumpKeyMap returns a debug representation of the key map.
func (sp *Processor) DumpKeyMap() string {
	var sb strings.Builder
	sb.WriteString("Key Map:\n")
	for k, v := range sp.rawMap {
		sb.WriteString("  ")
		sb.WriteString(k)
		sb.WriteString(" -> ")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return sb.String()
}
