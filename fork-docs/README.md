# Fork 二次开发文档（xiyuxu1/sub2api）

> 这是相对上游 `Wei-Shaw/sub2api` 的**下游自定义改动**说明。新会话/新 AI 接手时先读本文件。
> 目录 `fork-docs/` 是本 fork 新增的、上游没有的目录，不会和上游冲突。
>
> **新会话接手顺序**：先读 §8（当前进度与剩余待办）→ §5（git 身份/合并上游）→ §7（构建镜像 + 部署流程，踩过大坑）→ 需要背景再看 §1-§3。
>
> **当前一句话状态（2026-07-20）**：定制分支已合并官方 `0.1.162`，普通用户「我的账号」完整自助、OpenAI/Codex/Grok 创建流程、临时账号 JSON 导入，以及管理员账号列表的“归属人”展示与筛选均已随国内生产 `0.1.162-xdl7` 上线。线上容器健康，两个公网入口均正常。

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
- 写（改/删）：非 admin 仅能碰自己的，用**原子** `UPDATE/DELETE ... WHERE id=? AND owner_user_id=?`，命中 0 行 → 404，天然满足"删不掉 admin/别人的"；admin 不受限。删除父账号若挂有 admin 的 spark shadow 子账号，应拒绝（不走会级联硬删 shadow 的 admin 删除路径）。
- 必须覆盖的旁路（否则越权）：OAuth 建号（state 要签名绑定 user id、一次性、短 TTL，回调不信前端传的 owner）、Codex 导入、data import/export、batch/bulk、refresh/test、stats、代理质量检测，以及所有关联 ID（account.group_ids / account.proxy_id / proxy.backup_proxy_id）。

## 3. 实现方式：就地改 + owner 收口（不做 /self 平行体系）

> **决策更正（2026-07）**：一度打算新增 `/self` 窄接口 + 独立前端，但账号导入弹窗 `frontend/src/components/account/CreateAccountModal.vue` 是 **6248 行的巨型组件**，15+ 处硬编码调 `adminAPI.accounts.*` 和各平台 OAuth 接口。要让普通用户获得**和管理员完全一样**的导入体验（多平台、OAuth 授权跳转、Codex PAT、session 导入…），复用这个组件比重写划算得多；而复用它就必须让它调的那些 **admin 账号接口对普通用户也可用**。因此改为"就地改"：**开放现有账号功能给普通用户 + 在 service 层按 owner 收口**，前端几乎原样复用。代价是改动落在账号核心文件上、合上游冲突更大——这是"功能完整、体验一致"换来的，已知情接受。`/self` 脚手架已回退。

后端：
- `backend/migrations/9000_xdl_resource_visibility.sql` — 加两列 + owner 索引。**fork 专属命名，上线后永不重命名/改内容**（迁移 runner 按完整文件名 + SHA256 记录）。已完成。
- `backend/ent/schema/{account,group,proxy}.go` — 各加 owner_user_id + is_public 字段。改后 `cd backend && go generate ./ent/...` 重生成（**只跑 ent**）。已完成。
- **路由放开**：把普通用户需要的那部分账号端点（create / OAuth 授权与 exchange / apply-oauth / createOpenAICodexPAT / import-codex-session / update / delete / test / refresh / list / get / stats）注册到 JWT 认证组，指向**现有** `h.Admin.Account.*` handler。危险的 admin 基础设施端点（crs 同步、上游计费探测设置、batch、models sync、reset-quota 等）**保持 admin-only**。
- **service 层 owner 收口**（安全核心）：账号 service 的 list/get/create/update/delete/bulk 按"调用者是否 admin + 其 userID"收口。非 admin：list 只见自己的+public；get/改/删只能碰自己的（原子 `WHERE ... AND owner_user_id=?`）；create 写 owner=自己。admin：不受限。actor 身份从认证中间件经 context 下传。
- 保留了上游一个 wire bind 修复（`securityaudit.PromptAdminService`→`*PromptService`），使 `go generate`(wire) 可重新生成。

前端：
- 复用现有 `AccountsView` + `CreateAccountModal` 等，**基本不改业务逻辑**；只需：账号页/菜单对普通用户放开（`requiresAdmin:false`），导入/编辑弹窗加"公开/私有"开关，非管理员隐藏纯管理项（如把号绑进共享分组，仍归 admin 审核）。

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
# 解冲突：ent 生成代码不手工解 → 只跑 ent 重生成；迁移文件保持不动
cd backend && GOTOOLCHAIN=auto go generate ./ent/... && go build ./... && go test ./internal/handler/self/...
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

## 7. 构建镜像 & 部署到服务器（务必读——踩过大坑）

> ⛔ **绝对不要在生产服务器上 `docker build`。** 2026-07 试过一次：国内 4 核 4G 生产机跑镜像构建，Go 编译 + 前端构建并行吃光内存，把线上容器和 SSH 全拖垮，生产中断十几分钟，最后只能腾讯云控制台**强制重启**才救回。构建是给 16G 的 CI 机器干的，不是生产机。

### 7.1 构建：只用 GitHub Actions（免费、16G、不碰你服务器）
镜像由 fork 自带的 `.github/workflows/release.yml`（GoReleaser + `.goreleaser.simple.yaml`）构建并推到 GHCR。触发方式（**tag 必须真实存在，否则 release job 报 "tag could not be found"**；tag push 本身有时不触发 workflow，用 workflow_dispatch 最稳）：
```bash
cd /Users/xudelong/mine/sub2api/sub2api
# 1) 确保 fork 仓库变量 SIMPLE_RELEASE=true（只出 x86_64 GHCR 镜像，一次即可）
gh variable set SIMPLE_RELEASE --body true --repo xiyuxu1/sub2api
# 2) 打并推 tag（GoReleaser 会去掉 v 前缀 → 镜像 tag 无 v）
git tag -f v0.1.161-xdl1 <commit> && git push -f origin v0.1.161-xdl1
# 3) 触发构建（约 5 分钟）
gh workflow run release.yml --repo xiyuxu1/sub2api --ref xdl/self-service -f tag=v0.1.161-xdl1 -f simple_release=true
gh run watch <run-id> --repo xiyuxu1/sub2api
# 产物：ghcr.io/xiyuxu1/sub2api:0.1.161-xdl1（tag 无 v 前缀！）
```
⚠️ **别把本机 pnpm 产物提交进去**：本机 pnpm 是 11，会重写 `frontend/pnpm-lock.yaml` 并生成一个坏的 `frontend/pnpm-workspace.yaml`（内容是占位符 `set this to true or false`），导致 CI 的 pnpm@9 `--frozen-lockfile` 报 `packages field missing`。lockfile 保持上游原样、`pnpm-workspace.yaml` 不该存在。

### 7.2 GHCR 包必须是 public
`ghcr.io/xiyuxu1/sub2api` 这个 container package 要在 GitHub 上设为 **public**（Packages → 该包 → Package settings → Change visibility → Public）。镜像里无任何密钥（`.env`/证书被 `.gitignore`+`.dockerignore` 双重排除），公开安全。否则服务器拉不了（token 需 `read:packages`，且不该把带 `repo` 权限的 token 放生产机）。

### 7.3 拉镜像：走南京大学 GHCR 镜像站（国内直连 ghcr.io 只有 ~40KB/s，龟速）
国内直连 `ghcr.io` 被限速到几十 KB/s（240MB 要一小时）；用南大镜像站 `ghcr.nju.edu.cn` 秒下。**且 `docker pull` 经 SSH 会莫名丢输出/被 SSH 断连带死——必须用 `systemd-run` 把拉取跑成独立系统服务，与 SSH 解耦：**
```bash
ssh ubuntu@115.159.205.56
V=0.1.161-xdl1
sudo systemd-run --unit=njupull --collect /usr/bin/docker pull ghcr.nju.edu.cn/xiyuxu1/sub2api:$V
# 轮询直到 inactive；确认镜像到位
sudo journalctl -u njupull -n 5 --no-pager; docker images ghcr.nju.edu.cn/xiyuxu1/sub2api --format '{{.Tag}} {{.Size}}'
# 改名成 compose 用的规范名
docker tag ghcr.nju.edu.cn/xiyuxu1/sub2api:$V ghcr.io/xiyuxu1/sub2api:$V
```

### 7.4 切镜像地址 + 部署（国内节点，带秒回滚）
生产 compose 的镜像行**默认是 `weishaw/sub2api:latest`（上游官方）**；改成你的 fork 镜像即完成"切镜像地址"：
```bash
cd ~/sub2api-deploy
cp docker-compose.yml docker-compose.yml.bak-$(date +%F)          # 备份，回滚用
docker exec sub2api-postgres pg_dump -U <PGUSER> -d <PGDB> | gzip > backups/predeploy-$(date +%F-%H%M).sql.gz  # 备份库
# 切镜像地址：weishaw/sub2api:latest → ghcr.io/xiyuxu1/sub2api:<V>
sed -i "s#image: weishaw/sub2api:latest#image: ghcr.io/xiyuxu1/sub2api:$V#" docker-compose.yml
docker compose up -d sub2api          # 重建容器；新容器启动时自动跑 ApplyMigrations（含 9000_xdl 迁移）
docker compose ps; curl -s localhost:8080/health; docker compose logs --tail=50 sub2api | grep -iE "migrat|9000|error"
```
**回滚**（若异常）：把 image 行改回 `weishaw/sub2api:latest`（或上一个 fork tag），`docker compose up -d sub2api`。旧镜像和 compose 备份都在本地。
> ⚠️ **版本别降级**：部署前对比 `docker inspect sub2api --format '{{index .Config.Labels "org.opencontainers.image.version"}}'`（生产当前版本）与你的镜像版本。fork 分支必须先合并到 ≥ 生产版本的上游，否则老代码撞新库有风险。

### 7.5 部署后验证 owner 收口（用普通用户 token）
- 普通用户 `GET /api/v1/admin/accounts` → 只返回自己的（不是 403、也不含别人的）。
- 普通用户按 id 访问/改/删别人的账号 → 404。
- 普通用户建号 → 自动归属自己、不进任何分组、`schedulable=false`（惰性，等 admin 审核进池）。
- admin 后台不受影响，能看到全部账号。

## 8. 当前进度 & 待办（新会话从这里接手）

> 更新：2026-07-19。仓库在本机 `/Users/xudelong/mine/sub2api/sub2api`，分支 `xdl/self-service`。

### 8.1 已完成并【已上线国内生产】
- 分支已合并到上游 **0.1.161** + 账号 owner 收口改动，无降级。
- **国内节点 `115.159.205.56` 已部署镜像 `ghcr.io/xiyuxu1/sub2api:0.1.161-xdl1`**（部署代码 = 合并提交，tag `v0.1.161-xdl1`）。迁移 `9000_xdl` 已应用，accounts/proxies/groups 三表 owner_user_id + is_public 列都在。健康正常。旧镜像 `weishaw/sub2api:latest` + compose/DB 备份都在，可回滚。
- **账号自助（就地改 + owner 收口）已生效**：普通用户登录后左侧有「我的账号」页，复用管理员的 CreateAccountModal 导入（Claude OAuth + 手动填 key 可用），只看得到/能改删自己的号；建号自动归属自己、不进分组、schedulable=false（惰性，等 admin 审核进池）；admin 后台不受影响、看全部。经三轮 Codex 安全评审修过越权（P0 惰性建号 / 白名单收窄 / 列表只见自己 / duplicate·check-mixed-channel·scheduler_score 等旁路）。
- **is_public「公开/私有」开关已上线**（2026-07-19，镜像 `0.1.161-xdl2`，提交 `2714d8258` / tag `v0.1.161-xdl2`）：详见 §8.2。国内节点已切到 xdl2，健康正常，三表 is_public/owner_user_id 列俱在，启动无错误。旧 xdl1 镜像 + compose/DB 备份都在，可回滚。

### 8.2 已完成【已上线国内生产 `0.1.161-xdl2`】：is_public「公开/私有」开关（B 方案）
> 需求：账号"可选对别人是否可见"。已实现：owner 在「我的账号」每行翻转公开/私有；公开的号别人可在「公开账号」tab 只读浏览（严格白名单摘要）。安全前提兑现：跨用户展示走**只含 id/name/platform/type 的白名单 DTO**，绝不碰 credentials/extra/notes/error。
>
> **已落地改动**（2026-07-19，已提交 `2714d8258`、已推送、已上线 xdl2）：
> - 后端：
>   1. `service.Account` 加 `IsPublic bool`；ent mapper `accountEntityToService` 回填；`updateLockedAccount` 里 `SetIsPublic`（值由 UpdateAccount 从 owner 化 GetByID 回填/覆盖）。
>   2. 非 admin 建号默认私有：`account_repo.go createAccountRecord` 非 admin 分支 `SetIsPublic(false)`（opt-in 公开；admin/系统号仍走 DB 默认 true）。
>   3. 更新链路串通：`UpdateAccountRequest.IsPublic`(handler) → `UpdateAccountInput.IsPublic`(service) → `account.IsPublic`。列表每行开关调 `PUT /admin/accounts/:id {is_public}`，走 owner 收口。
>   4. 白名单 DTO `dto.PublicAccount`(id/name/platform/type) + `PublicAccountFromService`；单测 `public_account_whitelist_test.go` 断言绝不泄露 credentials/extra/notes/error。
>   5. 跨用户浏览：`account_repo.go ListPublicAccounts`（`IsPublicEQ(true)` AND `Or(OwnerUserIDIsNil, OwnerUserIDNEQ(uid))` — 显式处理 SQL NULL 三值逻辑，含系统公开号）；service+interface `ListPublicAccounts`；handler `List` 非 admin + `?scope=public` → `listPublicAccounts`（白名单渲染，跳过所有运行态富化侧信道）。
> - 前端 `MyAccountsView.vue`：每行「公开/私有」pill（乐观翻转，失败回滚）；新增「公开账号」只读 tab（`list(..., {scope:'public'})`）。types + zh/en i18n 已加。
> - 顺手修了一个**既有**测试编译错误：`internal/server/routes/ops_ingress_reject_routes_test.go` 调 `RegisterAdminRoutes` 少传 `jwtAuth` 参数（早前 fork 给账号子树加 jwtAuth 门卫时漏改此测试；已 stash 验证与本次 is_public 改动无关）。补了 pass-through `jwtAuth` stub。
> - 验证：**全量 `go build ./...` + `go vet ./...` 干净（无任何编译错误）**；改动包 `go test`（service / handler/admin / handler/dto / server/routes）全过；前端 `vue-tsc --noEmit` 全过；lockfile/pnpm-workspace 未动。
> - 部署：已上线 `0.1.161-xdl2`（国内节点），后端 is_public 写入/浏览链路都在，健康正常。
>
> ⚠️ **上线后用户验收反馈（2026-07-19，代码已改、待部署）**：
> - 后端 owner 收口的 is_public 写入链路**保留**（每行公开/私有开关这个属性是对的）。
> - ✅ **已删前端「公开账号」浏览 tab**（`MyAccountsView.vue` 的 tab 栏 / public 表格 / `switchTab·reloadPublic·publicAccounts` / `accounts.ts` 的 `scope`）+ 相关 i18n（tabMine/tabPublic/publicTabDesc/publicEmpty）。**后端 `scope=public` / `ListPublicAccounts` / `dto.PublicAccount` 故意留着**当无调用死代码（删它要再动 AccountRepository 接口 + 6 个测试 mock，低额度下不值当；无前端调用、无副作用）。若 §8.3 选定"完整自助"后仍不需要跨用户浏览，可一并清掉。
> - ✅ **已修「我的账号」缺导航**：`MyAccountsView.vue` 现已套 `<AppLayout>`（对齐 Dashboard/Keys），侧边栏/顶栏回来；页面标题走路由 meta，删掉了原来重复的本地 `<h1>`。
> - ⏳ **仍待办**：功能太少（只有导入/编辑/测试/删除+公开开关），用户要**对自己账号的完整所有权**（换分组/绑代理等）——这是大活 + P0 安全冲突，见 §8.3。
> - 这两个 ✅ 是**代码改好但还没重新部署**：生产 `0.1.161-xdl2` 上仍是旧的（带多余 tab、无导航）。下次部署（或 Codex 做 §8.3 时一起）按 §7 出新镜像即可。

### 8.3 【交接 Codex 重点】「我的账号」升级为完整自助 + is_public 收尾

> ✅ **已完成并上线（2026-07-19，国内生产 `0.1.161-xdl4`）**：用户明确选择 **B（信任模型放宽）**。`/my-accounts` 直接复用 `AccountsView.vue` 的 self-service 模式，保留搜索、筛选、手动/自动刷新、今日次数、实时用量、编辑、测试、重新授权、刷新 Token、公开/私有、代理和分组选择；隐藏批量操作、调度开关、CRS、计费探测、配额重置等 admin-only 功能。后端新增脱敏的共享代理/分组选项接口，开放必要 OAuth/统计/配额路由并逐账号执行 owner 校验；混合渠道预检也先校验 owner 并使用真实平台，批量今日统计限制 200 个 ID。普通用户新建账号仍 `schedulable=false`，等待 admin 审核。验证：前端 175 文件/1220 测试、`vue-tsc`、生产构建、后端相关四包全量测试及 `go vet` 均通过；独立审查反馈已清零。部署提交 `4340e306e`、tag `v0.1.161-xdl4`、GitHub Actions run `29677384818`；生产容器 healthy，容器内 `/health` 与两个公网入口均通过。

> 用户明确的新方向（2026-07-19）：**保留独立的「我的账号」菜单**，但要把管理员「账号管理」页（`frontend/src/views/admin/AccountsView.vue`，2087 行）的**大部分功能迁过来**，让普通用户对**自己的**账号有完整所有权——包括**换分组、绑代理**、改并发/优先级、配额、编辑凭据等；**只去掉极少数极敏感的管理基础设施操作**。使用体验要对齐管理员页，不要做成现在这个"只能导入"的残页。

#### 🔴 最关键的安全冲突（Codex 动手前必须先解决，别无脑迁功能）
上个会话把"非 admin 建号**不进分组、不绑代理、不可调度**"作为 **P0 越权修复**写死了（见 §8.1、§3、`admin_account.go` CreateAccount / UpdateAccount 里的 `NonAdminOwner` clamp：`input.GroupIDs=nil; input.ProxyID=nil`）。原因：**分组=共享调度池**。普通用户若能把"指向自己可控上游/代理"的账号绑进某个共享分组，该账号就会被拿去服务**别人**的 API 流量 → 他就能**截获别人的 prompt/响应**。这正是本项目最核心的越权风险。
- 用户现在要的"换分组/绑代理"**与这条 P0 直接冲突**。这不是纯 UI 迁移，是**安全策略变更**，必须先定风险姿态：
  - 选项 A（保守）：普通用户只能在**自己拥有的私有资源**之间绑（自己的分组、自己 owner 的代理），**绝不能绑进含别人账号/会服务公共流量的共享分组**。需要给分组/代理也做 owner + 可见性收口（P2/P3 本来就规划了），并在 bind 时校验目标分组/代理的 owner=自己。
  - 选项 B（信任模型放宽）：既然"都是信任的朋友"，允许绑进共享分组——但要**明确知情**：等于允许成员的账号进入公共调度池、也就接受"成员理论上能被路由到别人流量"的风险。**这个只能由用户拍板**，Codex 不能替他决定。
- ⛔ **在用户明确选定 A 或 B 之前，不要动 CreateAccount/UpdateAccount 里的 `GroupIDs=nil / ProxyID=nil` clamp。** 直接放开 = 重新捅穿 P0。

#### 迁移清单（在安全姿态定了之后再做）
- **前端**：优先方案是让「我的账号」直接**复用/内嵌 `AccountsView.vue` 的表格与操作**（而不是现在这个手写的精简 `MyAccountsView.vue`），按角色隐藏 admin-only 控件；或把 `AccountsView` 抽成可配置组件，普通用户模式下关掉危险控件。**务必逐一过 2087 行里的每个操作**，分类：普通用户可用 / admin-only 隐藏。
  - 建议保留给普通用户：导入(CreateAccountModal)、编辑(EditAccountModal)、测试、删除自己的、换分组*、绑代理*、改并发/优先级/配额、公开/私有开关（is_public）。（*绑分组/代理受上面安全姿态约束。）
  - 建议 admin-only 隐藏：批量改/批量删(Batch)、重置配额(reset-quota)、调度开关(schedulable)、CRS 同步、上游计费探测设置、models sync 等基础设施操作。
  - **先修 `<AppLayout>`**：无论走哪个方案，「我的账号」页必须套 `<AppLayout>`（对齐 `DashboardView.vue`/`KeysView.vue`），否则没导航。
  - 删掉本次多做的「公开账号」浏览 tab（`MyAccountsView.vue` 里 `tab==='public'` 那块 + `switchTab/reloadPublic/publicAccounts` + `accounts.ts` 的 `scope:'public'`）。
- **后端**：每放开一个操作，都要确认对应端点在 service/repo 层已按 owner 收口（非本人 → 404/403），并覆盖旁路（关联 ID、批量、导入、OAuth）。绑分组/代理放开必须加"目标资源 owner 校验"（选项 A）。参照 §2/§3 的授权规则与"必须覆盖的旁路"清单。
- **验证**：每次改完按 §7 全流程（CI 构建 → 南大站拉 → 切镜像），并用普通用户 + admin 两种 token 跑越权测试。

### 8.4 其余进度与待办
- **普通用户 OAuth/PAT 与临时账号导入已上线**（2026-07-19，国内生产 `0.1.161-xdl5`）：OpenAI OAuth、Codex PAT、Grok OAuth/SSO 创建路由已移入普通认证组；「我的账号」增加兼容原 JSON 格式的临时账号导入。自助导入会丢弃文件内代理及账号代理绑定，只创建当前 owner 的不可调度账号，不能创建或修改共享代理；失败重试复用幂等键。创建/编辑弹窗在 self-service 模式不再请求管理员设置和 TLS 配置。部署提交 `b590dcff3`、tag `v0.1.161-xdl5`、GitHub Actions run `29678696322`；生产切换前已备份数据库和 Compose（时间戳 `2026-07-19-155239`），容器 healthy，容器内 `/health`、`xiaoxu.xyz` 和 `xiyu.site:8443` 均通过。
- **管理员识别账号归属人已上线国内生产 `0.1.161-xdl6`**（2026-07-19）：账号 service/DTO 已透传 `owner_user_id`；管理员账号表新增“归属人”列，普通用户账号显示用户名与邮箱，已删除用户有明确标记，历史无 owner 的账号显示“系统/管理员”；self-service 页面不展示该列，也不加载用户目录。管理员筛选栏新增归属人下拉，选项顺序固定为“全部归属人”“系统/管理员账号”（即 `owner_user_id IS NULL`）和全部普通用户（含已删除用户，用户名 + 邮箱）。Spark 影子账号继承母账号的 owner 与公开属性；生产库部署前确认历史 Spark 影子账号为 0，无需回填。`owner` 条件在 repository 中执行，已同步覆盖分页总数、自动刷新 ETag 请求、调度分候选池、按筛选批量编辑和按筛选导出，避免列表范围与写操作/导出范围不一致。验证：全仓 Go 编译、`go vet ./...`、相关 handler/DTO/service/repository 测试、前端 176 文件/1225 测试、`vue-tsc` 和生产构建均通过；完整 service 包仅因沙箱禁止 `httptest` 绑定 `[::1]:0` 未能全跑，相关改动用例已通过。部署提交 `22dbb9020`、tag `v0.1.161-xdl6`、GitHub Actions run `29682597711`；生产切换前已备份数据库和 Compose（时间戳 `2026-07-19-180635`），容器 revision `22dbb9020`、healthy、重启次数 0，本机 `/health`、`xiaoxu.xyz` 和国内节点 `xiyu.site:8443` 均返回 200，切换后的实际 `/responses` 流量亦成功返回 200。
- **P2 代理**：同款 owner 收口就地改（代理的 owner_user_id/is_public 列已就绪）。注意代理 admin DTO 会返回密码明文，普通用户视图必须脱敏。
- **P3 分组**：同款。用户要求分组也做自助+可见性（本项目未启用分组计费/RPM/fallback/模型路由策略，故不锁字段；若将来启用要锁回 admin）。
- **P4 海外节点** `70.39.194.149`（/opt/sub2api-deploy，直连无 mihomo）：账号这套稳定后同样切镜像部署。
- 部署后活体验收：本会话没跑通（admin 密码已改、不在 .env；无普通用户密码），靠用户浏览器验收。

### 8.5 关键提醒（避免重复踩坑）
- ⛔ 永远别在生产机 `docker build`（会 OOM 打爆，2026-07 已停机一次）。构建只用 GitHub Actions（§7）。
- 拉镜像走南大镜像站 `ghcr.nju.edu.cn` + `systemd-run`（§7.3），直连 ghcr.io 龟速、且 docker pull 经 SSH 会被断连带死。
- 别把本机 pnpm 产物（改动的 pnpm-lock.yaml / 生成的 pnpm-workspace.yaml）提交进去（§7.1）。
- git 走 origin HTTPS（gh 活跃账号 xiyuxu1）；合并上游按 §5。

### 8.6 官方 0.1.162 合并与国内生产部署

- **已完成并上线（2026-07-20，国内生产 `0.1.162-xdl7`）**：`xdl/self-service` 合并官方 `0.1.162`，合并提交 `c9c6d3ccb`、tag `v0.1.162-xdl7`、GitHub Actions run `29753250700`。
- 合并仅在 `frontend/src/views/admin/AccountsView.vue` 出现冲突；处理结果保留 self-service 对管理工具和代理 fallback 回退操作的隐藏，同时采用上游新版深色样式。上游新增 repository 测试夹具已补齐 fork 的 `owner_user_id` / `is_public` 两列。
- 验证：全仓 Go 编译与 `go vet ./...` 通过；handler/admin、DTO、routes、owner 相关 service/repository 测试通过；全量 repository/service 测试仅受沙箱禁止 `miniredis` / `httptest` 监听本地端口影响；前端 17 个测试文件共 220 个用例、`vue-tsc` 和生产构建均通过。
- 部署前备份时间戳 `2026-07-20-230555`；数据库 dump 约 9.2 MB，Compose 备份均在 `/home/ubuntu/sub2api-deploy/backups/`。生产镜像 revision `c9c6d3ccb`、health=`healthy`、重启次数 0；容器内 `/health`、`xiaoxu.xyz/health`、`xiyu.site:8443/health` 均通过（两个公网入口 HTTP 200）。
