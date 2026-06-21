package serde

import (
	"fmt"
	"strings"
)

// indentOf returns the number of leading space/tab characters in s. Indentation
// is significant in the annotation DSL (it delimits nested struct blocks), so
// these characters are ASCII and the count equals the byte offset of the first
// non-whitespace rune.
func indentOf(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' && r != '\t' {
			break
		}
		n++
	}
	return n
}

// splitToken splits a trimmed directive line ("@Token rest...") into its token
// name and the remaining argument text (left-trimmed, may be empty).
func splitToken(s string) (token, rest string) {
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimLeft(s[i:], " \t")
	}
	return s, ""
}

// mapEscape returns the literal rune for the character following a backslash
// inside a quoted span. Recognised escapes are \\, \", \n, \t and \r; any other
// \x yields x verbatim.
func mapEscape(r rune) rune {
	switch r {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	default:
		return r
	}
}

// parseScalarLine resolves a single-line scalar value: a fully double-quoted
// remainder is unquoted and unescaped, otherwise the trimmed remainder is
// returned verbatim (so both `Hello` and `this is unquoted` work).
func parseScalarLine(s string) (string, error) {
	t := strings.TrimSpace(s)
	if t == "" || t[0] != '"' {
		return t, nil
	}
	var b strings.Builder
	body := t[1:]
	escaped := false
	for i, r := range body {
		if escaped {
			b.WriteRune(mapEscape(r))
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"': // closing quote; nothing meaningful may follow
			if strings.TrimSpace(body[i+1:]) != "" {
				return "", fmt.Errorf("unexpected text after quoted value")
			}
			return b.String(), nil
		default:
			b.WriteRune(r)
		}
	}
	return "", fmt.Errorf("unterminated quoted string")
}

// splitArgs tokenizes a line into whitespace-separated positional arguments.
// A double-quoted span keeps spaces within a single argument; the surrounding
// quotes are removed and backslash escapes interpreted.
func splitArgs(s string) ([]string, error) {
	var (
		args     []string
		cur      strings.Builder
		inQuote  bool
		escaped  bool
		hasToken bool
	)
	flush := func() {
		if hasToken {
			args = append(args, cur.String())
			cur.Reset()
			hasToken = false
		}
	}
	for _, r := range s {
		switch {
		case inQuote:
			switch {
			case escaped:
				cur.WriteRune(mapEscape(r))
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuote = false
			default:
				cur.WriteRune(r)
			}
		case r == '"':
			inQuote = true
			hasToken = true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
			hasToken = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

// splitCSV tokenizes a line into comma-separated elements. Commas and spaces
// inside double quotes are literal; backslash escapes are interpreted and each
// element is space-trimmed.
func splitCSV(s string) ([]string, error) {
	var (
		elems   []string
		cur     strings.Builder
		inQuote bool
		escaped bool
	)
	for _, r := range s {
		switch {
		case inQuote:
			switch {
			case escaped:
				cur.WriteRune(mapEscape(r))
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuote = false
			default:
				cur.WriteRune(r)
			}
		case r == '"':
			inQuote = true
		case r == ',':
			elems = append(elems, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	elems = append(elems, strings.TrimSpace(cur.String()))
	return elems, nil
}

// captureBacktick collects a backtick-delimited multi-line string. after is the
// opening-line content following the leading backtick; openIdx is that line's
// index in lines. It returns the dedented value and the index of the line
// holding the closing backtick (so the caller resumes at last+1).
func captureBacktick(after string, lines []string, openIdx int) (value string, last int, err error) {
	if idx := strings.IndexByte(after, '`'); idx >= 0 {
		return after[:idx], openIdx, nil // single line: `text`
	}

	var content []string
	if strings.TrimSpace(after) != "" {
		content = append(content, after)
	}
	for j := openIdx + 1; j < len(lines); j++ {
		line := lines[j]
		if idx := strings.IndexByte(line, '`'); idx >= 0 {
			if before := line[:idx]; strings.TrimSpace(before) != "" {
				content = append(content, before)
			}
			return dedent(content), j, nil
		}
		content = append(content, line)
	}
	return "", openIdx, fmt.Errorf("unterminated backtick string")
}

// dedent removes the common minimum leading indentation from a block of lines
// and drops fully-blank leading and trailing lines.
func dedent(lines []string) string {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if ind := indentOf(l); min < 0 || ind < min {
			min = ind
		}
	}
	if min < 0 {
		return ""
	}

	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	out := make([]string, 0, end-start)
	for _, l := range lines[start:end] {
		if len(l) >= min {
			out = append(out, l[min:])
		} else {
			out = append(out, strings.TrimLeft(l, " \t"))
		}
	}
	return strings.Join(out, "\n")
}
