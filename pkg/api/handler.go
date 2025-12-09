package api

import (
	"encoding/json"
	"net/http"

	"github.com/yourusername/luminate/pkg/config"
	"github.com/yourusername/luminate/pkg/storage"
)

// Handler handles HTTP API requests
type Handler struct {
	store  storage.MetricsStore
	config *config.Config
}

// NewHandler creates a new API handler
func NewHandler(store storage.MetricsStore, cfg *config.Config) *Handler {
	return &Handler{
		store:  store,
		config: cfg,
	}
}

// Health handles health check requests
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"version": "dev",
	})
}

// Metrics handles Prometheus metrics endpoint
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("# TODO: Implement Prometheus metrics\n"))
}

// Query handles metric query requests
func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Query endpoint - not yet implemented",
	})
}

// Write handles metric write requests
func (h *Handler) Write(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Write endpoint - not yet implemented",
	})
}

// ListMetrics handles list metrics requests
func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]string{})
}

// MetricsDimensions handles dimension discovery requests
func (h *Handler) MetricsDimensions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]string{})
}

// Admin handles admin requests
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Admin endpoint - not yet implemented",
	})
}
