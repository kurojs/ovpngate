//go:build windows

package main

import (
	"os"
	"strconv"
)

// conptySafeOutput sits between the Bubble Tea renderer and the real
// console.  It rewrites the frame flush pattern — "cursor home followed by
// newline-separated rows" — into per-row absolute cursor positioning
// (CUP + erase-to-EOL), the exact painting pattern tcell/lazygit use.
//
// Why: ConPTY hosts (WezTerm, Windows Terminal, cmd.exe, classic
// PowerShell) double-advance the cursor when a TUI flushes frames as a
// home sequence plus newlines, so every row renders twice (row + blank).
// Absolute per-row positioning renders identically in every backend,
// including herdr, which parses the same ANSI stream directly.
//
// It embeds *os.File so Bubble Tea's Windows tty detection
// (p.output.(term.File) + Fd()) still sees a real console and enables
// VT/resize exactly as before; only the byte stream is rewritten.
type conptySafeOutput struct {
	*os.File
	row int // last known cursor row, 1-based
}

// Write transforms one renderer buffer and forwards the result.
func (o *conptySafeOutput) Write(p []byte) (int, error) {
	out := o.rewrite(p)
	if _, err := o.File.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteString takes the string fast path through the same rewrite.  The
// embedded *os.File would otherwise promote its own WriteString and
// Bubble Tea's renderer uses io.WriteString for terminal control
// sequences (hide cursor, alt screen enter/exit, bracketed paste, ...).
func (o *conptySafeOutput) WriteString(s string) (int, error) {
	return o.Write([]byte(s))
}

// rewrite transforms one buffer of renderer bytes.  Every non-line byte
// passes through byte-identical; a CRLF or LF becomes "erase to end of
// line" (ESC[K) followed by an absolute cursor position (CUP) to the next
// row.  No byte ever advances the cursor through raw terminal newline
// processing.
func (o *conptySafeOutput) rewrite(p []byte) []byte {
	out := make([]byte, 0, len(p)+len(p)/4)
	row := o.row
	i := 0
	for i < len(p) {
		b := p[i]
		switch {
		case b == 0x1b:
			j := escEnd(p, i)
			seq := p[i:j]
			if r, ok := cupRow(seq); ok {
				if r == 1 {
					// Bubble Tea begins every painted frame with a cursor
					// home.  Anchor the row counter at the visible top so
					// legacy consoles that report the scrollback window
					// (instead of the visible one) can never shift the
					// header off the top or the footer below the bottom.
					row = 1
				} else {
					row = r
				}
			}
			out = append(out, seq...)
			i = j
		case b == '\r':
			if i+1 < len(p) && p[i+1] == '\n' {
				row++
				out = append(out, "\x1b[K\x1b["...)
				out = strconv.AppendInt(out, int64(row), 10)
				out = append(out, ";1H"...)
				i += 2
			} else {
				// Bare CR: return to column 1 on the same row.
				out = append(out, "\x1b[1G"...)
				i++
			}
		case b == '\n':
			row++
			out = append(out, "\x1b[K\x1b["...)
			out = strconv.AppendInt(out, int64(row), 10)
			out = append(out, ";1H"...)
			i++
		default:
			out = append(out, b)
			i++
		}
	}
	o.row = row
	return out
}

// escEnd returns the index just past the escape sequence starting at i.
// CSI sequences end at the first final byte (0x40-0x7E); OSC sequences
// (rare in the renderer) end at BEL.  A sequence truncated at the buffer
// end passes through verbatim as-is.
func escEnd(p []byte, i int) int {
	j := i + 1
	for j < len(p) {
		c := p[j]
		if c == '\a' {
			return j + 1
		}
		if c >= 0x40 && c <= 0x7e {
			return j + 1
		}
		j++
	}
	return len(p)
}

// cupRow extracts the row of a cursor-position sequence (ESC[...H),
// 1-based; bare ESC[H (CursorHome) is row 1.
func cupRow(seq []byte) (int, bool) {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != '[' || seq[len(seq)-1] != 'H' {
		return 0, false
	}
	params := seq[2 : len(seq)-1]
	if len(params) == 0 {
		return 1, true
	}
	rowB := params
	for k, c := range params {
		if c == ';' {
			rowB = params[:k]
			break
		}
	}
	r, err := strconv.Atoi(string(rowB))
	if err != nil || r < 1 {
		return 0, false
	}
	return r, true
}