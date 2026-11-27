package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSwaggerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := gin.New()
	initSwagger(g)
	g.GET("/v1/ping", apiAuthMiddleware([]string{"secret"}, nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return g
}

func TestSwaggerServedWithoutAuth(t *testing.T) {
	g := newSwaggerRouter()

	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index.html status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("doc.json status = %d, want 200", rec.Code)
	}
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("doc.json is not json: %v", err)
	}
	for _, p := range []string{"/v1/mos/list", "/v1/block", "/v1/event/listening"} {
		if _, ok := doc.Paths[p]; !ok {
			t.Errorf("doc.json missing path %s", p)
		}
	}
}

func TestSwaggerDoesNotOpenV1(t *testing.T) {
	g := newSwaggerRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/v1 status = %d, want 401", rec.Code)
	}
}
