package openapi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"log/slog"
	"strings"
)

// parseGenericInstantiation splits a generic instantiation string such as
// "PaginatedResponse[BusinessObject]" or "Map[string,order.Item]" into its base
// type name and the list of type-argument strings. It is bracket-depth aware so
// nested instantiations like "Page[Wrapper[Item]]" are handled correctly.
//
// It deliberately returns ok=false for slice/map/pointer expressions ("[]T",
// "map[K]V") so those keep flowing through the existing basic-type handling.
func parseGenericInstantiation(s string) (base string, args []string, ok bool) {
	s = strings.TrimSpace(s)
	open := strings.IndexByte(s, '[')
	// open <= 0 also rejects "[]T" (bracket at index 0).
	if open <= 0 || !strings.HasSuffix(s, "]") {
		return "", nil, false
	}
	base = strings.TrimSpace(s[:open])
	// A "map[" prefix is not a generic instantiation.
	if base == "map" {
		return "", nil, false
	}
	inner := s[open+1 : len(s)-1]
	args = splitTopLevelArgs(inner)
	if base == "" || len(args) == 0 {
		return "", nil, false
	}
	return base, args, true
}

// splitTopLevelArgs splits a comma-separated argument list while respecting
// bracket nesting, so "string,Wrapper[A,B]" yields ["string", "Wrapper[A,B]"].
func splitTopLevelArgs(s string) []string {
	var args []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		args = append(args, last)
	}
	return args
}

// sanitizeSchemaKey turns a (qualified) generic instantiation string into a
// $ref-safe component key, e.g. "order.PaginatedResponse[order.Item]" ->
// "order.PaginatedResponse-order.Item". Dots are kept because existing keys
// already rely on them; only bracket and separator characters are rewritten.
func sanitizeSchemaKey(s string) string {
	return strings.NewReplacer("[", "-", "]", "", ",", "-", " ", "").Replace(s)
}

// qualifyTypeString resolves an unqualified type-argument string to its
// qualified form, recursing through pointer/slice prefixes and nested generic
// instantiations so the resulting key is stable and unambiguous.
func (sg *SchemaGenerator) qualifyTypeString(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"*", "[]"} {
		if strings.HasPrefix(s, prefix) {
			return prefix + sg.qualifyTypeString(s[len(prefix):])
		}
	}
	if base, args, ok := parseGenericInstantiation(s); ok {
		qArgs := make([]string, len(args))
		for i, a := range args {
			qArgs[i] = sg.qualifyTypeString(a)
		}
		return sg.getQualifiedTypeName(base) + "[" + strings.Join(qArgs, ",") + "]"
	}
	return sg.getQualifiedTypeName(s)
}

// typeParamNames flattens the names declared in a type-parameter list,
// preserving order. It handles both "[T any]" and grouped "[K, V any]" forms.
func typeParamNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var names []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// generateGenericSchema monomorphizes a generic type instantiation into a
// dedicated component schema and returns a $ref to it.
func (sg *SchemaGenerator) generateGenericSchema(base string, rawArgs []string) *Schema {
	qBase := sg.getQualifiedTypeName(base)
	args := make([]string, len(rawArgs))
	for i, a := range rawArgs {
		args[i] = sg.qualifyTypeString(a)
	}

	key := sanitizeSchemaKey(qBase + "[" + strings.Join(args, ",") + "]")
	slog.Debug("[openapi] generateGenericSchema: called", "base", qBase, "args", args, "key", key)

	// Cache / recursion guard keyed by the sanitized component name.
	sg.mutex.Lock()
	if _, exists := sg.schemas[key]; exists {
		sg.mutex.Unlock()
		return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", key)}
	}
	sg.schemas[key] = nil
	sg.mutex.Unlock()

	var built *Schema
	if ts := sg.typeIndex.LookupQualifiedType(qBase); ts != nil && ts.TypeParams != nil {
		params := typeParamNames(ts.TypeParams)
		subst := make(map[string]string, len(params))
		for i, p := range params {
			if i < len(args) {
				subst[p] = args[i]
			}
		}

		// Swap in the generic's package context and parameter substitutions for
		// the duration of the conversion, then restore.
		oldPkg, oldParams := sg.currentPackage, sg.typeParams
		if idx := strings.LastIndex(qBase, "."); idx != -1 {
			sg.currentPackage = qBase[:idx]
		}
		sg.typeParams = subst

		if st, ok := ts.Type.(*ast.StructType); ok {
			built = sg.convertStructToSchema(st)
		} else {
			built = sg.convertFieldType(ts.Type)
		}

		sg.currentPackage, sg.typeParams = oldPkg, oldParams
	}

	if built == nil {
		slog.Debug("[openapi] generateGenericSchema: base not found or not generic", "base", qBase)
		built = &Schema{Type: "object", Description: "externally defined or unknown generic type"}
	}

	sg.mutex.Lock()
	sg.schemas[key] = built
	sg.mutex.Unlock()

	return &Schema{Ref: fmt.Sprintf("#/components/schemas/%s", key)}
}

// convertTypeString converts a (resolved) type-argument string into a schema by
// parsing it back into an AST expression and running it through the regular
// field-type conversion. This keeps a substituted type parameter rendering
// identical to an equivalent inline struct field (e.g. a "string" argument has
// no synthetic description, a "[]Foo" argument becomes an array, and so on).
func (sg *SchemaGenerator) convertTypeString(typeStr string) *Schema {
	if expr, err := parser.ParseExpr(typeStr); err == nil {
		return sg.convertFieldType(expr)
	}
	// Fall back to name-based generation if the string is not a parseable expr.
	return sg.GenerateSchema(typeStr)
}

// isTypeParam reports whether name is an active generic type parameter in the
// current conversion context.
func (sg *SchemaGenerator) isTypeParam(name string) bool {
	if sg.typeParams == nil {
		return false
	}
	_, ok := sg.typeParams[name]
	return ok
}

// exprTypeString renders an AST type expression back to a type string,
// substituting any active generic type parameters with their concrete
// arguments. Used to reconstruct nested generic instantiations encountered as
// struct field types.
func (sg *SchemaGenerator) exprTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if sg.typeParams != nil {
			if arg, ok := sg.typeParams[t.Name]; ok {
				return arg
			}
		}
		return sg.getQualifiedTypeName(t.Name)
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return "*" + sg.exprTypeString(t.X)
	case *ast.ArrayType:
		return "[]" + sg.exprTypeString(t.Elt)
	case *ast.IndexExpr:
		return sg.exprTypeString(t.X) + "[" + sg.exprTypeString(t.Index) + "]"
	case *ast.IndexListExpr:
		parts := make([]string, len(t.Indices))
		for i, ix := range t.Indices {
			parts[i] = sg.exprTypeString(ix)
		}
		return sg.exprTypeString(t.X) + "[" + strings.Join(parts, ",") + "]"
	}
	return ""
}
