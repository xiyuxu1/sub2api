// Package self 提供【普通用户自助】管理自己导入的账号/代理/分组的 HTTP 接口（fork 新增）。
//
// 设计要点（见 fork-docs/README.md）：
//   - 挂在现有 JWT 用户路由下（/self/*），admin 路由/handler/gateway 一律不动。
//   - 每个资源有 owner_user_id（归属人）+ is_public（管理可见性）。
//   - 普通用户只能看到"自己的 + 公开的"，只能改/删"自己的"。
//   - 独立脱敏 DTO：绝不返回 credentials / extra / 代理密码等敏感字段；
//     public 视图用更精简的 DTO，避免泄漏使用节奏/到期时间。
//   - 写操作走 owner 条件的【原子】ent 变更（WHERE id AND owner_user_id），不做先查后写。
//   - 删除【不复用】admin 的级联删除（admin 版会连带删 admin 拥有的 spark shadow）；
//     改为 owner 条件软删，且拒绝删除仍挂有子账号(shadow)的父账号。
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
	adminService service.AdminService // 仅复用其 CreateAccount 的完整校验逻辑
	ent          *ent.Client          // owner 过滤查询与 owner 条件原子写
}

// NewSelfAccountHandler 构造 SelfAccountHandler（供 wire 注入）。
func NewSelfAccountHandler(adminService service.AdminService, entClient *ent.Client) *SelfAccountHandler {
	return &SelfAccountHandler{adminService: adminService, ent: entClient}
}

// selfAccountDTO 是账号 owner 自己看的脱敏视图，绝不含凭证/extra。
type selfAccountDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	IsPublic   bool    `json:"is_public"`
	IsMine     bool    `json:"is_mine"`
	Notes      *string `json:"notes,omitempty"`
	ExpiresAt  *int64  `json:"expires_at,omitempty"`   // unix 秒
	LastUsedAt *int64  `json:"last_used_at,omitempty"` // unix 秒
	CreatedAt  int64   `json:"created_at"`
}

// publicAccountDTO 是别人公开账号的最小摘要：只够知道"存在一个某平台的共享账号"。
// 刻意不含 status / last_used_at / expires_at / notes，避免泄漏凭证健康与使用节奏。
type publicAccountDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Type     string `json:"type"`
	IsPublic bool   `json:"is_public"`
	IsMine   bool   `json:"is_mine"` // 恒 false
}

func mapOwnerAccount(a *ent.Account) selfAccountDTO {
	d := selfAccountDTO{
		ID:        a.ID,
		Name:      a.Name,
		Platform:  a.Platform,
		Type:      a.Type,
		Status:    a.Status,
		IsPublic:  a.IsPublic,
		IsMine:    true,
		Notes:     a.Notes,
		CreatedAt: a.CreatedAt.Unix(),
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

func mapPublicAccount(a *ent.Account) publicAccountDTO {
	return publicAccountDTO{
		ID:       a.ID,
		Name:     a.Name,
		Platform: a.Platform,
		Type:     a.Type,
		IsPublic: a.IsPublic,
		IsMine:   false,
	}
}

// List 列出普通用户可见的账号。GET /self/accounts?scope=mine|public
//   - mine（默认）：owner == 当前用户，返回完整脱敏视图。
//   - public：别人公开的（is_public 且 owner 为空或非当前用户），返回最小摘要。
func (h *SelfAccountHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	uid := subject.UserID
	ctx := c.Request.Context()

	if strings.EqualFold(strings.TrimSpace(c.DefaultQuery("scope", "mine")), "public") {
		// 显式处理 NULL：owner 为空（系统/admin）或 owner != 我，且 is_public。
		rows, err := h.ent.Account.Query().
			Where(
				account.IsPublicEQ(true),
				account.Or(account.OwnerUserIDIsNil(), account.OwnerUserIDNEQ(uid)),
			).
			Order(ent.Desc(account.FieldCreatedAt)).Limit(500).All(ctx)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		out := make([]publicAccountDTO, 0, len(rows))
		for _, a := range rows {
			out = append(out, mapPublicAccount(a))
		}
		response.Success(c, out)
		return
	}

	rows, err := h.ent.Account.Query().
		Where(account.OwnerUserIDEQ(uid)).
		Order(ent.Desc(account.FieldCreatedAt)).Limit(500).All(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]selfAccountDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, mapOwnerAccount(a))
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
// owner 写入失败时补偿删除刚建的账号，避免留下无主孤儿。
func (h *SelfAccountHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	uid := subject.UserID
	ctx := c.Request.Context()

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

	created, err := h.adminService.CreateAccount(ctx, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	saved, err := h.ent.Account.UpdateOneID(created.ID).
		SetOwnerUserID(uid).
		SetIsPublic(isPublic).
		Save(ctx)
	if err != nil {
		// 补偿：删掉刚建成、owner 尚未落库的账号，避免无主孤儿。
		_ = h.ent.Account.DeleteOneID(created.ID).Exec(ctx)
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, mapOwnerAccount(saved))
}

// updateSelfAccountRequest 自助更新账号（仅有限字段）。
type updateSelfAccountRequest struct {
	Name     *string `json:"name"`
	Notes    *string `json:"notes"`
	IsPublic *bool   `json:"is_public"`
}

// Update 更新自己的账号（名称/备注/可见性）。PATCH /self/accounts/:id
// owner 条件原子更新：WHERE id AND owner_user_id。命中 0 行 → 404（含非 owner/已删）。
func (h *SelfAccountHandler) Update(c *gin.Context) {
	uid, id, ok := subjectAndID(c)
	if !ok {
		return
	}
	var req updateSelfAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()

	upd := h.ent.Account.Update().Where(account.IDEQ(id), account.OwnerUserIDEQ(uid))
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
	n, err := upd.Save(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if n == 0 {
		response.NotFound(c, "Account not found")
		return
	}
	a, err := h.ent.Account.Get(ctx, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mapOwnerAccount(a))
}

// Delete 删除自己的账号。DELETE /self/accounts/:id
// 不走 admin 级联删除（那会连带删 admin 的 spark shadow）：
//   - 若账号仍挂有子账号(shadow)，拒绝并提示联系管理员；
//   - 否则按 owner 条件软删（ent 软删钩子转 UPDATE，不触发级联）。
func (h *SelfAccountHandler) Delete(c *gin.Context) {
	uid, id, ok := subjectAndID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	hasChildren, err := h.ent.Account.Query().
		Where(account.IDEQ(id), account.OwnerUserIDEQ(uid)).
		QueryChildren().Exist(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if hasChildren {
		response.Forbidden(c, "Account has linked resources; contact an administrator to delete it")
		return
	}

	n, err := h.ent.Account.Delete().
		Where(account.IDEQ(id), account.OwnerUserIDEQ(uid)).
		Exec(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if n == 0 {
		response.NotFound(c, "Account not found")
		return
	}
	response.Success(c, gin.H{"id": id})
}

// subjectAndID 取当前用户 id 与路径 :id；失败时已写好错误响应。
func subjectAndID(c *gin.Context) (uid int64, id int64, ok bool) {
	subject, authed := middleware2.GetAuthSubjectFromContext(c)
	if !authed {
		response.Unauthorized(c, "User not authenticated")
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, 0, false
	}
	return subject.UserID, id, true
}
