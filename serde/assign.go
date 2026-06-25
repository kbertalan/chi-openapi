package serde

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// assignScalar is setScalar with allocation for kindPtrScalar fields.
func assignScalar(fv reflect.Value, fd *fieldDescriptor, raw, token string) error {
	if fd.kind == kindPtrScalar {
		if fv.IsNil() {
			fv.Set(reflect.New(fd.elemType))
		}
		return setScalar(fv.Elem(), fd.elemKind, raw, token)
	}
	return setScalar(fv, fd.kind, raw, token)
}

// setScalar parses raw into the (addressable) field value fv according to kind.
// token is the directive name used in error messages.
func setScalar(fv reflect.Value, kind fieldKind, raw, token string) error {
	switch kind {
	case kindString:
		fv.SetString(raw)
	case kindInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("field %q: invalid integer %q", "@"+token, raw)
		}
		fv.SetInt(n)
	case kindUint:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("field %q: invalid integer %q", "@"+token, raw)
		}
		fv.SetUint(n)
	case kindFloat:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("field %q: invalid number %q", "@"+token, raw)
		}
		fv.SetFloat(n)
	case kindBool:
		switch raw {
		case "true":
			fv.SetBool(true)
		case "false":
			fv.SetBool(false)
		default:
			return fmt.Errorf("field %q: invalid bool %q", "@"+token, raw)
		}
	case kindTextCodec:
		return setTextCodec(fv, raw, token)
	case kindJSONCodec:
		return setJSONCodec(fv, raw, token)
	default:
		return fmt.Errorf("field %q: cannot assign value to this field kind", "@"+token)
	}
	return nil
}

// setTextCodec assigns raw via encoding.TextUnmarshaler (raw bytes, unquoted).
func setTextCodec(fv reflect.Value, raw, token string) error {
	u, ok := fv.Addr().Interface().(encoding.TextUnmarshaler)
	if !ok {
		return fmt.Errorf("field %q: does not implement TextUnmarshaler", "@"+token)
	}
	if err := u.UnmarshalText([]byte(raw)); err != nil {
		return fmt.Errorf("field %q: %v", "@"+token, err)
	}
	return nil
}

// setJSONCodec assigns raw via json.Unmarshaler. A raw value that is not already
// valid JSON is wrapped as a JSON string, so unquoted tokens (e.g. a bare UUID)
// decode into string-backed JSON types.
func setJSONCodec(fv reflect.Value, raw, token string) error {
	u, ok := fv.Addr().Interface().(json.Unmarshaler)
	if !ok {
		return fmt.Errorf("field %q: does not implement json.Unmarshaler", "@"+token)
	}
	data := []byte(raw)
	if !json.Valid(data) {
		quoted, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("field %q: %v", "@"+token, err)
		}
		data = quoted
	}
	if err := u.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("field %q: %v", "@"+token, err)
	}
	return nil
}

// bindPositional assigns args to the struct fields of dst in declaration order.
// Fewer args than fields leaves the trailing fields zero; more args is an error.
func bindPositional(dst reflect.Value, info *structInfo, args []string, token string) error {
	if len(args) > len(info.order) {
		return fmt.Errorf("field %q: too many positional args (%d for %d fields)", "@"+token, len(args), len(info.order))
	}
	for i, arg := range args {
		fd := info.order[i]
		// A scalar-slice field bound positionally consumes its single arg as a
		// comma-separated list (e.g. "@Security OAuth2 read,write").
		if fd.kind == kindSliceScalar {
			if err := appendSliceScalar(dst.Field(fd.index), fd, arg, fd.token); err != nil {
				return err
			}
			continue
		}
		if err := assignScalar(dst.Field(fd.index), fd, arg, fd.token); err != nil {
			return err
		}
	}
	return nil
}

// appendSliceScalar parses a comma-separated value and appends each element to
// the slice field fv. An empty value appends nothing.
func appendSliceScalar(fv reflect.Value, fd *fieldDescriptor, raw, token string) error {
	if raw == "" {
		return nil
	}
	elems, err := splitCSV(raw)
	if err != nil {
		return fmt.Errorf("field %q: %v", "@"+token, err)
	}
	for _, e := range elems {
		ev := reflect.New(fd.elemType).Elem()
		if err := setScalar(ev, fd.elemKind, e, token); err != nil {
			return err
		}
		fv.Set(reflect.Append(fv, ev))
	}
	return nil
}
