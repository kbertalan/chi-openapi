package serde

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// --- fixtures ---

type color string // enum-like named type, used by value

const (
	colorGreen color = "green"
)

// shortID is a custom codec backed by a string (Text + JSON).
type shortID string

func (s *shortID) UnmarshalText(b []byte) error { *s = shortID(b); return nil }
func (s shortID) MarshalText() ([]byte, error)  { return []byte(s), nil }

// jsonNum is a JSON codec backed by a number (no Text interfaces).
type jsonNum int

func (n *jsonNum) UnmarshalJSON(b []byte) error {
	var v int
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = jsonNum(v)
	return nil
}
func (n jsonNum) MarshalJSON() ([]byte, error) { return json.Marshal(int(n)) }

// jsonStr is a JSON codec backed by a string (no Text interfaces).
type jsonStr string

func (s *jsonStr) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = jsonStr(v)
	return nil
}
func (s jsonStr) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

type inner struct {
	Code int `tag:"Code"`
	Name string
	Note string
	Flag bool
}

type doc struct {
	Summary     string
	Description string
	Tags        []string
	Items       []string   `tag:"Item"`
	Nested      *inner     `tag:"Nested"`
	Embedded    inner      `tag:"Embed"`
	Params      []inner    `tag:"Param"`
	Color       color      `tag:"Color"`
	ID          shortID    `tag:"ID"`
	Num         jsonNum    `tag:"Num"`
	Str         jsonStr    `tag:"Str"`
	Matrix      [][]string `tag:"Row"`
}

// fixtures for map support
type withMaps struct {
	Scalars map[string]int   `tag:"Scalars"`
	Structs map[string]inner `tag:"Structs"`
	Keyed   map[inner]string `tag:"Keyed"`
}

// readOnlyID implements TextUnmarshaler but not TextMarshaler, so it can be
// decoded but not encoded.
type readOnlyID string

func (s *readOnlyID) UnmarshalText(b []byte) error { *s = readOnlyID(b); return nil }

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want doc
	}{
		{
			name: "scalar single word",
			in:   "@Summary Hello",
			want: doc{Summary: "Hello"},
		},
		{
			name: "scalar quoted multi word",
			in:   `@Summary "this is the summary"`,
			want: doc{Summary: "this is the summary"},
		},
		{
			name: "scalar unquoted multi word",
			in:   "@Description Test description",
			want: doc{Description: "Test description"},
		},
		{
			name: "slice one line csv",
			in:   "@Tags one,two,three",
			want: doc{Tags: []string{"one", "two", "three"}},
		},
		{
			name: "slice quoted elements",
			in:   `@Tags "Something longer", "Another tag"`,
			want: doc{Tags: []string{"Something longer", "Another tag"}},
		},
		{
			name: "slice quoted comma inside",
			in:   `@Tags "a,b", c`,
			want: doc{Tags: []string{"a,b", "c"}},
		},
		{
			name: "slice multi line append",
			in:   "@Tags one\n@Tags two",
			want: doc{Tags: []string{"one", "two"}},
		},
		{
			name: "struct positional partial",
			in:   "@Nested 200 model.Response",
			want: doc{Nested: &inner{Code: 200, Name: "model.Response"}},
		},
		{
			name: "pointer nil when absent",
			in:   "@Summary x",
			want: doc{Summary: "x"},
		},
		{
			name: "slice of struct positional",
			in:   "@Param 1 a\n@Param 2 b",
			want: doc{Params: []inner{{Code: 1, Name: "a"}, {Code: 2, Name: "b"}}},
		},
		{
			name: "struct block binds by name",
			in:   "@Nested\n  @Code 5\n  @Name resp",
			want: doc{Nested: &inner{Code: 5, Name: "resp"}},
		},
		{
			name: "block then trailing outer directive",
			in:   "@Nested\n  @Code 5\n@Summary after",
			want: doc{Summary: "after", Nested: &inner{Code: 5}},
		},
		{
			name: "mixed positional then block override",
			in:   "@Nested 1 a\n  @Code 2",
			want: doc{Nested: &inner{Code: 2, Name: "a"}},
		},
		{
			name: "mixed positional then block adds field",
			in:   `@Nested 200 model.Response` + "\n" + `  @Note "This is a longer description"`,
			want: doc{Nested: &inner{Code: 200, Name: "model.Response", Note: "This is a longer description"}},
		},
		{
			name: "enum by value",
			in:   "@Color green",
			want: doc{Color: colorGreen},
		},
		{
			name: "text codec unquoted",
			in:   "@ID 550e8400-uuid",
			want: doc{ID: shortID("550e8400-uuid")},
		},
		{
			name: "json codec number passthrough",
			in:   "@Num 42",
			want: doc{Num: jsonNum(42)},
		},
		{
			name: "json codec string wrapped",
			in:   "@Str hello",
			want: doc{Str: jsonStr("hello")},
		},
		{
			name: "blank lines and prose ignored",
			in:   "This is a human description.\n\n@Summary x\n\nmore prose\n@Tags a",
			want: doc{Summary: "x", Tags: []string{"a"}},
		},
		{
			name: "empty doc",
			in:   "",
			want: doc{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got doc
			if err := Unmarshal(tc.in, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// TestPositionalScalarSlice covers a struct whose trailing positional field is a
// scalar slice: the single arg is parsed as a comma-separated list, while
// omitting it leaves the slice nil. This backs the "@Security scheme scopes" form.
func TestPositionalScalarSlice(t *testing.T) {
	type secReq struct {
		Scheme string
		Scopes []string
	}
	type secDoc struct {
		Sec []secReq `tag:"Sec"`
	}

	tests := []struct {
		name string
		in   string
		want secDoc
	}{
		{
			name: "scheme only leaves scopes nil",
			in:   "@Sec ApiKey",
			want: secDoc{Sec: []secReq{{Scheme: "ApiKey"}}},
		},
		{
			name: "scheme with csv scopes",
			in:   "@Sec OAuth2 read,write",
			want: secDoc{Sec: []secReq{{Scheme: "OAuth2", Scopes: []string{"read", "write"}}}},
		},
		{
			name: "multiple directives accumulate",
			in:   "@Sec ApiKey\n@Sec OAuth2 read,write",
			want: secDoc{Sec: []secReq{{Scheme: "ApiKey"}, {Scheme: "OAuth2", Scopes: []string{"read", "write"}}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got secDoc
			if err := Unmarshal(tc.in, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestUnmarshalBacktickMultiline(t *testing.T) {
	in := "@Embed\n" +
		"  @Code 7\n" +
		"  @Note `\n" +
		"    line one\n" +
		"      indented\n" +
		"    line two\n" +
		"  `\n"

	var got doc
	if err := Unmarshal(in, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	wantNote := "line one\n  indented\nline two"
	if got.Embedded.Code != 7 || got.Embedded.Note != wantNote {
		t.Errorf("got %+v, want Code=7 Note=%q", got.Embedded, wantNote)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		substr  string
		wantLen int
	}{
		{"unknown token", "@Bogus x", `unknown token "@Bogus"`, 1},
		{"invalid integer", "@Nested notnum", "invalid integer", 1},
		{"invalid bool", "@Embed\n  @Flag maybe", "invalid bool", 1},
		{"unterminated quote", `@Summary "open`, "unterminated quoted string", 1},
		{"multiple errors", "@Nested notnum\n@Bogus y", "", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got doc
			err := Unmarshal(tc.in, &got)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected *ParseError, got %T", err)
			}
			if len(pe.Messages) != tc.wantLen {
				t.Errorf("expected %d messages, got %d: %v", tc.wantLen, len(pe.Messages), pe.Messages)
			}
			if tc.substr != "" && !strings.Contains(err.Error(), tc.substr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.substr)
			}
		})
	}
}

func TestUnknownTokenIsHardErrorButOthersParse(t *testing.T) {
	var got doc
	err := Unmarshal("@Bogus x\n@Summary ok", &got)
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
	if got.Summary != "ok" {
		t.Errorf("expected Summary parsed despite unknown token, got %q", got.Summary)
	}
}

// name fallback: without a tag, the token must equal the Go field name and is
// case-sensitive.
type untagged struct {
	Summary    string
	Parameters []inner
}

func TestNameFallback(t *testing.T) {
	var u untagged
	if err := Unmarshal("@Summary hi", &u); err != nil {
		t.Fatalf("field-name token should work: %v", err)
	}
	if u.Summary != "hi" {
		t.Errorf("got %q", u.Summary)
	}

	// @Param does not match field name Parameters (only the tagged doc accepts it).
	if err := Unmarshal("@Param 1 a", &untagged{}); err == nil {
		t.Error("expected unknown-token error for @Param on untagged struct")
	}
	// lowercase token is unknown (case-sensitive).
	if err := Unmarshal("@summary hi", &untagged{}); err == nil {
		t.Error("expected unknown-token error for lowercase @summary")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	in := doc{
		Summary:     "Get",
		Description: "this is longer",
		Tags:        []string{"a", "b c"},
		Nested:      &inner{Code: 200, Name: "resp"},
		Params:      []inner{{Code: 1, Name: "p1"}, {Code: 2, Name: "p2"}},
		Color:       colorGreen,
		ID:          shortID("abc-123"),
		Num:         jsonNum(9),
		Str:         jsonStr("text"),
	}

	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got doc
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n--- marshaled ---\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip mismatch\nmarshaled:\n%s\ngot  %+v\nwant %+v", out, got, in)
	}
}

func TestMarshalBlockFallback(t *testing.T) {
	// A non-contiguous set of fields (Code zero, Note set) cannot be a positional
	// one-liner, so it must fall back to a named block.
	in := doc{Nested: &inner{Name: "n", Note: "multi\nline"}}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if !strings.Contains(out, "@Nested\n") || !strings.Contains(out, "@Note `") {
		t.Errorf("expected block form with backtick, got:\n%s", out)
	}

	var got doc
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip mismatch\n%s\ngot %+v\nwant %+v", out, got, in)
	}
}

type withPtrScalars struct {
	Summary string
	Flag    *bool   `tag:"Flag"`
	Count   *int    `tag:"Count"`
	Note    *string `tag:"Note"`
	Ratio   *float64 `tag:"Ratio"`
}

func TestPtrScalarUnmarshalAndOmit(t *testing.T) {
	var got withPtrScalars
	if err := Unmarshal("@Summary s\n@Flag false\n@Count 3\n@Note hello", &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Flag == nil || *got.Flag != false {
		t.Errorf("expected Flag=*false, got %v", got.Flag)
	}
	if got.Count == nil || *got.Count != 3 {
		t.Errorf("expected Count=*3, got %v", got.Count)
	}
	if got.Note == nil || *got.Note != "hello" {
		t.Errorf("expected Note=*\"hello\", got %v", got.Note)
	}
	if got.Ratio != nil {
		t.Errorf("expected Ratio nil (unset), got %v", got.Ratio)
	}
}

func TestPtrScalarRoundTrip(t *testing.T) {
	flag := true
	count := -7
	note := "n"
	ratio := 1.5
	in := withPtrScalars{Summary: "s", Flag: &flag, Count: &count, Note: &note, Ratio: &ratio}

	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got withPtrScalars
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip mismatch\n%s\ngot %+v\nwant %+v", out, got, in)
	}
}

func TestPtrScalarMarshalNilOmits(t *testing.T) {
	in := withPtrScalars{Summary: "only"}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if strings.Contains(out, "@Flag") || strings.Contains(out, "@Count") ||
		strings.Contains(out, "@Note") || strings.Contains(out, "@Ratio") {
		t.Errorf("nil pointer fields should be omitted, got:\n%s", out)
	}
}

func TestPtrBool(t *testing.T) {
	type s struct{ V *bool }
	cases := []struct {
		in   string
		want bool
	}{
		{"@V true", true},
		{"@V false", false},
	}
	for _, c := range cases {
		var got s
		if err := Unmarshal(c.in, &got); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got.V == nil || *got.V != c.want {
			t.Errorf("%q: expected *%v, got %v", c.in, c.want, got.V)
		}
	}

	var bad s
	if err := Unmarshal("@V yes", &bad); err == nil {
		t.Error("expected error for non-bool value")
	}

	var unset s
	if err := Unmarshal("", &unset); err != nil {
		t.Fatalf("empty doc: %v", err)
	}
	if unset.V != nil {
		t.Errorf("expected nil when directive absent, got %v", unset.V)
	}
}

func TestPtrInt(t *testing.T) {
	type s struct{ V *int }
	cases := []struct {
		in   string
		want int
	}{
		{"@V 0", 0},
		{"@V 42", 42},
		{"@V -7", -7},
	}
	for _, c := range cases {
		var got s
		if err := Unmarshal(c.in, &got); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got.V == nil || *got.V != c.want {
			t.Errorf("%q: expected *%d, got %v", c.in, c.want, got.V)
		}
	}

	var bad s
	if err := Unmarshal("@V 1.5", &bad); err == nil {
		t.Error("expected error for non-integer value")
	}

	var unset s
	if err := Unmarshal("", &unset); err != nil {
		t.Fatalf("empty doc: %v", err)
	}
	if unset.V != nil {
		t.Errorf("expected nil when directive absent, got %v", unset.V)
	}
}

func TestPtrFloat64(t *testing.T) {
	type s struct{ V *float64 }
	cases := []struct {
		in   string
		want float64
	}{
		{"@V 0", 0},
		{"@V 1.5", 1.5},
		{"@V -3.25", -3.25},
		{"@V 1e3", 1000},
	}
	for _, c := range cases {
		var got s
		if err := Unmarshal(c.in, &got); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got.V == nil || *got.V != c.want {
			t.Errorf("%q: expected *%v, got %v", c.in, c.want, got.V)
		}
	}

	var bad s
	if err := Unmarshal("@V abc", &bad); err == nil {
		t.Error("expected error for non-numeric value")
	}

	var unset s
	if err := Unmarshal("", &unset); err != nil {
		t.Fatalf("empty doc: %v", err)
	}
	if unset.V != nil {
		t.Errorf("expected nil when directive absent, got %v", unset.V)
	}
}

func TestPtrScalarDuplicateDirectiveErrors(t *testing.T) {
	var got withPtrScalars
	err := Unmarshal("@Flag true\n@Flag false", &got)
	if err == nil {
		t.Fatal("expected duplicate-directive error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate directive") {
		t.Errorf("expected duplicate-directive error, got: %v", err)
	}
}

func TestCacheIsolation(t *testing.T) {
	var a, b doc
	if err := Unmarshal("@Summary first", &a); err != nil {
		t.Fatal(err)
	}
	if err := Unmarshal("@Summary second\n@Tags x", &b); err != nil {
		t.Fatal(err)
	}
	if a.Summary != "first" || len(a.Tags) != 0 {
		t.Errorf("first decode contaminated: %+v", a)
	}
	if b.Summary != "second" || len(b.Tags) != 1 {
		t.Errorf("second decode wrong: %+v", b)
	}
}

func TestInvalidTarget(t *testing.T) {
	if err := Unmarshal("@Summary x", doc{}); err == nil {
		t.Error("expected error for non-pointer target")
	}
	if err := Unmarshal("@Summary x", (*doc)(nil)); err == nil {
		t.Error("expected error for nil pointer target")
	}
	var n int
	if err := Unmarshal("@Summary x", &n); err == nil {
		t.Error("expected error for pointer-to-non-struct target")
	}
}

// --- tokenizer unit tables ---

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"200 model.Response", []string{"200", "model.Response"}},
		{`id path int true "ID param"`, []string{"id", "path", "int", "true", "ID param"}},
		{`"quoted only"`, []string{"quoted only"}},
		{"", nil},
	}
	for _, tc := range tests {
		got, err := splitArgs(tc.in)
		if err != nil {
			t.Errorf("splitArgs(%q) error: %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitArgs(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
	if _, err := splitArgs(`"unterminated`); err == nil {
		t.Error("expected error for unterminated quote")
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"one,two,three", []string{"one", "two", "three"}},
		{`"Something longer", "Another"`, []string{"Something longer", "Another"}},
		{`"a,b", c`, []string{"a,b", "c"}},
		{"single", []string{"single"}},
	}
	for _, tc := range tests {
		got, err := splitCSV(tc.in)
		if err != nil {
			t.Errorf("splitCSV(%q) error: %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestTopLevelIndentNotDropped(t *testing.T) {
	// The first directive is indented deeper than a later top-level directive;
	// neither must be dropped (regression for the baseIndent gap).
	var got doc
	if err := Unmarshal("  @Summary a\n@Tags b", &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Summary != "a" || len(got.Tags) != 1 || got.Tags[0] != "b" {
		t.Errorf("got %+v, want Summary=a Tags=[b]", got)
	}
}

func TestEmptySliceLiteral(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want doc
	}{
		{"empty scalar slice", "@Tags []", doc{Tags: []string{}}},
		{"empty struct slice", "@Param []", doc{Params: []inner{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got doc
			if err := Unmarshal(tc.in, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}

	// Round-trip: a non-nil empty slice survives Marshal -> Unmarshal.
	in := doc{Tags: []string{}, Params: []inner{}}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got doc
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("empty-slice round trip mismatch\n%s\ngot %#v\nwant %#v", out, got, in)
	}
}

func TestSliceOfSlices(t *testing.T) {
	in := "@Row [\"value 1\", \"value 2\"]\n" +
		"@Row [value 3]\n" +
		"@Row []\n"

	want := doc{Matrix: [][]string{{"value 1", "value 2"}, {"value 3"}, {}}}

	var got doc
	if err := Unmarshal(in, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}

	// Round-trip.
	out, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var back doc
	if err := Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("slice-of-slices round trip mismatch\n%s\ngot %#v\nwant %#v", out, back, want)
	}

	// Brackets are optional on input.
	var csv doc
	if err := Unmarshal("@Row a,b,c", &csv); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(csv.Matrix, [][]string{{"a", "b", "c"}}) {
		t.Errorf("unbracketed inner slice = %#v", csv.Matrix)
	}
}

func TestEscapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"escaped quotes", `@Summary "he said \"hi\""`, `he said "hi"`},
		{"escaped backslash", `@Summary "a\\b"`, `a\b`},
		{"escaped newline", `@Summary "line1\nline2"`, "line1\nline2"},
		{"escaped tab", `@Summary "a\tb"`, "a\tb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got doc
			if err := Unmarshal(tc.in, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.Summary != tc.want {
				t.Errorf("got %q, want %q", got.Summary, tc.want)
			}
		})
	}
}

func TestEscapeRoundTrip(t *testing.T) {
	in := doc{Summary: `quote " and back\slash`, Description: "with\ttab"}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got doc
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("escape round trip mismatch\n%s\ngot %#v\nwant %#v", out, got, in)
	}
}

func TestUnexpectedTextAfterQuote(t *testing.T) {
	var got doc
	if err := Unmarshal(`@Summary "a" trailing`, &got); err == nil {
		t.Error("expected error for text after quoted value")
	}
}

func TestWriteOnlyCodecMarshalError(t *testing.T) {
	type ro struct {
		ID readOnlyID
	}
	// Decoding works.
	var d ro
	if err := Unmarshal("@ID abc", &d); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if d.ID != readOnlyID("abc") {
		t.Errorf("got %q", d.ID)
	}
	// Marshaling a set value fails loudly.
	if _, err := Marshal(ro{ID: "abc"}); err == nil {
		t.Error("expected Marshal error for write-only codec")
	}
}

func TestMapScalarInline(t *testing.T) {
	in := "@Scalars a: 1\n@Scalars b: 2\n"
	var got withMaps
	if err := Unmarshal(in, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	want := map[string]int{"a": 1, "b": 2}
	if !reflect.DeepEqual(got.Scalars, want) {
		t.Errorf("got %#v, want %#v", got.Scalars, want)
	}
}

func TestMapStructValue(t *testing.T) {
	// value is a struct: inline positional after ':' or an indented block.
	in := "@Structs first: 200 resp\n" +
		"@Structs second:\n" +
		"  @Code 404\n" +
		"  @Name missing\n"
	var got withMaps
	if err := Unmarshal(in, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	want := map[string]inner{
		"first":  {Code: 200, Name: "resp"},
		"second": {Code: 404, Name: "missing"},
	}
	if !reflect.DeepEqual(got.Structs, want) {
		t.Errorf("got %#v, want %#v", got.Structs, want)
	}
}

func TestMapStructKey(t *testing.T) {
	// struct key (positional before ':'), scalar value.
	in := "@Keyed 1 a: hello\n"
	var got withMaps
	if err := Unmarshal(in, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	want := map[inner]string{{Code: 1, Name: "a"}: "hello"}
	if !reflect.DeepEqual(got.Keyed, want) {
		t.Errorf("got %#v, want %#v", got.Keyed, want)
	}
}

func TestMapEmptyAndNil(t *testing.T) {
	var got withMaps
	if err := Unmarshal("@Scalars []", &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Scalars == nil || len(got.Scalars) != 0 {
		t.Errorf("expected empty non-nil map, got %#v", got.Scalars)
	}
}

func TestMapRoundTrip(t *testing.T) {
	in := withMaps{
		Scalars: map[string]int{"a": 1, "b": 2},
		Structs: map[string]inner{"x": {Code: 1, Name: "one"}, "y": {Code: 2}},
		Keyed:   map[inner]string{{Code: 1, Name: "k"}: "v"},
	}
	out, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got withMaps
	if err := Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal error: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("map round trip mismatch\n%s\ngot %#v\nwant %#v", out, got, in)
	}
}

func TestMapEntryErrors(t *testing.T) {
	var got withMaps
	if err := Unmarshal("@Scalars", &got); err == nil {
		t.Error("expected error for bare map directive")
	}
	if err := Unmarshal("@Scalars a 1", &got); err == nil {
		t.Error("expected error for map entry without ':'")
	}
}

func TestDuplicateSingleValueError(t *testing.T) {
	var got doc
	err := Unmarshal("@Summary A\n@Summary B", &got)
	if err == nil || !strings.Contains(err.Error(), "duplicate directive") {
		t.Errorf("expected duplicate error, got %v", err)
	}

	// Slices and maps remain additive (no duplicate error).
	var ok doc
	if err := Unmarshal("@Tags a\n@Tags b", &ok); err != nil {
		t.Errorf("repeated slice directive should be allowed: %v", err)
	}
	if !reflect.DeepEqual(ok.Tags, []string{"a", "b"}) {
		t.Errorf("got %#v", ok.Tags)
	}
}

func TestStrictIndentationError(t *testing.T) {
	// @Tags is a flat slice; it does not open a block, so an indented following
	// directive is a misindentation error rather than silently re-bound.
	var got doc
	err := Unmarshal("@Summary hi\n  @Tags a", &got)
	if err == nil || !strings.Contains(err.Error(), "unexpected indentation") {
		t.Errorf("expected unexpected-indentation error, got %v", err)
	}
	if got.Summary != "hi" {
		t.Errorf("Summary should still parse, got %q", got.Summary)
	}
}

func TestOptionalBrackets(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"@Tags [a, b, c]", []string{"a", "b", "c"}},
		{"@Tags a,b,c", []string{"a", "b", "c"}},
		{`@Tags ["[]"]`, []string{"[]"}}, // literal "[]" element via quoting
	}
	for _, tc := range tests {
		var got doc
		if err := Unmarshal(tc.in, &got); err != nil {
			t.Fatalf("Unmarshal(%q) error: %v", tc.in, err)
		}
		if !reflect.DeepEqual(got.Tags, tc.want) {
			t.Errorf("Unmarshal(%q) = %#v, want %#v", tc.in, got.Tags, tc.want)
		}
	}

	// Bare slice directive is an error; [] is the empty literal.
	var bare doc
	if err := Unmarshal("@Tags", &bare); err == nil {
		t.Error("expected error for bare slice directive")
	}
	var empty doc
	if err := Unmarshal("@Tags []", &empty); err != nil || empty.Tags == nil || len(empty.Tags) != 0 {
		t.Errorf("expected empty non-nil slice, got %#v err=%v", empty.Tags, err)
	}
}

func TestDedent(t *testing.T) {
	in := []string{
		"",
		"    line one",
		"      indented",
		"    line two",
		"",
	}
	want := "line one\n  indented\nline two"
	if got := dedent(in); got != want {
		t.Errorf("dedent = %q, want %q", got, want)
	}
}
