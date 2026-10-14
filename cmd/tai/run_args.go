package main

import "strings"

// valueFlags are the flags that can appear before the run target and take
// the next token as their value. splitRunArgs needs them to tell a value from
// the target, and Ra does not expose flag types, so a new global flag that
// takes a value must be added here.
var valueFlags = map[string]bool{"--config": true}

// splitRunArgs splits a `tai run` command line at the target: taiArgs runs up
// to and including the target, and targetArgs is everything after it, as
// written. Any other command line comes back whole, with no targetArgs.
//
// The target's arguments cannot go through Ra. Ra claims any flag it knows
// wherever it appears, so the target would lose a --config or --no-color of
// its own, and it shows tai's help for a --help anywhere in the command line,
// even after "--".
func splitRunArgs(args []string) (taiArgs, targetArgs []string) {
	cmd := nextPositional(args, 0)
	if cmd < 0 || args[cmd] != "run" {
		return args, nil
	}
	target := nextPositional(args, cmd+1)
	if target < 0 {
		return args, nil
	}
	rest := args[target+1:]
	// Before this split existed, "--" was the only way to pass a flag through,
	// so it is dropped here and existing command lines keep working. Only the
	// first one: the target may need a "--" of its own.
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	return args[:target+1], rest
}

// nextPositional returns the index of the first token from start on that is
// neither a flag nor a flag's value, or -1 if there is none. A token after
// "--" is positional even if it starts with "-".
func nextPositional(args []string, start int) int {
	for i := start; i < len(args); i++ {
		switch {
		case args[i] == "--":
			if i+1 < len(args) {
				return i + 1
			}
			return -1
		case valueFlags[args[i]]:
			i++
		case !strings.HasPrefix(args[i], "-"):
			return i
		}
	}
	return -1
}
