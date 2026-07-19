package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 跨用户"公开账号"浏览的白名单 DTO 是安全边界（fork §8.2）：即便源账号带 credentials /
// extra / notes / error_message，序列化后也绝不能出现这些键。这个测试在字段名冒进时立刻失败。
func TestPublicAccountFromService_OnlyWhitelistedFields(t *testing.T) {
	src := &service.Account{
		ID:       7,
		Name:     "friend-oauth",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":     "SECRET-TOKEN",
			"header_overrides": map[string]any{"x-auth-token": "SECRET-HEADER"},
		},
		Extra:        map[string]any{"quota_used": 123, "api_key": "SECRET-EXTRA"},
		Notes:        ptr("private note"),
		ErrorMessage: "upstream 401 with SECRET in body",
		IsPublic:     true,
	}

	raw, err := json.Marshal(PublicAccountFromService(src))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	allowed := map[string]bool{"id": true, "name": true, "platform": true, "type": true}
	for k := range got {
		if !allowed[k] {
			t.Errorf("public account DTO leaked field %q (payload=%s)", k, raw)
		}
	}
	// 任何密钥子串出现在输出里都是泄露。
	for _, secret := range []string{"SECRET-TOKEN", "SECRET-HEADER", "SECRET-EXTRA", "private note", "401"} {
		if containsSub(string(raw), secret) {
			t.Errorf("public account DTO leaked secret %q (payload=%s)", secret, raw)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
