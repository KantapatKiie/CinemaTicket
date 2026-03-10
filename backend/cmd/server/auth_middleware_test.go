package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthMiddlewareMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{cfg: Config{}, jwtSecretBytes: []byte("test-secret")}
	r := gin.New()
	r.GET("/protected", app.authMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d but got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{cfg: Config{}, jwtSecretBytes: []byte("test-secret")}
	r := gin.New()
	r.GET("/protected", app.authMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d but got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddlewareValidJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{cfg: Config{}, jwtSecretBytes: []byte("test-secret")}
	r := gin.New()
	r.GET("/protected", app.authMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("user_id"), "role": c.GetString("role")})
	})

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{
		UserID: "user-99",
		Role:   "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	raw, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("cannot sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d but got %d", http.StatusOK, w.Code)
	}
}
