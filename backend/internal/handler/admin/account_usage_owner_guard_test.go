package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ownerDenyAdminService struct {
	*stubAdminService
}

func (s *ownerDenyAdminService) GetAccount(context.Context, int64) (*service.Account, error) {
	return nil, service.ErrAccountNotFound
}

func TestAccountUsageHandlersRejectNonOwnedAccountBeforeReadingStats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &ownerDenyAdminService{stubAdminService: newStubAdminService()}
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/accounts/:id/stats", handler.GetStats)
	router.GET("/accounts/:id/usage", handler.GetUsage)
	router.GET("/accounts/:id/today-stats", handler.GetTodayStats)
	router.POST("/accounts/today-stats/batch", handler.GetBatchTodayStats)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "stats", method: http.MethodGet, path: "/accounts/42/stats"},
		{name: "usage", method: http.MethodGet, path: "/accounts/42/usage"},
		{name: "today", method: http.MethodGet, path: "/accounts/42/today-stats"},
		{name: "batch today", method: http.MethodPost, path: "/accounts/today-stats/batch", body: `{"account_ids":[42]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}
