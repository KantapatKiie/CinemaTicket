package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "USER")
		c.Next()
	})
	r.GET("/admin-only", app.requireRole("ADMIN"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected %d but got %d", http.StatusForbidden, w.Code)
	}
}

func TestRequireRoleAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "ADMIN")
		c.Next()
	})
	r.GET("/admin-only", app.requireRole("ADMIN"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d but got %d", http.StatusOK, w.Code)
	}
}
