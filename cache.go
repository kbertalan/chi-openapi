package openapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
)

var (
	typeIndex     *TypeIndex
	typeIndexOnce sync.Once
	modulePath    string // primary module path (the module containing the cwd)
	projectRoot   string // primary module root directory

	modules     []moduleInfo // every module in scope (a Go workspace may have several)
	modulesOnce sync.Once
)

// moduleInfo describes a single Go module in scope. With a go.work workspace
// there may be several; otherwise there is exactly one.
type moduleInfo struct {
	dir  string // absolute, slash-normalized directory containing go.mod
	path string // module path (e.g. "github.com/me/app")
}

// Find a way to add method that will add external known types to the type index
// This is useful for types that are not defined in the current package but are known to the OpenAPI spec,
// such as types from external libraries or standard library types that we want to document.
func ensureTypeIndex() {
	// debug.PrintStack()
	typeIndexOnce.Do(func() {
		// load module path for package classification
		loadModulePath()
		slog.Debug("[openapi] cache.go: initializing typeIndex and schemaCache")
		// Build type index once at startup
		typeIndex = BuildTypeIndex()

		slog.Debug("[openapi] cache.go: typeIndex built, setting schemaCache")
		typeIndex.schemaCache = defaultSchemaCache()
		// Log the number of types and files indexed
		slog.Debug(
			"[openapi] cache.go: typeIndex initialized",
			"types",
			len(typeIndex.types),
			"files",
			len(typeIndex.files),
		)
	})
}

// TypeIndex provides fast lookup of type definitions by package and type name.
// All maps except schemaCache are built once and read-only afterwards;
// schemaCache is written during generation, so it is guarded by schemaCacheMu.
type TypeIndex struct {
	types          map[string]map[string]*ast.TypeSpec // package -> type -> spec
	files          map[string]*ast.File                // file path -> parsed file
	qualifiedTypes map[string]*ast.TypeSpec            // qualified type name -> spec (e.g., "order.CreateReq")
	packageImports map[string]string                   // import path -> package name (e.g., "github.com/user/sqlc" -> "sqlc")
	schemaCacheMu  sync.RWMutex                         // guards schemaCache
	schemaCache    map[string]*Schema                  // qualified type name -> schema (seeded external types + generated $refs)
}

// lookupSchemaCache returns the cached schema for a qualified type name, if any.
func (idx *TypeIndex) lookupSchemaCache(name string) (*Schema, bool) {
	if idx == nil {
		return nil, false
	}
	idx.schemaCacheMu.RLock()
	defer idx.schemaCacheMu.RUnlock()
	schema, ok := idx.schemaCache[name]
	return schema, ok
}

// storeSchemaCache caches a schema for a qualified type name.
func (idx *TypeIndex) storeSchemaCache(name string, schema *Schema) {
	if idx == nil {
		return
	}
	idx.schemaCacheMu.Lock()
	defer idx.schemaCacheMu.Unlock()
	if idx.schemaCache == nil {
		idx.schemaCache = make(map[string]*Schema)
	}
	idx.schemaCache[name] = schema
}

// BuildTypeIndex scans the given roots and builds a type index for all Go types.
func BuildTypeIndex() *TypeIndex {
	idx := &TypeIndex{
		types:          make(map[string]map[string]*ast.TypeSpec),
		files:          make(map[string]*ast.File),
		schemaCache:    make(map[string]*Schema),
		qualifiedTypes: make(map[string]*ast.TypeSpec),
		packageImports: make(map[string]string),
	}

	// Index every module in scope (a go.work workspace may declare several).
	roots := moduleDirs()
	if len(roots) == 0 {
		slog.Debug("[openapi] BuildTypeIndex: no modules found, using current directory")
		roots = []string{"."}
	} else {
		slog.Debug("[openapi] BuildTypeIndex: using module roots", "roots", roots)
	}

	for _, root := range roots {
		idx.indexModule(root)
	}

	idx.schemaCache = defaultSchemaCache()

	slog.Debug("[openapi] BuildTypeIndex: completed", "totalPackages", len(idx.types), "totalFiles", len(idx.files))
	return idx
}

// indexModule walks a single module directory and indexes its Go source files,
// skipping nested modules (subdirectories that declare their own go.mod).
func (idx *TypeIndex) indexModule(root string) {
	rootClean := filepath.Clean(root)
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// A subdirectory with its own go.mod belongs to a different module;
			// it is indexed under its own root (if in scope), not here.
			if filepath.Clean(p) != rootClean {
				if _, statErr := os.Stat(filepath.Join(p, "go.mod")); statErr == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		return idx.indexFile(p)
	})
}

// defaultSchemaCache returns hard-coded schemas for well-known external library
// types (time, uuid, sql, pgtype, ...) that cannot be introspected from source.
func defaultSchemaCache() map[string]*Schema {
	return map[string]*Schema{
		// JSON and raw data types
		"any":             {Description: "Any type (interface{})"},
		"json.RawMessage": {Description: "Raw JSON data"},
		"jsontext.Value":  {Description: "Raw JSON data"},
		"byte":            {Type: "integer", Format: "int32", Description: "Byte value"},
		"[]byte":          {Type: "string", Format: "byte", Description: "Binary data (base64-encoded)"},
		"rune":            {Type: "integer", Format: "int32", Description: "Rune (Unicode code point) value"},
		"[]rune":          {Type: "string", Description: "String data"},

		// PostgreSQL types (jackc/pgtype)
		"pgtype.Text":        {Type: "string", Description: "PostgreSQL text type"},
		"pgtype.Bool":        {Type: "boolean", Description: "PostgreSQL boolean type"},
		"pgtype.Int2":        {Type: "integer", Format: "int32", Description: "PostgreSQL smallint (int16)"},
		"pgtype.Int4":        {Type: "integer", Format: "int32", Description: "PostgreSQL integer (int32)"},
		"pgtype.Int8":        {Type: "integer", Format: "int64", Description: "PostgreSQL bigint (int64)"},
		"pgtype.Float4":      {Type: "number", Format: "float", Description: "PostgreSQL real (float32)"},
		"pgtype.Float8":      {Type: "number", Format: "double", Description: "PostgreSQL double precision (float64)"},
		"pgtype.Numeric":     {Type: "number", Description: "PostgreSQL numeric/decimal type"},
		"pgtype.Interval":    {Type: "string", Description: "PostgreSQL interval type"},
		"pgtype.Timestamptz": {Type: "string", Format: "date-time", Description: "PostgreSQL timestamp with timezone"},
		"pgtype.Timestamp": {
			Type:        "string",
			Format:      "date-time",
			Description: "PostgreSQL timestamp without timezone",
		},
		"pgtype.Date":  {Type: "string", Format: "date", Description: "PostgreSQL date type"},
		"pgtype.Point": {Type: "string", Description: "PostgreSQL point type (e.g., '(1.0,2.0)')"},
		"pgtype.UUID":  {Type: "string", Format: "uuid", Description: "PostgreSQL UUID type"},
		"pgtype.JSONB": {Description: "PostgreSQL JSONB type"},
		"pgtype.JSON":  {Description: "PostgreSQL JSON type"},

		// Time types
		"time.Time": {Type: "string", Format: "date-time", Description: "RFC3339 date-time"},
		"*time.Time": {
			Type:        []any{"string", "null"},
			Format:      "date-time",
			Description: "Nullable RFC3339 date-time",
		},
		"time.Duration": {
			Type:        "string",
			Description: "Duration string (e.g., '1h30m'). Note: default Go JSON marshal is nanoseconds (integer).",
		},
		"time.Weekday": {Type: "integer", Description: "Go time.Weekday (0=Sunday, ...)"},

		// UUID types
		"uuid.UUID": {Type: "string", Format: "uuid", Description: "UUID string"},
		"*uuid.UUID": {
			Type:        []any{"string", "null"},
			Format:      "uuid",
			Description: "Nullable UUID string",
		},

		// Network types
		"net.IP":    {Type: "string", Format: "ipv4", Description: "IPv4 address"},
		"net.IPNet": {Type: "string", Description: "IP network (CIDR notation)"},
		"url.URL":   {Type: "string", Format: "uri", Description: "URL string"},
		"*url.URL": {
			Type:        []any{"string", "null"},
			Format:      "uri",
			Description: "Nullable URL string",
		},

		// Database driver types (database/sql)
		"sql.NullString":  {Type: []any{"string", "null"}, Description: "Nullable string"},
		"sql.NullInt64":   {Type: []any{"integer", "null"}, Format: "int64", Description: "Nullable integer"},
		"sql.NullInt32":   {Type: []any{"integer", "null"}, Format: "int32", Description: "Nullable integer"},
		"sql.NullFloat64": {Type: []any{"number", "null"}, Description: "Nullable number"},
		"sql.NullBool":    {Type: []any{"boolean", "null"}, Description: "Nullable boolean"},
		"sql.NullTime":    {Type: []any{"string", "null"}, Format: "date-time", Description: "Nullable date-time"},
		"sql.RawBytes":    {Type: "string", Format: "byte", Description: "Raw database bytes (base64)"},

		// Common Go types
		"big.Int": {Type: "string", Description: "Big integer as string"},
		"*big.Int": {
			Type:        []any{"string", "null"},
			Description: "Nullable big integer as string",
		},
		"decimal.Decimal": {Type: "string", Description: "Decimal number as string"},
		"*decimal.Decimal": {
			Type:        []any{"string", "null"},
			Description: "Nullable decimal number as string",
		},
	}
}

// indexFile processes a single Go file and indexes its types
func (idx *TypeIndex) indexFile(filePath string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		slog.Debug("[openapi] BuildTypeIndex: failed to parse file", "path", filePath, "err", err)
		return nil // Continue with other files
	}

	// Key by module-relative path so lookups are consistent across platforms
	// and identical whether or not the binary was built with -trimpath.
	normalizedPath := toModuleRelativePath(filePath)
	idx.files[normalizedPath] = file
	pkg := file.Name.Name

	// Record package imports for external vs internal classification
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		var alias string
		if imp.Name != nil && imp.Name.Name != "" {
			alias = imp.Name.Name
		} else {
			alias = path.Base(importPath)
		}
		idx.packageImports[importPath] = alias
	}

	if _, ok := idx.types[pkg]; !ok {
		idx.types[pkg] = make(map[string]*ast.TypeSpec)
	}

	// Index type declarations
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			for _, spec := range gd.Specs {
				if ts, isTypeSpec := spec.(*ast.TypeSpec); isTypeSpec {
					typeName := ts.Name.Name
					qualifiedName := idx.getQualifiedTypeName(pkg, typeName)

					// Store in both maps
					idx.types[pkg][typeName] = ts
					idx.qualifiedTypes[qualifiedName] = ts

					slog.Debug(
						"[openapi] BuildTypeIndex: indexed type",
						"package", pkg,
						"type", typeName,
						"qualified", qualifiedName,
						"file", filePath,
					)
				}
			}
		}
	}

	return nil
}

func GetTypeIndex() *TypeIndex {
	if typeIndex == nil {
		slog.Error("[openapi] GetTypeIndex: typeIndex is nil, building type index")
		typeIndex = BuildTypeIndex()
	} else {
		slog.Debug("[openapi] GetTypeIndex: returning existing typeIndex")
	}
	return typeIndex
}

// LookupType returns the TypeSpec for a given package and type name, or nil if not found.
func (idx *TypeIndex) LookupType(pkg, typeName string) *ast.TypeSpec {
	if idx == nil {
		return nil
	}
	if pkgTypes, ok := idx.types[pkg]; ok {
		return pkgTypes[typeName]
	}
	return nil
}

// LookupQualifiedType returns the TypeSpec for a qualified type name (e.g., "order.CreateReq")
func (idx *TypeIndex) LookupQualifiedType(qualifiedName string) *ast.TypeSpec {
	if idx == nil {
		return nil
	}
	return idx.qualifiedTypes[qualifiedName]
}

// LookupFile returns the AST for a given file path, handling normalization and case-insensitivity on Windows.
func (idx *TypeIndex) LookupFile(filePath string) *ast.File {
	if idx == nil {
		return nil
	}
	normalized := filepath.ToSlash(filePath)
	if f, ok := idx.files[normalized]; ok {
		return f
	}

	// Case-insensitive fallback for Windows
	for p, f := range idx.files {
		if strings.EqualFold(p, normalized) {
			return f
		}
	}
	return nil
}

// LookupUnqualifiedType searches for a type across all packages and returns the first match along with qualified name
func (idx *TypeIndex) LookupUnqualifiedType(typeName string) (*ast.TypeSpec, string) {
	if idx == nil {
		return nil, ""
	}

	// First check if it's a basic type
	if isBasicType(typeName) {
		return nil, ""
	}

	// Collect candidate packages that define this type. Map iteration order
	// is nondeterministic, so we gather and sort package names to be stable.
	var candidates []string
	for pkgName, pkgTypes := range idx.types {
		if _, exists := pkgTypes[typeName]; exists {
			candidates = append(candidates, pkgName)
		}
	}
	if len(candidates) == 0 {
		return nil, ""
	}

	sort.Strings(candidates)

	// Prefer internal (non-external) packages over external ones.
	for _, pkgName := range candidates {
		if !idx.isExternalPackage(pkgName) {
			typeSpec := idx.types[pkgName][typeName]
			qualifiedName := idx.getQualifiedTypeName(pkgName, typeName)
			return typeSpec, qualifiedName
		}
	}

	// No internal candidate found; return the first external candidate (deterministic due to sorting)
	pkgName := candidates[0]
	typeSpec := idx.types[pkgName][typeName]
	qualifiedName := idx.getQualifiedTypeName(pkgName, typeName)
	return typeSpec, qualifiedName
}

// GetQualifiedTypeName returns the appropriate qualified name for a type
func (idx *TypeIndex) GetQualifiedTypeName(typeName string) string {
	// If already qualified, return as-is
	if strings.Contains(typeName, ".") {
		return typeName
	}

	// Look up the type and return its qualified name
	if _, qualifiedName := idx.LookupUnqualifiedType(typeName); qualifiedName != "" {
		return qualifiedName
	}

	// Fallback to original name
	return typeName
}

// AddExternalKnownType registers a hard-coded schema for a qualified type name.
func AddExternalKnownType(name string, schema *Schema) {
	ensureTypeIndex() // Ensure typeIndex is initialized
	if typeIndex == nil {
		slog.Error("[openapi] AddExternalKnownType: typeIndex is nil, cannot add external type", "name", name)
		return
	}
	typeIndex.storeSchemaCache(name, schema)
	slog.Debug("[openapi] AddExternalKnownType: added external known type", "name", name)
}

// resetTypeIndexForTesting resets the type index for testing purposes
// This should only be used in tests
func resetTypeIndexForTesting() {
	typeIndex = nil
	typeIndexOnce = sync.Once{}
}

// getQualifiedTypeName creates a qualified type name for indexing.
// For external packages (like sqlc, pgtype), use the package name as-is.
// For internal project types, use package.TypeName format.
func (idx *TypeIndex) getQualifiedTypeName(pkg, typeName string) string {
	// Check if this is an external/third-party package
	if idx.isExternalPackage(pkg) {
		return pkg + "." + typeName
	}

	// For internal project types, use package.TypeName format
	return pkg + "." + typeName
}

// isExternalPackage determines if a package is external/third-party
func (idx *TypeIndex) isExternalPackage(pkg string) bool {
	// If an import alias maps to a path outside every in-scope module, external.
	for importPath, alias := range idx.packageImports {
		if alias == pkg {
			return !isInternalImportPath(importPath)
		}
	}
	// Default to internal
	return false
}

// isInternalImportPath reports whether an import path belongs to one of the
// in-scope modules (the single module, or any module of a go.work workspace).
func isInternalImportPath(importPath string) bool {
	loadModules()
	for _, m := range modules {
		if importPath == m.path || strings.HasPrefix(importPath, m.path+"/") {
			return true
		}
	}
	return false
}

// findProjectRoot finds the project root by looking for go.mod file
func findProjectRoot() string {
	// Start from current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Walk up the directory tree looking for go.mod
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return currentDir
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			break
		}
		currentDir = parentDir
	}

	return ""
}

// loadModules discovers every Go module in scope, exactly once. With a go.work
// workspace this is each `use`d module; otherwise it is the single module found
// by walking up for go.mod. Modules are sorted by descending directory and path
// length so that prefix matching against nested modules picks the most specific.
func loadModules() {
	modulesOnce.Do(func() {
		modules = discoverModules()
		sort.Slice(modules, func(i, j int) bool {
			if len(modules[i].dir) != len(modules[j].dir) {
				return len(modules[i].dir) > len(modules[j].dir)
			}
			return len(modules[i].path) > len(modules[j].path)
		})
	})
}

// discoverModules returns the modules in scope: the workspace modules when a
// go.work is active, otherwise the single module containing the cwd.
func discoverModules() []moduleInfo {
	if workFile := findGoWork(); workFile != "" {
		if mods := parseWorkspaceModules(workFile); len(mods) > 0 {
			return mods
		}
	}

	root := findProjectRoot()
	if root == "" {
		return nil
	}
	if mp := readModulePath(filepath.Join(root, "go.mod")); mp != "" {
		return []moduleInfo{{dir: filepath.ToSlash(root), path: mp}}
	}
	return nil
}

// findGoWork locates the active go.work file, mirroring the go tool's rules:
// GOWORK=off disables workspace mode, GOWORK=<path> names the file explicitly,
// and an unset GOWORK triggers a search upward from the cwd.
func findGoWork() string {
	switch gw := os.Getenv("GOWORK"); gw {
	case "off":
		return ""
	case "":
		dir, err := os.Getwd()
		if err != nil {
			return ""
		}
		for {
			candidate := filepath.Join(dir, "go.work")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return ""
			}
			dir = parent
		}
	default:
		return gw
	}
}

// parseWorkspaceModules reads a go.work file and resolves each `use` directive
// to a module (directory + module path read from its go.mod).
func parseWorkspaceModules(workFile string) []moduleInfo {
	data, err := os.ReadFile(workFile)
	if err != nil {
		return nil
	}
	wf, err := modfile.ParseWork(workFile, data, nil)
	if err != nil {
		slog.Debug("[openapi] parseWorkspaceModules: failed to parse go.work", "file", workFile, "err", err)
		return nil
	}

	workDir := filepath.Dir(workFile)
	var mods []moduleInfo
	for _, use := range wf.Use {
		dir := use.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workDir, dir)
		}
		mp := readModulePath(filepath.Join(dir, "go.mod"))
		if mp == "" {
			slog.Debug("[openapi] parseWorkspaceModules: skipping use without module path", "dir", dir)
			continue
		}
		mods = append(mods, moduleInfo{dir: filepath.ToSlash(dir), path: mp})
	}
	return mods
}

// readModulePath extracts the module path from a go.mod file, or "" on error.
func readModulePath(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	return modfile.ModulePath(data)
}

// moduleDirs returns the filesystem directories of every in-scope module.
func moduleDirs() []string {
	loadModules()
	dirs := make([]string, 0, len(modules))
	for _, m := range modules {
		dirs = append(dirs, filepath.FromSlash(m.dir))
	}
	return dirs
}

// loadModulePath resolves the primary module (the one containing the cwd, else
// the first in scope) into modulePath/projectRoot for callers that assume a
// single module.
func loadModulePath() {
	loadModules()
	if modulePath != "" || len(modules) == 0 {
		return
	}

	primary := modules[0]
	if cwd, err := os.Getwd(); err == nil {
		cwdSlash := filepath.ToSlash(cwd)
		for _, m := range modules { // sorted longest-dir-first: most specific wins
			if cwdSlash == m.dir || strings.HasPrefix(cwdSlash, m.dir+"/") {
				primary = m
				break
			}
		}
	}
	modulePath = primary.path
	projectRoot = filepath.FromSlash(primary.dir)
}

// toModuleRelativePath converts a filesystem path to a module-relative path
// (e.g. "github.com/me/app/handlers/users.go"). This is the canonical form used
// as the type-index key and in HandlerInfo, matching what -trimpath emits, so
// handler resolution behaves identically with or without -trimpath. A path that
// belongs to no in-scope module is returned slash-normalized but unchanged.
func toModuleRelativePath(filePath string) string {
	if filePath == "" || filePath == "<autogenerated>" {
		return filePath
	}
	normalized := filepath.ToSlash(filePath)
	loadModules()

	// Already module-relative (e.g. emitted by -trimpath).
	for _, m := range modules {
		if normalized == m.path || strings.HasPrefix(normalized, m.path+"/") {
			return normalized
		}
	}
	// A filesystem path under a module directory becomes module-relative.
	for _, m := range modules {
		if rel, ok := strings.CutPrefix(normalized, m.dir+"/"); ok {
			return m.path + "/" + rel
		}
	}
	return normalized
}

// fromModuleRelativePath converts a module-relative path back to a filesystem
// path so the file can be opened. Paths that are not module-relative (e.g. bare
// or already-absolute paths used by tests) are returned unchanged.
func fromModuleRelativePath(filePath string) string {
	normalized := filepath.ToSlash(filePath)
	loadModules()
	for _, m := range modules {
		if rel, ok := strings.CutPrefix(normalized, m.path+"/"); ok {
			return filepath.Join(filepath.FromSlash(m.dir), filepath.FromSlash(rel))
		}
	}
	return filePath
}
