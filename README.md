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
- Key aliases (`^H` = `back`, `^M` = `return`, …)
- Context-sensitive help topics per sequence prefix
- Completion listing for a partially typed sequence
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
