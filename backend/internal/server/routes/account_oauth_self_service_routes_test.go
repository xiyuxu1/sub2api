package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountOAuthSelfServiceRoutesIncludeEveryCreateFlowUsedByAccountModal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		OpenAIOAuth:      &adminhandler.OpenAIOAuthHandler{},
		GeminiOAuth:      &adminhandler.GeminiOAuthHandler{},
		AntigravityOAuth: &adminhandler.AntigravityOAuthHandler{},
		GrokOAuth:        &adminhandler.GrokOAuthHandler{},
	}}
	registerAccountOAuthSelfServiceRoutes(router.Group("/api/v1/admin"), handlers)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		"POST /api/v1/admin/openai/create-from-oauth",
		"POST /api/v1/admin/openai/create-from-codex-pat",
		"POST /api/v1/admin/grok/oauth/create-from-oauth",
		"POST /api/v1/admin/grok/sso-to-oauth",
	} {
		_, ok := routes[expected]
		require.Truef(t, ok, "missing self-service account creation route %s", expected)
	}
}
