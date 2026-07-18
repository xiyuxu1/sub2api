-- fork 自定义迁移（见 fork-docs/README.md）：给账号/代理/分组加"归属人 + 管理可见性"。
-- 加法变更，向后兼容（旧镜像忽略新列照常运行）。fork 专属命名，上线后永不改名/改内容。
--   owner_user_id: 资源归属人。NULL = 系统/admin 历史导入；新建由服务端写入。
--   is_public:     管理可见性。true = 其他普通用户可见（只读摘要）。默认 true（自己人透明）。
-- 只给 owner_user_id 建索引；is_public 默认几乎全 true、选择性差，不建索引。

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT true;

CREATE INDEX IF NOT EXISTS idx_accounts_owner_user_id ON accounts (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_proxies_owner_user_id ON proxies (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_groups_owner_user_id ON groups (owner_user_id);
