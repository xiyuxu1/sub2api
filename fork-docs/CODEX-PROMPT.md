# 交接 Codex 的提示词（2026-07-19）

> **状态更新**：下方原交接任务已于 2026-07-19 完成，国内生产已部署 `0.1.161-xdl4`（提交 `4340e306e`）。不要重复执行其中的 xdl3 部署或「我的账号」完整自助改造；最新状态以 `fork-docs/README.md` §8.3 为准。

> 直接把下面「===」之间的整段贴给 Codex。它是自包含的：先读仓库里的 `fork-docs/README.md`（尤其 §7 部署、§8.2/§8.3），再干活。

===

你在接手一个 Claude/Codex 订阅号聚合网关的 fork（`xiyuxu1/sub2api`）。仓库在本机
`/Users/xudelong/mine/sub2api/sub2api`，工作分支 `xdl/self-service`（生产从这个分支部署）。
**动手前先完整读 `fork-docs/README.md`，重点 §7（构建/部署，踩过大坑）、§8.2、§8.3、§8.5。**

## 背景一句话
个人+几个信任的朋友拼车共用，不收费。普通用户要能「自助管理自己的上游账号」而不给 admin 权限。
后端已按 owner 收口（`backend/internal/...` 的 account service/repo，非本人改删 → 404）。

## 现在的状态
- 账号自助 + is_public「公开/私有」开关：后端已上线生产 `0.1.161-xdl2`（国内节点 `115.159.205.56`）。
- 最新分支已修：①「我的账号」页 `MyAccountsView.vue` 套上 `<AppLayout>`（导航栏回来）；
  ②删掉多余的「公开账号」浏览 tab；③保留每行「公开/私有」开关。**但这些还没重新部署**，
  生产上仍是旧版（带多余 tab、无导航）。最新提交见 `git log -1`。

## 任务 1（先做，低风险）：把当前分支的修复部署上线
严格照 `fork-docs/README.md` §7：
1. ⛔ **绝不在生产机 `docker build`**（会 OOM 打爆，已停机过一次）。构建只用 GitHub Actions。
2. 打 tag `v0.1.161-xdl3`（指向当前 `xdl/self-service` HEAD）→ push →
   `gh workflow run release.yml --repo xiyuxu1/sub2api --ref xdl/self-service -f tag=v0.1.161-xdl3 -f simple_release=true` → 等构建完成。
3. SSH `ubuntu@115.159.205.56`：用 `systemd-run` + 南大镜像站 `ghcr.nju.edu.cn` 拉镜像（§7.3，
   直连 ghcr.io 龟速、且 docker pull 经 SSH 会被断连带死）→ 重命名 → 备份 compose+DB →
   `sed` 切 compose 的 image 行 `0.1.161-xdl2`→`0.1.161-xdl3` → `docker compose up -d sub2api` →
   健康检查 `curl localhost:8080/health`。回滚：image 行改回 xdl2。
4. git 走 origin **HTTPS**（gh 活跃账号 `xiyuxu1`；SSH 会 denied）。别提交本机 pnpm 产物（§7.1）。

## 任务 2（大活）：把「我的账号」升级为完整自助 —— 见 §8.3
用户要：保留独立「我的账号」菜单，但把管理员账号管理页 `frontend/src/views/admin/AccountsView.vue`
（2087 行）的**大部分功能迁过来**，让普通用户对自己的号有完整所有权（换分组、绑代理、改并发/
优先级/配额、编辑凭据等），只去掉极敏感的基础设施操作（批量、重置配额、CRS 同步、计费探测等）。

### 🔴 动手前必须先让用户拍板一个安全决策（别自己决定）
上个会话把「非 admin 建号**不进分组、不绑代理**」当 **P0 越权修复**写死了
（`backend/internal/service/admin_account.go` 的 CreateAccount/UpdateAccount 里
`if _, ok := authctx.NonAdminOwner(ctx); ok { input.GroupIDs=nil; input.ProxyID=nil }`）。
原因：**分组=共享调度池**，普通用户若把指向自己可控上游/代理的号绑进共享分组，该号会被拿去服务
**别人**的 API 流量 → 能截获别人 prompt。用户要的「换分组/绑代理」与此**直接冲突**。
- 让用户在两个姿态里选：
  - **A 保守**：只能绑自己 owner 的私有资源，禁止绑进含别人账号/服务公共流量的共享分组；
    需给分组/代理也做 owner+可见性收口，bind 时校验目标 owner=自己。
  - **B 放宽**：信任朋友，允许进共享池，但用户知情接受「成员账号可能被路由到别人流量」的风险。
- ⛔ **用户没明确选 A/B 之前，不要动那两个 `=nil` clamp**，否则重新捅穿 P0。

### 迁移做法（姿态定了之后）
优先让「我的账号」复用/内嵌 `AccountsView.vue` 的表格与操作，按角色隐藏 admin-only 控件；
或把 `AccountsView` 抽成可配置组件。**逐一过 2087 行里每个操作**分类（用户可用 / admin-only 隐藏）。
每放开一个操作都要确认对应后端端点在 service/repo 已按 owner 收口，并覆盖旁路（关联 ID/批量/导入/
OAuth）。参照 §2/§3 的授权规则与「必须覆盖的旁路」清单。改完按 §7 全流程构建部署，用普通用户+admin
两种 token 跑越权测试。

### 其余待办（§8.4）
- OpenAI/Codex/Gemini/Grok 等独立 OAuth 组对普通用户放开（仿账号组做 jwtAuth 放开）。
- P2 代理、P3 分组：同款 owner 收口就地改（列已就绪；代理 DTO 返回密码明文，普通用户视图必须脱敏）。
- 后端 `scope=public`/`ListPublicAccounts`/`dto.PublicAccount` 是本会话留的无调用死代码，可清。
- P4 海外节点 `70.39.194.149`（/opt/sub2api-deploy）：稳定后同样切镜像部署。

## 关键避坑（§8.5）
⛔ 别在生产机 build；拉镜像走南大站+systemd-run；别提交本机 pnpm 产物；git 走 HTTPS；
合并上游按 §5（ent 生成码不手工解冲突、只 `go generate ./ent/...` 重生成；迁移文件不动）。

===
