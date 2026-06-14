package serde

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sync"
)

// fieldKind classifies a struct field once, so the hot decode/encode paths can
// switch on it instead of repeatedly interrogating reflect.Type.
type fieldKind int

const (
	kindUnsupported fieldKind = iota
	kindString
	kindInt
	kindUint
	kindFloat
	kindBool
	kindStruct
	kindPtrStruct
	kindSliceScalar
	kindSliceStruct
	kindSliceSlice // slice of scalar slices ([][]scalar)
	kindMap        // map[K]V
	kindTextCodec  // implements encoding.TextUnmarshaler/TextMarshaler
	kindJSONCodec  // implements json.Unmarshaler/json.Marshaler
)

// fieldDescriptor holds the precomputed binding information for a single struct
// field: which token maps to it and how its value is parsed/rendered.
type fieldDescriptor struct {
	index    int
	token    string
	kind     fieldKind
	elemType reflect.Type // slice element type, or pointee for *struct
	elemKind fieldKind    // for kindSliceScalar/kindSliceSlice: element/inner-scalar kind
	elemInfo *structInfo  // for struct/ptrStruct/sliceStruct: info of the struct type
	keyDesc  *fieldDescriptor // for kindMap: how to bind/render the key
	valDesc  *fieldDescriptor // for kindMap: how to bind/render the value
}

// structInfo is the cached binding map for a struct type.
type structInfo struct {
	byToken map[string]*fieldDescriptor
	order   []*fieldDescriptor
}

var typeCache sync.Map // map[reflect.Type]*structInfo

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()
)

// infoFor returns the cached structInfo for t, building it on first use. The
// in-progress *structInfo is stored before its fields are populated so that
// self-referential types do not recurse forever.
func infoFor(t reflect.Type) *structInfo {
	if cached, ok := typeCache.Load(t); ok {
		return cached.(*structInfo)
	}
	info := &structInfo{byToken: make(map[string]*fieldDescriptor)}
	typeCache.Store(t, info)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		token := f.Tag.Get("tag")
		if token == "-" {
			continue
		}
		if token == "" {
			token = f.Name
		}

		fd := newDescriptor(f.Type, token)
		fd.index = i
		info.byToken[token] = fd
		info.order = append(info.order, fd)
	}
	return info
}

// newDescriptor classifies t and fills in the auxiliary binding information
// (element kinds, nested struct info, map key/value descriptors). It is used for
// struct fields and, recursively, for map keys and values.
func newDescriptor(t reflect.Type, token string) *fieldDescriptor {
	fd := &fieldDescriptor{token: token}
	fd.kind, fd.elemType = classify(t)

	switch fd.kind {
	case kindStruct:
		fd.elemInfo = infoFor(t)
	case kindPtrStruct:
		fd.elemInfo = infoFor(fd.elemType)
	case kindSliceStruct:
		st := fd.elemType
		if st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		fd.elemInfo = infoFor(st)
	case kindSliceScalar:
		fd.elemKind, _ = classify(fd.elemType)
	case kindSliceSlice:
		// elemType is the inner []scalar type; elemKind is the inner scalar's
		// kind (used to parse/render each @Item value).
		fd.elemKind, _ = classify(fd.elemType.Elem())
	case kindMap:
		fd.keyDesc = newDescriptor(t.Key(), "Key")
		fd.valDesc = newDescriptor(t.Elem(), "Value")
		if fd.keyDesc.kind == kindUnsupported || fd.valDesc.kind == kindUnsupported {
			fd.kind = kindUnsupported
		}
	}
	return fd
}

// classify maps a Go type to a fieldKind. Custom codec types (Text/JSON
// (un)marshalers) take precedence over the underlying reflect.Kind, so a type
// such as uuid.UUID is treated as a codec rather than its array/struct form.
// The second return value is the element type for slices, or the pointee for
// *struct (nil otherwise).
func classify(t reflect.Type) (fieldKind, reflect.Type) {
	if isTextCodec(t) {
		return kindTextCodec, nil
	}
	if isJSONCodec(t) {
		return kindJSONCodec, nil
	}

	switch t.Kind() {
	case reflect.String:
		return kindString, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return kindInt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return kindUint, nil
	case reflect.Float32, reflect.Float64:
		return kindFloat, nil
	case reflect.Bool:
		return kindBool, nil
	case reflect.Pointer:
		if t.Elem().Kind() == reflect.Struct {
			return kindPtrStruct, t.Elem()
		}
		return kindUnsupported, nil
	case reflect.Struct:
		return kindStruct, t
	case reflect.Slice:
		et := t.Elem()
		ek, _ := classify(et)
		switch {
		case ek == kindStruct || ek == kindPtrStruct:
			return kindSliceStruct, et
		case isScalarKind(ek):
			return kindSliceScalar, et
		case ek == kindSliceScalar:
			return kindSliceSlice, et // [][]scalar
		default:
			return kindUnsupported, nil // [][]struct, [][][]…, []map, …
		}
	case reflect.Map:
		return kindMap, nil // key/value descriptors are built by newDescriptor
	}
	return kindUnsupported, nil
}

// isScalarKind reports whether a kind can be parsed from / rendered to a single
// token (used to decide positional binding and compact one-liner encoding).
func isScalarKind(k fieldKind) bool {
	switch k {
	case kindString, kindInt, kindUint, kindFloat, kindBool, kindTextCodec, kindJSONCodec:
		return true
	}
	return false
}

func isTextCodec(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

func isJSONCodec(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(jsonUnmarshalerType)
}
