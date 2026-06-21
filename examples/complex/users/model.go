package users

import "time"

// User is a registered account.
//
// The struct tags show two tag dialects chi-openapi understands: `validate`
// (here contributing the email format) and `openapi` (free-form schema
// metadata such as readOnly and example).
type User struct {
	ID        string    `json:"id" openapi:"readOnly=true,description=Server-assigned identifier"`
	Email     string    `json:"email" validate:"email"`
	Name      string    `json:"name" openapi:"example=Ada Lovelace"`
	CreatedAt time.Time `json:"createdAt" openapi:"readOnly=true"`
}

// CreateUserRequest is the payload accepted by the create endpoint.
type CreateUserRequest struct {
	Email string `json:"email" validate:"email" openapi:"description=Contact email address"`
	Name  string `json:"name" openapi:"minLength=2,maxLength=64,example=Ada Lovelace"`
}
