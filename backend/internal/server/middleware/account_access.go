package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/authctx"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// accountOwnerSafeRoutes 是允许【普通用户】访问的账号端点白名单（fork）。
// 默认拒绝：不在表内的账号端点（批量、crs 同步、models 同步、上游计费探测设置、
// reset-quota、set-privacy、schedulable 等纯管理/基础设施操作）非管理员一律 403。
// 这样上游将来新增的账号端点在被显式加入前，对普通用户默认不可见，前向安全。
//
// key = METHOD + " " + FullPath()。owner 收口由账号 repository 依据 authctx 完成：
// 列表只见自己+public，按 id 取/改/删只作用于自己的，建号自动归属自己。
var accountOwnerSafeRoutes = map[string]struct{}{
	"GET /api/v1/admin/accounts":                       {},
	"POST /api/v1/admin/accounts":                      {},
	"GET /api/v1/admin/accounts/self-service-options":  {},
	"GET /api/v1/admin/accounts/:id":                   {},
	"PUT /api/v1/admin/accounts/:id":                   {},
	"DELETE /api/v1/admin/accounts/:id":                {},
	"POST /api/v1/admin/accounts/import/codex-session": {},
	"POST /api/v1/admin/accounts/import/data":          {},
	// B 模式允许选择共享分组，因此放行只返回渠道冲突摘要的预检；duplicate 仍会
	// 连带复制 source 的绑定与配置，保持 admin-only。
	"POST /api/v1/admin/accounts/check-mixed-channel":              {},
	"POST /api/v1/admin/accounts/:id/test":                         {},
	"POST /api/v1/admin/accounts/:id/refresh":                      {},
	"POST /api/v1/admin/accounts/:id/apply-oauth-credentials":      {},
	"GET /api/v1/admin/accounts/:id/stats":                         {},
	"GET /api/v1/admin/accounts/:id/usage":                         {},
	"GET /api/v1/admin/accounts/:id/today-stats":                   {},
	"POST /api/v1/admin/accounts/today-stats/batch":                {},
	"GET /api/v1/admin/accounts/:id/models":                        {},
	"GET /api/v1/admin/accounts/antigravity/default-model-mapping": {},
	// stats / today-stats / today-stats/batch 已在 handler 先走 owner 化 GetAccount，
	// 因此可供“我的账号”展示实时用量；clear-error 等写操作仍保持 admin-only。
	// Claude/Anthropic OAuth 与 setup-token 导入流程（无账号归属，仅生成 URL / 交换 code；
	// 最终建号仍走上面 owner 收口的 POST /accounts）。
	"POST /api/v1/admin/accounts/generate-auth-url":         {},
	"POST /api/v1/admin/accounts/generate-setup-token-url":  {},
	"POST /api/v1/admin/accounts/exchange-code":             {},
	"POST /api/v1/admin/accounts/exchange-setup-token-code": {},
	"POST /api/v1/admin/accounts/cookie-auth":               {},
	"POST /api/v1/admin/accounts/setup-token-cookie-auth":   {},
}

// AccountAccessMiddleware 账号子树门卫（fork）：替代账号路由上原有的 AdminOnly 语义。
// 必须挂在 JWTAuth 之后（依赖其写入的 user id / role）。
//   - 管理员：不受限，注入 admin actor 后放行（repo 层不加 owner 过滤）。
//   - 普通用户：仅白名单内端点放行，其余 403；注入非 admin actor，repo 层据此按 owner 收口。
func AccountAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}
		role, _ := GetUserRoleFromContext(c)
		isAdmin := role == service.RoleAdmin

		if !isAdmin {
			routeKey := c.Request.Method + " " + c.FullPath()
			if _, allowed := accountOwnerSafeRoutes[routeKey]; !allowed {
				AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
				return
			}
		}

		// 把操作者写入请求 context，供 account repository 做 owner 收口。
		ctx := authctx.WithActor(c.Request.Context(), authctx.Actor{UserID: subject.UserID, IsAdmin: isAdmin})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// AccountActorMiddleware only propagates the authenticated actor into request context.
// It is used by explicitly registered self-service-safe OAuth/quota routes which do not
// need the account endpoint whitelist but still require repository owner scoping.
func AccountActorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
			return
		}
		role, _ := GetUserRoleFromContext(c)
		ctx := authctx.WithActor(c.Request.Context(), authctx.Actor{
			UserID:  subject.UserID,
			IsAdmin: role == service.RoleAdmin,
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
