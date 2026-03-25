package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCORS_AllowOriginHeader(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestCORS_AllowMethodsHeader(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	want := "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	got := w.Header().Get("Access-Control-Allow-Methods")
	if got != want {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, want)
	}
}

func TestCORS_AllowHeadersContainsContentType(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Headers")
	if got != "Content-Type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type")
	}
}

func TestCORS_MaxAgeHeader(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Max-Age")
	if got != "43200" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", got, "43200")
	}
}

func TestCORS_OptionsReturns204AndAbortsChain(t *testing.T) {
	r := gin.New()
	r.Use(CORS())

	downstreamCalled := false
	r.OPTIONS("/test", func(c *gin.Context) {
		downstreamCalled = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if downstreamCalled {
		t.Error("downstream handler should not be called for OPTIONS preflight")
	}
}

func TestCORS_NonOptionsCallsDownstream(t *testing.T) {
	r := gin.New()
	r.Use(CORS())

	downstreamCalled := false
	r.GET("/test", func(c *gin.Context) {
		downstreamCalled = true
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if !downstreamCalled {
		t.Error("downstream handler should be called for GET requests")
	}
	if w.Code != http.StatusOK {
		t.Errorf("GET status = %d, want %d", w.Code, http.StatusOK)
	}
}
