// Package self 提供【普通用户自助】管理自己导入的账号/代理/分组的 HTTP 接口（fork 新增）。
//
// 设计要点（见 fork-docs/README.md）：
//   - 挂在现有 JWT 用户路由下（/self/*），admin 路由/handler/gateway 一律不动。
//   - 每个资源有 owner_user_id（归属人）+ is_public（管理可见性）。
//   - 普通用户只能看到"自己的 + 公开的"，只能改/删"自己的"。
//   - 独立脱敏 DTO：绝不返回 credentials / extra / 代理密码等敏感字段。
//   - 自助创建的账号【不绑分组】；是否进共享池由 admin 审核后在后台绑定。
package self

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SelfAccountHandler 处理普通用户自助管理自己账号的请求。
type SelfAccountHandler struct {
	adminService service.AdminService // 复用其 Create/Delete 的完整校验逻辑
	ent          *ent.Client          // 直接做 owner 过滤查询与 owner 写入
}

// NewSelfAccountHandler 构造 SelfAccountHandler（供 wire 注入）。
func NewSelfAccountHandler(adminService service.AdminService, entClient *ent.Client) *SelfAccountHandler {
	return &SelfAccountHandler{adminService: adminService, ent: entClient}
}

// selfAccountDTO 是给普通用户看的脱敏账号视图，绝不含凭证/extra。
type selfAccountDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	IsPublic   bool    `json:"is_public"`
	IsMine     bool    `json:"is_mine"`
	Notes      *string `json:"notes,omitempty"`       // 仅自己的账号返回
	ExpiresAt  *int64  `json:"expires_at,omitempty"`  // unix 秒
	LastUsedAt *int64  `json:"last_used_at,omitempty"`// unix 秒
	CreatedAt  int64   `json:"created_at"`
}

func mapAccount(a *ent.Account, uid int64) selfAccountDTO {
	mine := a.OwnerUserID != nil && *a.OwnerUserID == uid
	d := selfAccountDTO{
		ID:        a.ID,
		Name:      a.Name,
		Platform:  a.Platform,
		Type:      a.Type,
		Status:    a.Status,
		IsPublic:  a.IsPublic,
		IsMine:    mine,
		CreatedAt: a.CreatedAt.Unix(),
	}
	if mine {
		d.Notes = a.Notes
	}
	if a.ExpiresAt != nil {
		v := a.ExpiresAt.Unix()
		d.ExpiresAt = &v
	}
	if a.LastUsedAt != nil {
		v := a.LastUsedAt.Unix()
		d.LastUsedAt = &v
	}
	return d
}

// List 列出普通用户可见的账号。GET /self/accounts?scope=mine|public
//   - mine（默认）：owner == 当前用户
//   - public：别人公开的（is_public 且 owner != 当前用户，含 admin/系统 owner=NULL 的公开账号）
func (h *SelfAccountHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	uid := subject.UserID

	q := h.ent.Account.Query()
	switch strings.ToLower(strings.TrimSpace(c.DefaultQuery("scope", "mine"))) {
	case "public":
		q = q.Where(account.IsPublicEQ(true), account.Not(account.OwnerUserIDEQ(uid)))
	default: // mine
		q = q.Where(account.OwnerUserIDEQ(uid))
	}

	// 软删除由 ent 拦截器自动过滤。上限保护，避免无分页拉全表。
	rows, err := q.Order(ent.Desc(account.FieldCreatedAt)).Limit(500).All(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]selfAccountDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, mapAccount(a, uid))
	}
	response.Success(c, out)
}

// createSelfAccountRequest 是自助创建账号的请求体（字段受限，不含 owner/分组）。
type createSelfAccountRequest struct {
	Name        string         `json:"name" binding:"required"`
	Platform    string         `json:"platform" binding:"required"`
	Type        string         `json:"type" binding:"required"`
	Credentials map[string]any `json:"credentials" binding:"required"`
	Notes       *string        `json:"notes"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	ExpiresAt   *int64         `json:"expires_at"` // unix 秒
	IsPublic    *bool          `json:"is_public"`  // 默认 true
}

// Create 自助创建账号。POST /self/accounts
// 复用 adminService.CreateAccount 的完整校验，随后由服务端写入 owner + is_public。
func (h *SelfAccountHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	uid := subject.UserID

	var req createSelfAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 3
	}
	if req.Priority <= 0 {
		req.Priority = 50
	}

	input := &service.CreateAccountInput{
		Name:        strings.TrimSpace(req.Name),
		Notes:       req.Notes,
		Platform:    strings.TrimSpace(req.Platform),
		Type:        strings.TrimSpace(req.Type),
		Credentials: req.Credentials,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		ExpiresAt:   req.ExpiresAt,
		// 自助导入不绑分组；进池由 admin 审核后在后台绑定。
		SkipDefaultGroupBind: true,
	}

	created, err := h.adminService.CreateAccount(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 写入归属人与可见性（owner 由服务端写死，不信前端）。
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	saved, err := h.ent.Account.UpdateOneID(created.ID).
		SetOwnerUserID(uid).
		SetIsPublic(isPublic).
		Save(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, mapAccount(saved, uid))
}

// updateSelfAccountRequest 自助更新账号（仅有限字段）。
type updateSelfAccountRequest struct {
	Name     *string `json:"name"`
	Notes    *string `json:"notes"`
	IsPublic *bool   `json:"is_public"`
}

// Update 更新自己的账号（名称/备注/可见性）。PATCH /self/accounts/:id
func (h *SelfAccountHandler) Update(c *gin.Context) {
	uid, a, ok := h.loadOwned(c)
	if !ok {
		return
	}
	var req updateSelfAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	upd := h.ent.Account.UpdateOneID(a.ID)
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			response.BadRequest(c, "name cannot be empty")
			return
		}
		upd.SetName(name)
	}
	if req.Notes != nil {
		upd.SetNotes(*req.Notes)
	}
	if req.IsPublic != nil {
		upd.SetIsPublic(*req.IsPublic)
	}
	saved, err := upd.Save(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mapAccount(saved, uid))
}

// Delete 删除自己的账号。DELETE /self/accounts/:id
// 复用 adminService.DeleteAccount（软删除 + 清理），但先做 owner 校验。
func (h *SelfAccountHandler) Delete(c *gin.Context) {
	_, a, ok := h.loadOwned(c)
	if !ok {
		return
	}
	if err := h.adminService.DeleteAccount(c.Request.Context(), a.ID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": a.ID})
}

// loadOwned 解析 :id，加载账号并校验归属于当前用户；否则写好错误响应并返回 ok=false。
// 非 owner（含 admin/系统账号、别人的账号）一律 404，不泄漏存在性。
func (h *SelfAccountHandler) loadOwned(c *gin.Context) (uid int64, a *ent.Account, ok bool) {
	subject, authed := middleware2.GetAuthSubjectFromContext(c)
	if !authed {
		response.Unauthorized(c, "User not authenticated")
		return 0, nil, false
	}
	uid = subject.UserID
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, nil, false
	}
	a, err = h.ent.Account.Get(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "Account not found")
		return 0, nil, false
	}
	if a.OwnerUserID == nil || *a.OwnerUserID != uid {
		response.NotFound(c, "Account not found")
		return 0, nil, false
	}
	return uid, a, true
}
