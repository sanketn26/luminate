package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourusername/luminate/pkg/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{}
	handler := NewHandler(nil, cfg)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
