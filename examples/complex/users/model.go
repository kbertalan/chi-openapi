package users

import "time"

// User is a registered account.
//
// The struct tags use the `openapi` dialect for free-form schema metadata such
// as format, readOnly, and example.
type User struct {
	ID        string    `json:"id" openapi:"readOnly=true,description=Server-assigned identifier"`
	Email     string    `json:"email" openapi:"format=email"`
	Name      string    `json:"name" openapi:"example=Ada Lovelace"`
	CreatedAt time.Time `json:"createdAt" openapi:"readOnly=true"`
}

// CreateUserRequest is the payload accepted by the create endpoint.
type CreateUserRequest struct {
	Email string `json:"email" openapi:"format=email,description=Contact email address"`
	Name  string `json:"name" openapi:"minLength=2,maxLength=64,example=Ada Lovelace"`
}
