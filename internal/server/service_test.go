package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// mockExplorerService is a test double for ExplorerService (injected via SetExplorerService).
type mockExplorerService struct {
	searchFunc func(ctx context.Context, query string) (string, map[string]interface{}, error)
	statusFunc func(ctx context.Context) (map[string]interface{}, error)
}

func (m *mockExplorerService) SearchBlockchain(ctx context.Context, query string) (string, map[string]interface{}, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, query)
	}
	return "", nil, ErrNotFound
}

func (m *mockExplorerService) GetNetworkStatus(ctx context.Context) (map[string]interface{}, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx)
	}
	return map[string]interface{}{"ok": true}, nil
}

func TestExplorerService_MockSearchHandlerUsesInterface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := &mockExplorerService{
		searchFunc: func(ctx context.Context, query string) (string, map[string]interface{}, error) {
			if query == "1testaddr" {
				return "address", map[string]interface{}{"mocked": true}, nil
			}
			return "", nil, ErrNotFound
		},
	}
	SetExplorerService(m)
	defer ResetDefaultServices()

	r := gin.New()
	r.GET("/search", searchHandler)

	req := httptest.NewRequest(http.MethodGet, "/search?q=1testaddr", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
