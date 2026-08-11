# key-sequence-processor

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

*WordStar/JOE-style key sequences and chords for Go: multi-key sequences, precedence levels, wildcards, aliases and context help topics.*
*If you use this, please support me on ko-fi:  [https://ko-fi.com/jeffday](https://ko-fi.com/F2F61JR2B4)*

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/F2F61JR2B4)

Pure Go, standard library only.

## Features

- Multi-key sequences (`^K X`, `^Q F`) with prefix disambiguation
- Precedence levels (`capture` / `override`) so a hosted layer can claim keys
- Per-level wildcards, and specific bindings that shadow them
- A binding can decline a key and drop resolution to the level below
- Configurable key aliases (`^I` = `Tab`, `^M` = `Return`, …)
- Context-sensitive help topics per sequence prefix
- Completion listing for a partially typed sequence
- Optional macOS Option-character layer for unbound Meta keys, on any platform
- No opinion about your key names, your commands, or what an unbound key does

## Installation

```bash
go get github.com/phroun/key-sequence-processor/keyseq
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/phroun/key-sequence-processor/keyseq"
)

func main() {
	p := keyseq.NewProcessor(func(key, command string) bool {
		fmt.Printf("%s -> %s\n", key, command)
		return true // handled; a clean false declines the key
	})

	p.SetMappings(map[string]string{
		"^K X":   "save_and_exit",
		"^K B":   "block_begin",
		"^Q F":   "find",
		"^K help": "\"Block commands\"", // shown while ^K is pending
	})

	// An unbound key is the application's business, not the processor's.
	p.SetDefaultHandler(func(key string) string {
		if len([]rune(key)) == 1 {
			return "insert '" + key + "'"
		}
		return ""
	})

	p.ProcessKey("^K") // opens the sequence, dispatches nothing
	p.ProcessKey("X")  // -> save_and_exit
	p.ProcessKey("q")  // -> insert 'q'
}
```

## Key names

The processor never parses a keyboard; it resolves whatever names you feed it.
A key event is a string, and a sequence is those strings joined by spaces —
so `"^K X"` is Ctrl-K then X, and a name may not itself contain a space (spell
the spacebar `space`, and `"^B space"` is a chord ending on it).

Names are yours to choose. If you decode a terminal with
[direct-key-handler](https://github.com/phroun/direct-key-handler), its
`Options.KeyNames` lets you emit exactly the vocabulary you bind against, so
nothing has to be translated in between.

## Precedence levels

Prefix a mapping with `capture` (+1) or `override` (+2) to raise its level;
they compound, so a layer can always outbid another by writing one more word.

```go
p.SetMappings(map[string]string{
	"up":         "cursor_up",   // level 0: the base keymap
	"capture *":  "terminal_key", // level 1: a hosted terminal claims everything
	"capture ^C": "false",        // ...except ^C, which it hands back
})
```

Resolution runs from the highest level down:

1. The longest live sequence wins across all levels — nothing single-key
   outranks a chord in progress.
2. Among equal-length matches, the highest level goes first.
3. Within a level, a specific binding shadows that level's wildcard.
4. A candidate that declines (a clean `false` from the executor) drops one
   level and re-runs.
5. Exhausted candidates fall to the default handler.

## Declining a key

`CommandExecutor` returns whether it *handled* the key. Only a clean `false`
means "not mine" — success, an async suspension, or an error all hold the key,
so a command that merely went wrong can never volunteer its keystroke to a
layer below it.

## Resolution-only mode

Pass a `nil` executor and nothing is dispatched: `ProcessKey` reports the
top-precedence command in `ProcessResult.Command` instead. Useful for showing a
user what a key would do.

## Key aliases

A terminal sends the same byte for `Tab` as for `^I`, so a binding written
either way should reach the same place. `DefaultAliasGroups` covers those cases
in [direct-key-handler](https://github.com/phroun/direct-key-handler)'s
vocabulary. The first entry of each group is the primary; the rest are
spellings a binding may use.

| Key | Spellings | Why |
|-----|-----------|-----|
| `Backspace` | `^H`, `^8` | `^H` is BS (8); `^8` is DEL (127), which arrives as Backspace |
| `Tab` | `^I` | one byte (9) for both |
| `Return` | `^M` | one byte (13) for both |
| `Escape` | `^[`, `^3` | one byte (27); `^3` is how a keyboard makes it with a digit |
| `^@` | `^2`, `^space` | NUL, also Ctrl+Space |
| `^\` | `^4` | FS |
| `^]` | `^5` | GS |
| `^^` | `^6` | RS |
| `^_` | `^7` | US |

`Return` and `Enter` are deliberately **not** aliased: they are two physical
keys, and folding them is an application's decision, not a default that quietly
discards the distinction.

Aliases are lookups, never rewrites. The spelling as pressed is tried first, so
naming both `^H` and `Backspace` keeps them separately bindable — a group only
fills in when the keymap left one of its spellings unnamed.

### Spellings

A second kind of entry names a key that has only one real token. Nothing ever
*emits* `Minus`; the word exists so a binding can be written without fighting
the syntax it is written in — `-` is the modifier separator, so `M--` reads
badly and `^-` cannot show where the modifier stops. (The processor already
depends on this for `space`, which cannot be spelled literally at all.)

| Key | Spelling | | Key | Spelling |
|-----|----------|-|-----|----------|
| `-` | `Minus` | | `\` | `Backslash` |
| `+` | `Plus` | | `/` | `Slash` |
| `=` | `Equals` | | `;` | `Semicolon` |
| `'` | `Apos` | | `:` | `Colon` |
| `"` | `Quote` | | \| | `Pipe` |
| `~` | `Tilde`, `Wave` | | | |
| `` ` `` | `Backtick` | | | |

Spellings resolve through modifiers, not just on a bare key: the prefix stack is
peeled off, the base varied, and the prefix put back, so `M-Minus` and `M--`
name one key. `^` and `C-` are one modifier under two spellings, so `^-`,
`^Minus`, `C--` and `C-Minus` all reach the same binding.

An application with its own key names supplies its own groups — first entry is
the primary, the rest are spellings a binding may use:

```go
p.SetAliasGroups([]keyseq.AliasGroup{
	{"esc", "escape", "^["},
	{"back", "^H", "backspace"},
})
```

Pass `nil` to drop aliasing entirely.

## The Option-character layer

A terminal forces an either/or: Option is Meta, or Option types characters —
not both. `SetMacOptionInsert(true)` restores the missing half from the binding
side. An `M-` key no binding claimed reports the character Option composes, so
bindings take the combos they name and every other combo still types:

```go
p.SetMacOptionInsert(true)
p.SetDefaultHandler(func(key string) string {
	if ch, ok := p.MacOptionChar(key); ok { // "M-d" -> "∂"
		return "insert '" + ch + "'"
	}
	return ""
})
```

It is a **user setting, never a property of the host OS** — nothing here reads
`runtime.GOOS`. Someone typing on a Mac keyboard through an SSH session to a
Linux box wants this layer, and the far end cannot see their keyboard. It also
gives a Linux or Windows terminal mac-style Option typing it never had.

The inverse — decoding the composed character back into an `M-` name so it can
be bound at all — is the input decoder's half, and
[direct-key-handler](https://github.com/phroun/direct-key-handler) carries it
as `DecodeMacOSOption`. The two are useful independently: with the decoder on,
`M-x` is bindable without writing `≈`; with only this layer, a terminal already
in Meta mode can still type Option characters.

## Help topics

Map the reserved pseudo-key `help` under a prefix and `HelpTopic` finds the
most specific one for the sequence in progress — `"^K B help"`, then
`"^K help"`, then `"help"` — which is how an on-screen helper narrates a chord
as the user types it.

## License

MIT — see [LICENSE](LICENSE).

## Change Log

### 0.1.0

- Extracted from the [mew](https://github.com/phroun/mew) editor, where this
  ran as `internal/keys`, and relicensed MIT.
- Default handling for unbound keys became the application's, through
  `SetDefaultHandler`, rather than a table of one editor's command names.
- `SequenceProcessor` is now `keyseq.Processor`.
- The Option-character layer stayed, and no longer consults the host OS:
  `SetMacOptionInsert` / `MacOptionChar` report the character, the application
  spells the command.

### 0.1.1

- Key aliases became configurable (`SetAliasGroups`, `AliasGroup`), and the
  defaults now use direct-key-handler's vocabulary (`Tab`, `Return`, `Escape`,
  `Backspace`) rather than one editor's. `Return` and `Enter` are no longer
  aliased to each other.
- **Upgrading:** an application whose key names differ from the defaults must
  now declare its own groups; previously one editor's were assumed.

### 0.1.2

- Aliases resolve in **every** position of a sequence, not only at the tail and
  not only one per chord. A chord bound `esc x` could not be typed `^[ x` at
  all — the initiator was not recognized, so the chord never began — and
  `^K Minus Minus` matched neither slot. Alias spellings of a bound initiator are
  now registered as initiators too.
- Aliases resolve **through modifier prefixes**: the prefix stack is peeled off,
  the base varied, and the prefix restored, so `M-Minus` and `M--` name one key.
- `^` and `C-` are recognized as one modifier under two spellings, in bindings
  and in control-chord detection alike (`C-B M` completes on `C-B ^M`).
- Added word **spellings** for punctuation to the defaults — `Minus`, `Plus`,
  `Equals`, `Apos`, `Quote`, `Tilde`/`Wave`, `Backtick`, `Backslash`, `Slash`,
  `Semicolon`, `Colon`, `Pipe` — so a binding can name a key the binding syntax
  would otherwise fight over.
- The control/case ladder is unchanged and still contextual: a plain letter
  admits its control form only inside a control-started chord.
- Fixed: a bare `^` is the caret character and is mappable as one. It was read
  as a control chord, which switched the control/case ladder on for whatever
  followed it, so a sequence bound `^ M` also answered to caret then Ctrl-M.
- Fixed: the first key of a chord now varies exactly as the others do. A chord
  bound `M x` could not be opened with a lowercase `m` — the prefix was never
  held, so the tail's case-insensitivity was unreachable. Only single-letter
  initiators are affected.

**Note on letter initiators:** binding a chord whose first key is an ordinary
character makes that character a prefix, so typing it waits for the next key
and an abandoned chord discards both. That has always been true of the bound
spelling; it now applies to the other case as well. Control keys and named
keys (`^K`, `esc`) are the ordinary choice for a reason.
