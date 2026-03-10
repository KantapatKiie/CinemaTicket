package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

func TestMockLoginSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{jwtSecretBytes: []byte("test-secret")}
	r := gin.New()
	r.POST("/auth/mock", app.mockLogin)

	body := map[string]string{"user_id": "user-1", "role": "ADMIN"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/auth/mock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d but got %d", http.StatusOK, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("cannot parse response: %v", err)
	}
	tokenRaw, ok := resp["token"].(string)
	if !ok || tokenRaw == "" {
		t.Fatalf("missing token in response")
	}
	token, err := jwt.ParseWithClaims(tokenRaw, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("invalid signed token: %v", err)
	}
	claims := token.Claims.(*UserClaims)
	if claims.UserID != "user-1" || claims.Role != "ADMIN" {
		t.Fatalf("unexpected claims: user=%s role=%s", claims.UserID, claims.Role)
	}
}

func TestMockLoginRejectInvalidRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := &App{jwtSecretBytes: []byte("test-secret")}
	r := gin.New()
	r.POST("/auth/mock", app.mockLogin)

	body := map[string]string{"user_id": "user-1", "role": "GUEST"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/auth/mock", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d but got %d", http.StatusBadRequest, w.Code)
	}
}

func TestIsFirebaseToken(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://securetoken.google.com/demo-project","sub":"abc"}`))
	raw := header + "." + payload + ".signature"

	if !isFirebaseToken(raw) {
		t.Fatalf("expected firebase token to be detected")
	}
}

func TestIsFirebaseTokenFalse(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://example.com","sub":"abc"}`))
	raw := header + "." + payload + ".signature"

	if isFirebaseToken(raw) {
		t.Fatalf("expected non-firebase token")
	}
}
