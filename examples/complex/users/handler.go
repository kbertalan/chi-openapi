package users

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kbertalan/chi-openapi/examples/complex/apierr"
	"github.com/kbertalan/chi-openapi/examples/complex/pagination"
)

// Handler serves the user endpoints from an in-memory store.
type Handler struct {
	mu    sync.Mutex
	users map[string]User
	seq   int
}

// NewHandler returns a Handler seeded with one user.
func NewHandler() *Handler {
	return &Handler{
		users: map[string]User{
			"1": {ID: "1", Email: "ada@example.com", Name: "Ada Lovelace", CreatedAt: time.Unix(0, 0).UTC()},
		},
		seq: 1,
	}
}

// Register mounts the user routes on r. Auth middleware is applied by the caller.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Delete("/{id}", h.Delete)
}

// List returns a page of users.
//
// @Summary List users
// @Description Return a page of users, optionally capped by a limit.
// @Tags users
// @Param limit query int false "Maximum number of users to return"
// @Success 200 PaginatedResponse[users.User] "page of users"
// @Failure 500 ErrorResponse "internal error"
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := -1
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	items := make([]User, 0, len(h.users))
	for _, u := range h.users {
		if limit >= 0 && len(items) >= limit {
			break
		}
		items = append(items, u)
	}
	writeJSON(w, http.StatusOK, pagination.New(items))
}

// Create registers a new user.
//
// @Summary Create a user
// @Description Create a user from the supplied payload.
// @Tags users
// @Accept application/json
// @Param body body CreateUserRequest true "user to create"
// @Success 201 User "the created user"
// @Failure 400 ErrorResponse "invalid payload"
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		apierr.Write(w, http.StatusBadRequest, "email is required")
		return
	}

	h.mu.Lock()
	h.seq++
	id := strconv.Itoa(h.seq)
	u := User{ID: id, Email: req.Email, Name: req.Name, CreatedAt: time.Now().UTC()}
	h.users[id] = u
	h.mu.Unlock()

	writeJSON(w, http.StatusCreated, u)
}

// Get returns a single user by ID.
//
// @Summary Get a user
// @Tags users
// @Success 200 User "the requested user"
// @Failure 404 ErrorResponse "user not found"
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	u, ok := h.users[chi.URLParam(r, "id")]
	h.mu.Unlock()
	if !ok {
		apierr.Write(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// Delete removes a user by ID.
//
// @Summary Delete a user
// @Tags users
// @Success 204 User "deleted"
// @Failure 404 ErrorResponse "user not found"
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.mu.Lock()
	_, ok := h.users[id]
	delete(h.users, id)
	h.mu.Unlock()
	if !ok {
		apierr.Write(w, http.StatusNotFound, "user not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
