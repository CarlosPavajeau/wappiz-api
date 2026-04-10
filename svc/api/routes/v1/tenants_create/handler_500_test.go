//go:build integration

package tenants_create

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"wappiz/internal/testutil/integrationtest"

	"github.com/gin-gonic/gin"
)

func TestHandle_ReturnsInternalServerError_WhenDatabaseUnavailable(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := integrationtest.RequireDatabase(t)

	if err := database.Close(); err != nil {
		t.Fatalf("failed to close database before test: %v", err)
	}

	h := &Handler{DB: database}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "user-int-500")
		c.Next()
	})
	r.POST("/v1/tenants", h.Handle)

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"name":"Failure Barber"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d, body=%s", http.StatusInternalServerError, w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON body: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("expected error field in response body, got %v", body)
	}
}
