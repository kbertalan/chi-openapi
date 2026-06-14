package serde

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const indentUnit = "  " // two spaces per nesting level

// Marshal renders v (a struct or pointer to struct) into an annotation block.
// Zero-valued fields are omitted (empty string, 0, false, nil pointer, empty
// slice). Structs are emitted as compact positional one-liners when every set
// field is a simple scalar, and as an indented named block otherwise.
func Marshal(v any) (string, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", fmt.Errorf("serde: Marshal of nil pointer")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", fmt.Errorf("serde: Marshal requires a struct, got %s", rv.Kind())
	}

	// Work on an addressable copy so codec fields with pointer-receiver
	// marshalers are reachable.
	cp := reflect.New(rv.Type()).Elem()
	cp.Set(rv)

	var b strings.Builder
	if err := marshalStruct(&b, infoFor(cp.Type()), cp, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// marshalStruct emits the non-empty fields of sv as directives at the given
// indentation level.
func marshalStruct(b *strings.Builder, info *structInfo, sv reflect.Value, indent int) error {
	for _, fd := range info.order {
		fv := sv.Field(fd.index)
		if isOmitted(fv) {
			continue
		}
		if err := marshalField(b, fd, fv, indent); err != nil {
			return err
		}
	}
	return nil
}

func marshalField(b *strings.Builder, fd *fieldDescriptor, fv reflect.Value, indent int) error {
	pad := strings.Repeat(indentUnit, indent)

	switch fd.kind {
	case kindStruct:
		return marshalStructDirective(b, fd, fv, indent)
	case kindPtrStruct:
		return marshalStructDirective(b, fd, fv.Elem(), indent)
	case kindSliceStruct:
		if fv.Len() == 0 {
			fmt.Fprintf(b, "%s@%s []\n", pad, fd.token)
			return nil
		}
		for k := 0; k < fv.Len(); k++ {
			ev := fv.Index(k)
			if ev.Kind() == reflect.Pointer {
				ev = ev.Elem()
			}
			if err := marshalStructDirective(b, fd, ev, indent); err != nil {
				return err
			}
		}
		return nil
	case kindSliceScalar:
		s, err := renderScalarSlice(fv, fd.elemKind)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s@%s %s\n", pad, fd.token, s)
		return nil
	case kindSliceSlice:
		return marshalSliceSlice(b, fd, fv, indent)
	case kindMap:
		return marshalMap(b, fd, fv, indent)
	default: // scalar / codec
		s, err := renderScalar(fv, fd.kind)
		if err != nil {
			return err
		}
		// Prefer a readable backtick block for multi-line values, but fall back
		// to a quoted single line (with \n escapes) when the value itself
		// contains a backtick, which a raw block cannot represent.
		if strings.Contains(s, "\n") && !strings.Contains(s, "`") {
			writeBacktick(b, pad, fd.token, s)
			return nil
		}
		fmt.Fprintf(b, "%s@%s %s\n", pad, fd.token, quoteIfNeeded(s))
		return nil
	}
}

// marshalMap emits a map field as one directive per entry: `@Field key: value`
// when the value fits on a line, or `@Field key:` followed by an indented block
// for a non-compact struct value. An empty map is `@Field []`. Entries are
// sorted by their rendered key for deterministic output.
func marshalMap(b *strings.Builder, fd *fieldDescriptor, fv reflect.Value, indent int) error {
	pad := strings.Repeat(indentUnit, indent)
	if fv.Len() == 0 {
		fmt.Fprintf(b, "%s@%s []\n", pad, fd.token)
		return nil
	}

	type entry struct {
		key, val reflect.Value
		sortKey  string
	}
	entries := make([]entry, 0, fv.Len())
	for _, k := range fv.MapKeys() {
		e := entry{key: k, val: fv.MapIndex(k)}
		if s, _, err := renderValueInline(fd.keyDesc, k); err == nil {
			e.sortKey = s
		} else {
			e.sortKey = fmt.Sprintf("%v", k.Interface())
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].sortKey < entries[j].sortKey })

	for _, e := range entries {
		keyStr, keyInline, err := renderValueInline(fd.keyDesc, e.key)
		if err != nil {
			return err
		}
		if !keyInline {
			return fmt.Errorf("serde: map key for %q cannot be rendered on one line", "@"+fd.token)
		}
		valStr, valInline, err := renderValueInline(fd.valDesc, e.val)
		if err != nil {
			return err
		}
		if valInline {
			if valStr == "" {
				fmt.Fprintf(b, "%s@%s %s:\n", pad, fd.token, keyStr)
			} else {
				fmt.Fprintf(b, "%s@%s %s: %s\n", pad, fd.token, keyStr, valStr)
			}
			continue
		}
		// Non-compact struct value: header then an indented block of its fields.
		fmt.Fprintf(b, "%s@%s %s:\n", pad, fd.token, keyStr)
		sv := e.val
		if sv.Kind() == reflect.Pointer {
			sv = sv.Elem()
		}
		if err := marshalStruct(b, fd.valDesc.elemInfo, sv, indent+1); err != nil {
			return err
		}
	}
	return nil
}

// marshalSliceSlice emits a [][]scalar field as one bracketed directive per inner
// slice (`@Field [a, b]`), or `@Field []` for an empty inner slice.
func marshalSliceSlice(b *strings.Builder, fd *fieldDescriptor, fv reflect.Value, indent int) error {
	pad := strings.Repeat(indentUnit, indent)
	for k := 0; k < fv.Len(); k++ {
		s, err := renderScalarSlice(fv.Index(k), fd.elemKind)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s@%s %s\n", pad, fd.token, s)
	}
	return nil
}

// renderScalarSlice renders a scalar slice as a bracketed CSV literal, e.g.
// `[a, "b c"]` or `[]` when empty.
func renderScalarSlice(fv reflect.Value, elemKind fieldKind) (string, error) {
	parts := make([]string, 0, fv.Len())
	for k := 0; k < fv.Len(); k++ {
		s, err := renderScalar(fv.Index(k), elemKind)
		if err != nil {
			return "", err
		}
		parts = append(parts, quoteIfNeeded(s))
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

// renderValueInline renders a map key or value to its one-line form. It returns
// (text, inline, err): inline is false only for a non-compact struct value,
// which the caller must emit as an indented block instead.
func renderValueInline(fd *fieldDescriptor, v reflect.Value) (string, bool, error) {
	switch fd.kind {
	case kindStruct, kindPtrStruct:
		sv := v
		if fd.kind == kindPtrStruct {
			if v.IsNil() {
				return "", true, nil
			}
			sv = v.Elem()
		}
		ok, last := canCompact(fd.elemInfo, sv)
		if !ok {
			return "", false, nil
		}
		if last < 0 {
			return "", true, nil
		}
		parts := make([]string, 0, last+1)
		for k := 0; k <= last; k++ {
			cfd := fd.elemInfo.order[k]
			s, err := renderScalar(sv.Field(cfd.index), cfd.kind)
			if err != nil {
				return "", false, err
			}
			parts = append(parts, quoteIfNeeded(s))
		}
		return strings.Join(parts, " "), true, nil
	case kindSliceScalar:
		s, err := renderScalarSlice(v, fd.elemKind)
		return s, true, err
	default:
		if !isScalarKind(fd.kind) {
			return "", false, fmt.Errorf("serde: unsupported map key/value kind for %q", "@"+fd.token)
		}
		s, err := renderScalar(v, fd.kind)
		return quoteIfNeeded(s), true, err
	}
}

// marshalStructDirective emits a single struct value, preferring a compact
// positional one-liner and falling back to a named block when the set fields are
// non-contiguous, non-scalar, or contain a newline.
func marshalStructDirective(b *strings.Builder, fd *fieldDescriptor, sv reflect.Value, indent int) error {
	pad := strings.Repeat(indentUnit, indent)

	if ok, last := canCompact(fd.elemInfo, sv); ok {
		if last < 0 {
			fmt.Fprintf(b, "%s@%s\n", pad, fd.token)
			return nil
		}
		parts := make([]string, 0, last+1)
		for k := 0; k <= last; k++ {
			cfd := fd.elemInfo.order[k]
			s, err := renderScalar(sv.Field(cfd.index), cfd.kind)
			if err != nil {
				return err
			}
			parts = append(parts, quoteIfNeeded(s))
		}
		fmt.Fprintf(b, "%s@%s %s\n", pad, fd.token, strings.Join(parts, " "))
		return nil
	}

	fmt.Fprintf(b, "%s@%s\n", pad, fd.token)
	return marshalStruct(b, fd.elemInfo, sv, indent+1)
}

// canCompact reports whether sv can be written as a positional one-liner and, if
// so, the index of the last field to emit. It requires the set fields to form a
// contiguous prefix of scalar values with no embedded newline.
func canCompact(info *structInfo, sv reflect.Value) (bool, int) {
	last := -1
	for k, fd := range info.order {
		if !isEmptyValue(sv.Field(fd.index)) {
			last = k
		}
	}
	for k := 0; k <= last; k++ {
		fd := info.order[k]
		fv := sv.Field(fd.index)
		if !isScalarKind(fd.kind) || isEmptyValue(fv) {
			return false, last
		}
		if fd.kind == kindString && strings.Contains(fv.String(), "\n") {
			return false, last
		}
	}
	return true, last
}

func renderScalar(fv reflect.Value, kind fieldKind) (string, error) {
	switch kind {
	case kindString:
		return fv.String(), nil
	case kindInt:
		return strconv.FormatInt(fv.Int(), 10), nil
	case kindUint:
		return strconv.FormatUint(fv.Uint(), 10), nil
	case kindFloat:
		return strconv.FormatFloat(fv.Float(), 'g', -1, 64), nil
	case kindBool:
		return strconv.FormatBool(fv.Bool()), nil
	case kindTextCodec:
		return renderTextCodec(fv)
	case kindJSONCodec:
		return renderJSONCodec(fv)
	default:
		return "", fmt.Errorf("serde: cannot render field kind")
	}
}

func renderTextCodec(fv reflect.Value) (string, error) {
	m := asTextMarshaler(fv)
	if m == nil {
		return "", fmt.Errorf("serde: type %s implements TextUnmarshaler but not TextMarshaler; cannot Marshal", fv.Type())
	}
	b, err := m.MarshalText()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func renderJSONCodec(fv reflect.Value) (string, error) {
	m := asJSONMarshaler(fv)
	if m == nil {
		return "", fmt.Errorf("serde: type %s implements json.Unmarshaler but not json.Marshaler; cannot Marshal", fv.Type())
	}
	b, err := m.MarshalJSON()
	if err != nil {
		return "", err
	}
	// Unwrap JSON strings so the value is emitted without surrounding quotes.
	if len(b) >= 2 && b[0] == '"' {
		var s string
		if json.Unmarshal(b, &s) == nil {
			return s, nil
		}
	}
	return string(b), nil
}

func asTextMarshaler(fv reflect.Value) encoding.TextMarshaler {
	if m, ok := fv.Interface().(encoding.TextMarshaler); ok {
		return m
	}
	if fv.CanAddr() {
		if m, ok := fv.Addr().Interface().(encoding.TextMarshaler); ok {
			return m
		}
	}
	return nil
}

func asJSONMarshaler(fv reflect.Value) json.Marshaler {
	if m, ok := fv.Interface().(json.Marshaler); ok {
		return m
	}
	if fv.CanAddr() {
		if m, ok := fv.Addr().Interface().(json.Marshaler); ok {
			return m
		}
	}
	return nil
}

// quoteIfNeeded double-quotes s when it is empty or contains characters that
// would otherwise break tokenization (whitespace, commas, quotes, backslashes),
// escaping backslashes, quotes, newlines and tabs inside the quotes.
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\r\n,\"\\") {
		return s
	}
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\t", `\t`,
		"\r", `\r`,
	)
	return `"` + r.Replace(s) + `"`
}

// writeBacktick emits a multi-line string value as an indented backtick block.
func writeBacktick(b *strings.Builder, pad, token, value string) {
	fmt.Fprintf(b, "%s@%s `\n", pad, token)
	inner := pad + indentUnit
	for line := range strings.SplitSeq(value, "\n") {
		b.WriteString(inner)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s`\n", pad)
}

// isOmitted reports whether a field is skipped by Marshal. Nil pointers,
// interfaces and slices are omitted; a non-nil empty slice is NOT omitted (it is
// emitted as `[]`). Other types are omitted when zero.
func isOmitted(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

// isEmptyValue reports whether v is the zero value of its type, treating empty
// (but non-nil) slices and maps as empty too. Used to detect holes when deciding
// whether a struct can be rendered as a compact positional one-liner.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
