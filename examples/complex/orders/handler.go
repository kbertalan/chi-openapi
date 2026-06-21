package orders

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kbertalan/chi-openapi/examples/complex/apierr"
	"github.com/kbertalan/chi-openapi/examples/complex/pagination"
	"github.com/kbertalan/chi-openapi/examples/complex/users"
)

// Handler serves the order endpoints from an in-memory store.
type Handler struct {
	mu     sync.Mutex
	orders map[string]Order
	seq    int
}

// NewHandler returns an empty order Handler.
func NewHandler() *Handler {
	return &Handler{orders: map[string]Order{}}
}

// Register mounts the order routes on r. Auth middleware is applied by the caller.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Patch("/{id}/status", h.UpdateStatus)
}

// List returns all orders.
//
// @Summary List orders
// @Description Return a page of orders.
// @Tags orders
// @Param status query string false "Filter by status"
// @Success 200 PaginatedResponse[orders.Order] "page of matching orders"
// @Failure 500 ErrorResponse "internal error"
// @See https://example.com/docs/orders "Orders guide"
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	want := OrderStatus(r.URL.Query().Get("status"))

	h.mu.Lock()
	defer h.mu.Unlock()

	items := make([]Order, 0, len(h.orders))
	for _, o := range h.orders {
		if want != "" && o.Status != want {
			continue
		}
		items = append(items, o)
	}
	writeJSON(w, http.StatusOK, pagination.New(items))
}

// Create places a new order.
//
// @Summary Create an order
// @Tags orders
// @Accept application/json
// @Param body body CreateOrderRequest true "order to create"
// @Success 201 Order "the created order"
// @Failure 400 ErrorResponse "invalid payload"
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		apierr.Write(w, http.StatusBadRequest, "at least one item is required")
		return
	}

	h.mu.Lock()
	h.seq++
	id := strconv.Itoa(h.seq)
	o := Order{
		ID:        id,
		Customer:  users.User{ID: req.CustomerID},
		Items:     req.Items,
		Status:    OrderStatusPending,
		CreatedAt: time.Now().UTC(),
	}
	h.orders[id] = o
	h.mu.Unlock()

	writeJSON(w, http.StatusCreated, o)
}

// Get returns a single order by ID.
//
// @Summary Get an order
// @Tags orders
// @Success 200 Order "the requested order"
// @Failure 404 ErrorResponse "order not found"
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	o, ok := h.orders[chi.URLParam(r, "id")]
	h.mu.Unlock()
	if !ok {
		apierr.Write(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// UpdateStatus transitions an order to a new status.
//
// @Summary Update order status
// @Tags orders
// @Accept application/json
// @Param body body UpdateStatusRequest true "new status"
// @Success 200 Order "the updated order"
// @Failure 400 ErrorResponse "invalid status"
// @Failure 404 ErrorResponse "order not found"
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.Write(w, http.StatusBadRequest, "invalid status payload")
		return
	}

	id := chi.URLParam(r, "id")
	h.mu.Lock()
	defer h.mu.Unlock()
	o, ok := h.orders[id]
	if !ok {
		apierr.Write(w, http.StatusNotFound, "order not found")
		return
	}
	o.Status = req.Status
	h.orders[id] = o
	writeJSON(w, http.StatusOK, o)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
