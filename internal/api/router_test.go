package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIAuthMiddleware_RejectsMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/ping", apiAuthMiddleware([]string{"secret"}, nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIAuthMiddleware_AllowsAPIKeyHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/ping", apiAuthMiddleware([]string{"secret"}, nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for name, value := range map[string]string{
		"X-API-Key":     "secret",
		"Authorization": "Bearer secret",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set(name, value)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", name, rec.Code)
		}
	}
}

func TestAPIAuthMiddleware_AllowsMultipleKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/ping", apiAuthMiddleware([]string{"client-a", "client-b"}, nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, key := range []string{"client-a", "client-b"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("X-API-Key", key)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("key %q status = %d, want 204", key, rec.Code)
		}
	}
}

func TestAPIAuthMiddleware_AllowsWhitelistedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/ping", apiAuthMiddleware([]string{"secret"}, []string{"198.51.100.10", "203.0.113.0/24"}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		remoteAddr string
		header     string
		want       int
	}{
		{name: "direct ip", remoteAddr: "198.51.100.10:1234", want: http.StatusNoContent},
		{name: "cidr forwarded ip", remoteAddr: "192.0.2.10:1234", header: "203.0.113.55", want: http.StatusNoContent},
		{name: "not whitelisted", remoteAddr: "192.0.2.10:1234", want: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.RemoteAddr = tt.remoteAddr
		if tt.header != "" {
			req.Header.Set("X-Forwarded-For", tt.header)
		}
		router.ServeHTTP(rec, req)

		if rec.Code != tt.want {
			t.Fatalf("%s status = %d, want %d", tt.name, rec.Code, tt.want)
		}
	}
}
