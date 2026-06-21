package orders

import (
	"time"

	"github.com/kbertalan/chi-openapi/examples/complex/users"
)

// OrderStatus is the lifecycle state of an order.
//
// The constants below are a string-based enum; chi-openapi detects them and
// emits an OpenAPI `enum` for this type.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// OrderItem is a single line in an order. The openapi tag adds a pattern and
// numeric bounds to the generated schema.
type OrderItem struct {
	SKU      string `json:"sku" openapi:"pattern=^[A-Z]{3}-[0-9]{4}$,example=ABC-0001"`
	Quantity int    `json:"quantity" openapi:"minimum=1,maximum=999,default=1"`
}

// Order is a customer order. Customer is a cross-package reference to the
// users package, so the generated schema links the two with a $ref.
type Order struct {
	ID        string      `json:"id"`
	Customer  users.User  `json:"customer"`
	Items     []OrderItem `json:"items"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"createdAt"`
}

// CreateOrderRequest is the payload accepted by the create endpoint.
type CreateOrderRequest struct {
	CustomerID string      `json:"customerId"`
	Items      []OrderItem `json:"items"`
}

// UpdateStatusRequest changes the status of an existing order.
type UpdateStatusRequest struct {
	Status OrderStatus `json:"status"`
}
