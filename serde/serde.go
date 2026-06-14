// Package serde decodes and encodes Go structs to and from the line-oriented
// annotation DSL used in doc comments (e.g. "@Summary", "@Tags", "@Success").
//
// A directive "@Token args" maps to a struct field whose token is taken from
// the `tag:"Token"` struct tag, falling back to the exact Go field name.
//
// The grammar supports:
//   - scalar fields (quoted or unquoted), incl. backtick-delimited multi-line strings;
//   - slices: comma-separated or repeated, with optional surrounding brackets
//     (`@Tags a,b` ≡ `@Tags [a,b]`); an empty slice is the explicit literal `[]`;
//   - structs: positional one-liners, indented name-bound blocks, or a mix
//     (inline positional values overridden by an indented block);
//   - slices of scalar slices: one bracketed value per directive (`@Matrix [a,b]`);
//   - maps: one entry per directive, `@Field key: value`, where the value part may
//     be inline or an indented block.
//
// Indentation is significant and strict: a directive deeper than its block's base
// indent (without a container directive having opened a child block) is an error.
// A second directive for a single-value (scalar/struct) field is a duplicate error;
// slices and maps are additive.
package serde

import (
	"fmt"
	"reflect"
	"strings"
)

// ParseError accumulates the per-directive failures encountered during Unmarshal
// (malformed values, unknown tokens). Decoding is best-effort: the destination
// is still populated with everything that parsed.
type ParseError struct {
	Messages []string
}

func (e *ParseError) Error() string {
	return "serde: " + strings.Join(e.Messages, "; ")
}

func (e *ParseError) add(msg string) {
	e.Messages = append(e.Messages, msg)
}

// Unmarshal decodes a doc-comment annotation block into the struct pointed to by
// v. doc is the comment text as produced by go/ast *CommentGroup.Text(): the
// "// " markers are stripped but content indentation is preserved. v must be a
// non-nil pointer to a struct.
func Unmarshal(doc string, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("serde: Unmarshal requires a non-nil pointer to a struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("serde: Unmarshal requires a pointer to a struct, got %s", rv.Kind())
	}

	lines := strings.Split(doc, "\n")
	pe := &ParseError{}
	info := infoFor(rv.Type())
	// Process top-level directives in runs; each run derives its own base indent
	// from its first directive, so an inconsistently-indented later run is parsed
	// rather than silently dropped.
	for i := 0; i < len(lines); {
		next := parseBlock(lines, i, info, rv, -1, pe)
		if next <= i {
			i++
		} else {
			i = next
		}
	}
	if len(pe.Messages) > 0 {
		return pe
	}
	return nil
}

// parseBlock processes the directives belonging to a single struct value dst.
// baseIndent is the indentation shared by this block's directives (-1 means it
// is taken from the first directive). It returns the index of the first line
// that belongs to an outer block (or len(lines) at EOF).
//
// Indentation is strict: a directive deeper than the base (without a container
// directive having opened a child block for it) is an error.
func parseBlock(lines []string, start int, info *structInfo, dst reflect.Value, baseIndent int, pe *ParseError) int {
	seen := make(map[string]bool)
	i := start
	for i < len(lines) {
		content := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimLeft(content, " \t")
		if !strings.HasPrefix(trimmed, "@") {
			i++ // blank line or free-form prose
			continue
		}

		ind := indentOf(content)
		if baseIndent < 0 {
			baseIndent = ind
		}
		if ind < baseIndent {
			return i // belongs to an outer block
		}
		if ind > baseIndent {
			pe.add(fmt.Sprintf("unexpected indentation at %q", trimmed))
			i = skipBlockBody(lines, i, ind)
			continue
		}

		token, rest := splitToken(trimmed)
		fd, ok := info.byToken[token]
		if !ok {
			pe.add(fmt.Sprintf("unknown token %q", "@"+token))
			i = skipBlockBody(lines, i, ind)
			continue
		}
		if isSingleValue(fd.kind) {
			if seen[token] {
				pe.add(fmt.Sprintf("duplicate directive %q", "@"+token))
				i = skipBlockBody(lines, i, ind)
				continue
			}
			seen[token] = true
		}
		i = applyDirective(lines, i, ind, rest, fd, dst.Field(fd.index), pe)
	}
	return i
}

// isSingleValue reports whether a field holds at most one value, so a second
// directive for it is a duplicate. Slices and maps are additive instead.
func isSingleValue(k fieldKind) bool {
	switch k {
	case kindSliceScalar, kindSliceStruct, kindSliceSlice, kindMap:
		return false
	}
	return true
}

// applyDirective binds one directive (and, for struct/map fields, any indented
// block that follows it) into target — the destination value for this field
// (a struct field, or a map key/value). It returns the next line index.
func applyDirective(lines []string, i, ind int, rest string, fd *fieldDescriptor, target reflect.Value, pe *ParseError) int {
	switch fd.kind {
	case kindSliceScalar:
		content, empty, err := sliceContent(rest)
		if err != nil {
			pe.add(fmt.Sprintf("field %q: %v", "@"+fd.token, err))
			return i + 1
		}
		if empty {
			ensureNonNilSlice(target)
			return i + 1
		}
		if err := appendSliceScalar(target, fd, content, fd.token); err != nil {
			pe.add(err.Error())
		}
		return i + 1

	case kindSliceSlice:
		return applyInnerSlice(rest, fd, target, i, pe)

	case kindMap:
		return applyMapEntry(lines, i, ind, rest, fd, target, pe)

	case kindStruct, kindPtrStruct:
		sv := structTarget(target, fd)
		return bindStructValue(lines, i, ind, rest, fd, sv, pe)

	case kindSliceStruct:
		if strings.TrimSpace(rest) == "[]" {
			ensureNonNilSlice(target)
			return i + 1
		}
		st := fd.elemType
		isPtr := st.Kind() == reflect.Pointer
		if isPtr {
			st = st.Elem()
		}
		ev := reflect.New(st)
		next := bindStructValue(lines, i, ind, rest, fd, ev.Elem(), pe)
		if isPtr {
			target.Set(reflect.Append(target, ev))
		} else {
			target.Set(reflect.Append(target, ev.Elem()))
		}
		return next

	default: // scalar / codec
		if strings.HasPrefix(rest, "`") {
			val, last, err := captureBacktick(rest[1:], lines, i)
			if err != nil {
				pe.add(fmt.Sprintf("field %q: %v", "@"+fd.token, err))
				return last + 1
			}
			if err := setScalar(target, fd.kind, val, fd.token); err != nil {
				pe.add(err.Error())
			}
			return last + 1
		}
		val, err := parseScalarLine(rest)
		if err != nil {
			pe.add(fmt.Sprintf("field %q: %v", "@"+fd.token, err))
			return i + 1
		}
		if err := setScalar(target, fd.kind, val, fd.token); err != nil {
			pe.add(err.Error())
		}
		return i + 1
	}
}

// structTarget returns the addressable struct value to populate for a struct or
// *struct field, allocating the pointer on first use.
func structTarget(field reflect.Value, fd *fieldDescriptor) reflect.Value {
	if fd.kind == kindPtrStruct {
		if field.IsNil() {
			field.Set(reflect.New(fd.elemType))
		}
		return field.Elem()
	}
	return field
}

// bindStructValue binds a struct directive into sv: inline positional args first
// (when present), then an indented block of named fields that overrides them.
func bindStructValue(lines []string, i, ind int, rest string, fd *fieldDescriptor, sv reflect.Value, pe *ParseError) int {
	if strings.TrimSpace(rest) != "" {
		args, err := splitArgs(rest)
		if err != nil {
			pe.add(fmt.Sprintf("field %q: %v", "@"+fd.token, err))
		} else if err := bindPositional(sv, fd.elemInfo, args, fd.token); err != nil {
			pe.add(err.Error())
		}
	}

	if idx, childInd := peekDirective(lines, i+1); idx >= 0 && childInd > ind {
		return parseBlock(lines, i+1, fd.elemInfo, sv, childInd, pe)
	}
	return i + 1
}

// peekDirective returns the index and indentation of the next directive line at
// or after `from`, skipping blank and prose lines. It returns (-1, -1) at EOF.
func peekDirective(lines []string, from int) (idx, indent int) {
	for j := from; j < len(lines); j++ {
		content := strings.TrimRight(lines[j], " \t")
		if !strings.HasPrefix(strings.TrimLeft(content, " \t"), "@") {
			continue
		}
		return j, indentOf(content)
	}
	return -1, -1
}

// ensureNonNilSlice makes a slice field a non-nil empty slice when it is nil,
// implementing the `[]` empty-slice literal without discarding earlier elements.
func ensureNonNilSlice(field reflect.Value) {
	if field.IsNil() {
		field.Set(reflect.MakeSlice(field.Type(), 0, 0))
	}
}

// sliceContent normalises a one-line slice value: it requires a non-empty rest
// (a bare directive is an error), strips an optional surrounding `[ ]`, and
// reports whether the result is the empty slice.
func sliceContent(rest string) (content string, empty bool, err error) {
	t := strings.TrimSpace(rest)
	if t == "" {
		return "", false, fmt.Errorf("expected values or [] for a slice")
	}
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		t = strings.TrimSpace(t[1 : len(t)-1])
	}
	return t, t == "", nil
}

// applyInnerSlice handles one directive of a [][]scalar field: it builds a single
// inner slice from a bracketed/CSV one-liner (or `[]`) and appends it to target.
func applyInnerSlice(rest string, fd *fieldDescriptor, target reflect.Value, i int, pe *ParseError) int {
	content, empty, err := sliceContent(rest)
	if err != nil {
		pe.add(fmt.Sprintf("field %q: %v", "@"+fd.token, err))
		return i + 1
	}
	inner := reflect.MakeSlice(fd.elemType, 0, 0)
	if !empty {
		inner, err = appendCSVElements(inner, fd, content)
		if err != nil {
			pe.add(err.Error())
			return i + 1
		}
	}
	target.Set(reflect.Append(target, inner))
	return i + 1
}

// applyMapEntry handles one map directive: `@Field []` yields (or keeps) an empty
// non-nil map; otherwise `@Field key: value` inserts one entry. The key binds
// inline; the value binds via the full directive grammar (inline or block).
func applyMapEntry(lines []string, i, ind int, rest string, fd *fieldDescriptor, target reflect.Value, pe *ParseError) int {
	if target.IsNil() {
		target.Set(reflect.MakeMap(target.Type()))
	}

	t := strings.TrimSpace(rest)
	if t == "" {
		pe.add(fmt.Sprintf("field %q: expected \"key: value\" or []", "@"+fd.token))
		return i + 1
	}
	if t == "[]" {
		return i + 1
	}

	keyPart, valPart, ok := splitMapEntry(rest)
	if !ok {
		pe.add(fmt.Sprintf("field %q: map entry requires \"key: value\"", "@"+fd.token))
		return i + 1
	}

	key := reflect.New(target.Type().Key()).Elem()
	val := reflect.New(target.Type().Elem()).Elem()
	if err := bindInline(fd.keyDesc, keyPart, key); err != nil {
		pe.add(err.Error())
	}
	next := applyDirective(lines, i, ind, valPart, fd.valDesc, val, pe)
	target.SetMapIndex(key, val)
	return next
}

// bindInline binds a single-line value into target according to fd's kind, with
// no following block (used for map keys). Scalars/codecs are parsed directly;
// structs bind positionally; scalar slices accept a bracketed/CSV value.
func bindInline(fd *fieldDescriptor, rest string, target reflect.Value) error {
	switch fd.kind {
	case kindStruct, kindPtrStruct:
		sv := target
		if fd.kind == kindPtrStruct {
			target.Set(reflect.New(fd.elemType))
			sv = target.Elem()
		}
		args, err := splitArgs(rest)
		if err != nil {
			return fmt.Errorf("field %q: %v", "@"+fd.token, err)
		}
		return bindPositional(sv, fd.elemInfo, args, fd.token)
	case kindSliceScalar:
		content, empty, err := sliceContent(rest)
		if err != nil {
			return fmt.Errorf("field %q: %v", "@"+fd.token, err)
		}
		ensureNonNilSlice(target)
		if !empty {
			return appendSliceScalar(target, fd, content, fd.token)
		}
		return nil
	default:
		if !isScalarKind(fd.kind) {
			return fmt.Errorf("field %q: cannot use this type inline (as a map key)", "@"+fd.token)
		}
		val, err := parseScalarLine(rest)
		if err != nil {
			return fmt.Errorf("field %q: %v", "@"+fd.token, err)
		}
		return setScalar(target, fd.kind, val, fd.token)
	}
}

// splitMapEntry splits a map directive's argument on the first unquoted ':' into
// the key and value parts (both trimmed).
func splitMapEntry(s string) (key, val string, ok bool) {
	inQuote, escaped := false, false
	for i, r := range s {
		switch {
		case inQuote:
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inQuote = false
			}
		case r == '"':
			inQuote = true
		case r == ':':
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
		}
	}
	return "", "", false
}

// appendCSVElements parses a comma-separated value and returns inner with each
// element appended, using the inner scalar kind.
func appendCSVElements(inner reflect.Value, fd *fieldDescriptor, rest string) (reflect.Value, error) {
	elems, err := splitCSV(rest)
	if err != nil {
		return inner, fmt.Errorf("field %q: %v", "@"+fd.token, err)
	}
	for _, e := range elems {
		ev := reflect.New(fd.elemType.Elem()).Elem()
		if err := setScalar(ev, fd.elemKind, e, fd.token); err != nil {
			return inner, err
		}
		inner = reflect.Append(inner, ev)
	}
	return inner, nil
}

// skipBlockBody advances past an unknown directive at line i and any indented
// body lines that belong to it.
func skipBlockBody(lines []string, i, ind int) int {
	j := i + 1
	for j < len(lines) {
		content := strings.TrimRight(lines[j], " \t")
		if !strings.HasPrefix(strings.TrimLeft(content, " \t"), "@") {
			j++
			continue
		}
		if indentOf(content) <= ind {
			break
		}
		j++
	}
	return j
}
