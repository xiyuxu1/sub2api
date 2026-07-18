// Package authctx 在请求 context 里携带"操作者"身份（fork 新增）。
//
// 用途：让普通用户也能使用现有 admin 账号功能，但在 repository 层按归属人(owner)自动收口。
// HTTP 中间件把当前登录用户写入 context；账号 repo 读取它决定：
//   - 管理员(IsAdmin=true) 或 无 Actor（后台/网关等 background context）：不受限；
//   - 普通用户：列表只见自己+public，按 id 取/改/删只作用于自己的，建号自动归属自己。
//
// 只在 HTTP 请求链路注入；background context（context.Background）不带 Actor，
// 因此调度器、网关等内部路径不受影响。
package authctx

import "context"

// Actor 是当前请求的操作者身份。
type Actor struct {
	UserID  int64
	IsAdmin bool
}

type actorKey struct{}

// WithActor 返回携带 actor 的新 context。
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// FromContext 取出 actor；ok=false 表示这是不受限的内部/后台上下文。
func FromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// NonAdminOwner 返回需要按 owner 收口的用户 id：仅当存在 actor 且非管理员时返回 (uid, true)。
// 管理员或无 actor 时返回 (0, false)，调用方据此不加 owner 过滤。
func NonAdminOwner(ctx context.Context) (int64, bool) {
	a, ok := FromContext(ctx)
	if !ok || a.IsAdmin {
		return 0, false
	}
	return a.UserID, true
}
