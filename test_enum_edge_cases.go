// Test file to check enum extraction edge cases
package openapi

// StatusEnum: const with implicit type inherited from a predecessor.
type StatusEnum string

const (
	StatusActive   StatusEnum = "active"
	StatusInactive            = "inactive" // Implicitly StatusEnum
	StatusPending  StatusEnum = "pending"
)

// TypeA and TypeB: multiple enum types sharing one const block.
type TypeA string
type TypeB string

const (
	TypeAVal1 TypeA = "a1"
	TypeAVal2 TypeA = "a2"
	TypeBVal1 TypeB = "b1"
	TypeBVal2 TypeB = "b2"
)
