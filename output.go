// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
}

// printJSON prints a value as indented JSON, used for --json output.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printRawJSON pretty-prints an already-encoded JSON body.
func printRawJSON(raw []byte) {
	fmt.Println(prettyJSON(raw))
}

// printfErr reports progress on stderr. Quiet mode silences it; errors are
// reported by main regardless.
func (g *globals) printfErr(format string, a ...any) {
	if g.quiet {
		return
	}
	out := g.errOut
	if out == nil {
		out = os.Stderr
	}
	fmt.Fprintf(out, format, a...)
}
