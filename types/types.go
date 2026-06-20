// Package types provides ready-made OpenAPI schema mappings for common
// third-party Go libraries. These types cannot be introspected from source, so
// register the relevant set with openapi.AddExternalKnownTypes, e.g.:
//
//	openapi.AddExternalKnownTypes(types.PgType)
//	openapi.AddExternalKnownTypes(types.UUID)
//
// The maps are keyed by qualified type name (package selector + type), matching
// how the type appears in source.
package types

import openapi "github.com/kbertalan/chi-openapi"

// PgType maps github.com/jackc/pgx/v5/pgtype types to OpenAPI schemas.
var PgType = map[string]*openapi.Schema{
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
}

// UUID maps github.com/google/uuid types to OpenAPI schemas.
var UUID = map[string]*openapi.Schema{
	"uuid.UUID": {Type: "string", Format: "uuid", Description: "UUID string"},
	"*uuid.UUID": {
		Type:        []any{"string", "null"},
		Format:      "uuid",
		Description: "Nullable UUID string",
	},
}

// Decimal maps github.com/shopspring/decimal types to OpenAPI schemas.
var Decimal = map[string]*openapi.Schema{
	"decimal.Decimal": {Type: "string", Description: "Decimal number as string"},
	"*decimal.Decimal": {
		Type:        []any{"string", "null"},
		Description: "Nullable decimal number as string",
	},
}
