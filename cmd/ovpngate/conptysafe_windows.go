//go:build windows

package main

import (
	"os"
	"strconv"
)

type conptySafeOutput struct {
	*os.File
	row int
}

func (o *conptySafeOutput) Write(p []byte) (int, error) {
	out := o.rewrite(p)
	if _, err := o.File.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (o *conptySafeOutput) WriteString(s string) (int, error) {
	return o.Write([]byte(s))
}

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
