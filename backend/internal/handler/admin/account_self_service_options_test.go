package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerGetSelfServiceOptionsRedactsInfrastructureSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	svc.proxies[0].Username = "proxy-user"
	svc.proxies[0].Password = "proxy-secret"
	svc.groups[0].ModelRouting = map[string][]int64{"claude": {99}}

	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/self-service-options", handler.GetSelfServiceOptions)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/self-service-options", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "proxy-secret")
	require.NotContains(t, recorder.Body.String(), "model_routing")

	var payload struct {
		Data struct {
			Proxies []map[string]any `json:"proxies"`
			Groups  []map[string]any `json:"groups"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Proxies, 1)
	require.Len(t, payload.Data.Groups, 1)
	require.Equal(t, "proxy", payload.Data.Proxies[0]["name"])
	require.NotContains(t, payload.Data.Proxies[0], "password")
	require.Equal(t, "group", payload.Data.Groups[0]["name"])
}
