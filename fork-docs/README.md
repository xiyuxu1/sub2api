# Fork 二次开发文档（xiyuxu1/sub2api）

> 这是相对上游 `Wei-Shaw/sub2api` 的**下游自定义改动**说明。新会话/新 AI 接手时先读本文件。
> 目录 `fork-docs/` 是本 fork 新增的、上游没有的目录，不会和上游冲突。

## 0. 一句话背景
个人 + 几个**信任的朋友**拼车共用，把多个 Claude/Codex 订阅号聚合成统一 API。**不收费、不接支付**。生产部署在两台服务器（详见 `/Users/xudelong/mine/sub2api/` 下的运维手册与资产清单）。

## 1. 需求
让普通用户（朋友）能**自助导入并管理自己的账号 / 代理 / 分组**，而**不给 admin 权限**：
- 私有资源对**其他普通用户不可见**；**admin 恒可见所有**。
- 普通用户**删不掉 admin 或别人导入**的资源。
- 每个资源有"公开/私有"开关。

## 2. 架构决策（与 Codex 评审结论综合后）
- **可见性语义 = A：只控"后台管理可见性"，不碰 gateway 调度/流量。** 私有=别的普通用户在后台看不到/改不了；不改变哪些账号被拿去服务 API 流量。字段语义是"管理可见性/可发现性"，不是"流量私有"。
- **实现方式 = 新增 `/self` 窄接口，admin / gateway / 现有 admin handler 一行不动。** 这样冲突面最小（改的是新增文件，不是上游高频改的 admin 页面）。
  - 原因：`/admin` 整组前置 `AdminAuthMiddleware`，普通用户到不了；且 admin 账号/代理接口混着导入导出、批量改凭证、OAuth、重置配额等危险操作，代理的 admin DTO 甚至直接返回密码明文——**绝不能复用 admin 接口/DTO**。
- **三类资源（账号 / 代理 / 分组）统一同样处理**：都加 owner + 可见性，都开 `/self` 自助。分组之所以也能开放，是因为本项目**没用到分组的计费倍率/RPM/fallback/模型路由等付费/策略功能**，分组在这里只是普通归类，风险等同账号。（⚠️ 若将来启用了分组的计费/路由策略，需回头把这些字段重新锁回 admin。）

### 数据模型
三张表 `accounts` / `groups` / `proxies` 各加两列：
| 列 | 类型 | 说明 |
|---|---|---|
| `owner_user_id` | `BIGINT NULL` | 资源归属人。**NULL = 系统/admin 历史导入**。新建时由**服务端**写入当前登录 user id，**不接受请求体赋值**。 |
| `is_public` | `BOOLEAN NOT NULL DEFAULT true` | 公开=其他普通用户可见（只读摘要）。**默认 true**（都是自己人，默认透明）。 |

> owner 语义保持简单：朋友不是攻击者，不做 ON DELETE RESTRICT 之类的重防护。owner 检查只为实现"删不掉别人的"这个功能本身。默认 public=true 意味着上线后 admin 现有账号对朋友可见（只读，删改不了）——这是可接受的、符合拼车透明预期。

### 授权规则
- 读：owner 或 admin → 完整（脱敏后）详情；其他普通用户 → 仅当 `is_public` 且只给**最小摘要**（绝不含 credentials / 代理密码 / extra / 错误详情）。
- 写（改/删）：仅 owner（admin 走原有 admin 接口）。用**原子** `UPDATE/DELETE ... WHERE id=? AND owner_user_id=?`，避免先查后写竞态。非 owner → 403，天然满足"删不掉 admin/别人的"。
- 必须覆盖的旁路（否则越权）：OAuth 建号（state 要签名绑定 user id、一次性、短 TTL，回调不信前端传的 owner）、Codex 导入、data import/export、batch/bulk、refresh/test、stats、代理质量检测，以及所有关联 ID（account.group_ids / account.proxy_id / proxy.backup_proxy_id）。

## 3. 改动清单（全部为新增文件，尽量不碰上游文件）
后端：
- `backend/migrations/9000_xdl_resource_visibility.sql` — 加上述两列 + 索引。**fork 专属命名，上线后永不重命名/改内容**（迁移 runner 按完整文件名 + SHA256 记录）。
- `backend/ent/schema/{account,group,proxy}.go` — 各加两个字段（**这是必须改的上游文件**）。改后 `go generate ./...` 重新生成 ent 代码；生成代码冲突时**不手工解，按最终 schema 重跑**。
- `backend/internal/server/routes/self.go` — 在现有 JWT 用户路由下注册 `/self/accounts`、`/self/proxies`、`/self/groups`。
- `backend/internal/handler/self/*.go` — list / create / update / delete（原子 owner guard）。
- `backend/internal/handler/dto/self_*.go` — 独立脱敏 DTO（public 摘要 vs owner 详情）。

前端：
- `frontend/src/views/self/` + `frontend/src/api/self/` — 三个 `requiresAdmin:false` 页面（My Accounts/Proxies/Groups）+ 独立 API client + 导入弹窗加"公开/私有"开关。尽量不改现有 admin 页面。

## 4. 分期
- **P1 账号**：schema+迁移+`/self/accounts` 全套+导入+脱敏 DTO+前端 → 端到端跑通，并真实体验一次 upstream merge。
- **P2 代理**：同模式 + 删除保护。
- **P3 分组**：同账号处理。

## 5. 如何跟随上游更新 / 合并代码（新会话必读）

### remotes 与身份
- `origin` = `https://github.com/xiyuxu1/sub2api.git`（你的 fork，**用 HTTPS**）。
- `upstream` = `git@github.com:Wei-Shaw/sub2api.git`（官方）。
- ⚠️ **身份坑**：本机 SSH key 解析成 `xudp97`，对 `xiyuxu1` fork **没写权限**，SSH push 会 `denied to xudp97`。`gh` 里活跃账号是 `xiyuxu1`，所以 origin 走 HTTPS（用 gh 凭据）。
- ⚠️ **workflow scope 坑**：上游改了 `.github/workflows/`，同步/推送需要 token 有 `workflow` scope。若 `gh repo sync` 报缺 scope：`gh auth refresh -s workflow -h github.com`（人工授权，AI 代不了）。（2026-07 已授权过。）

### 分支策略
- `main` 保持为**干净的 upstream 镜像**，只用来 fetch 上游。
- 自定义改动放长期主题分支（建议 `xdl/self-service`），**生产从这个分支部署**。
- 开 `git config rerere.enabled true`，让重复冲突自动复用解法。
- 下游改动尽量压成少量主题提交，别把改动摊成一堆散提交。

### 更新流程
```bash
cd /Users/xudelong/mine/sub2api/sub2api
git fetch upstream main
git checkout main && git merge --ff-only upstream/main        # main 跟上上游
git push origin main                                          # 更新 GitHub fork（走 HTTPS/gh）
# 或直接 GitHub 网页 Sync fork / gh repo sync xiyuxu1/sub2api --source Wei-Shaw/sub2api

git checkout xdl/self-service
git merge main                                                # 把上游合进你的改动分支
# 解冲突：ent 生成代码不手工解 → go generate ./... 重跑；迁移文件保持不动
cd backend && go generate ./... && go build ./... && go test ./internal/handler/self/...
cd ../frontend && pnpm build   # 或 npm，按仓库为准
```

### 合并时的高风险点
1. `backend/ent/schema/{account,group,proxy}.go` 三个改过的上游文件——最可能冲突，冲突后重跑 `go generate`。
2. 迁移 runner / DTO / 路由注册若上游动过，检查 `/self` 注册点是否还在。
3. 合并后务必跑一遍 owner 越权测试（非 owner 删/改应 403；public 摘要不漏凭证）。

## 6. Codex 评审要点（结论留存，原文不留）
- 认同选 A；否决"替换 AdminOnly / 复用 admin CRUD"，改用独立 `/self` + 独立脱敏 DTO + service 层原子授权。
- 一个中间件不足以做授权边界，必须下沉到 service/repo 并覆盖 OAuth/导入/批量/关联 ID。
- （Codex 原建议 `is_public` 默认 false、owner 加重防护——本项目因"都是朋友、不收费"**主动放宽为默认 true、owner 从简**，属知情取舍。）
