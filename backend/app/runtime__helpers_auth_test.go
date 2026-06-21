package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock runtime settings store with a test token
	testToken := "test-access-token-12345"
	mockStore := &runtimeState{}
	mockStore.settings.AccessToken = testToken
	originalStore := runtimeSettingsStore
	runtimeSettingsStore = mockStore
	defer func() { runtimeSettingsStore = originalStore }()

	tests := []struct {
		name           string
		releaseMode    bool
		disableAuth    string
		authHeader     string
		queryKey       string
		bearerToken    string
		expectedStatus int
	}{
		{
			name:           "Valid X-API-KEY header",
			releaseMode:    true,
			authHeader:     testToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid query parameter",
			releaseMode:    true,
			queryKey:       testToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid Bearer token",
			releaseMode:    true,
			bearerToken:    testToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid token",
			releaseMode:    true,
			authHeader:     "wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Missing token",
			releaseMode:    true,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Auth disabled via env",
			releaseMode:    true,
			disableAuth:    "true",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Case sensitivity",
			releaseMode:    true,
			authHeader:     "TEST-ACCESS-TOKEN-12345",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Dev mode - no auth required",
			releaseMode:    false,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.releaseMode {
				gin.SetMode(gin.ReleaseMode)
			} else {
				gin.SetMode(gin.DebugMode)
			}
			defer gin.SetMode(gin.TestMode)

			if tt.disableAuth != "" {
				os.Setenv("DISABLE_AUTH", tt.disableAuth)
				defer os.Unsetenv("DISABLE_AUTH")
			}

			router := gin.New()
			router.Use(authMiddleware())
			router.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("X-API-KEY", tt.authHeader)
			}
			if tt.bearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+tt.bearerToken)
			}
			if tt.queryKey != "" {
				req.URL.RawQuery = "key=" + tt.queryKey
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	defer gin.SetMode(gin.TestMode)

	mockStore := &runtimeState{}
	mockStore.settings.AccessToken = ""
	originalStore := runtimeSettingsStore
	runtimeSettingsStore = mockStore
	defer func() { runtimeSettingsStore = originalStore }()

	router := gin.New()
	router.Use(authMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-KEY", "any-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 when empty token configured, got %d", w.Code)
	}
}

type mockRuntimeSettingsStore struct {
	token string
}

func (m *mockRuntimeSettingsStore) ExpectedToken() string {
	return m.token
}
