package securityaudit

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewPostgreSQLRepository,
	wire.Bind(new(JobRepository), new(*PostgreSQLRepository)),
	wire.Bind(new(EventRepository), new(*PostgreSQLRepository)),
	NewRedisPayloadStore,
	wire.Bind(new(PayloadStore), new(*RedisPayloadStore)),
	NewOpenAICompatibleScanner,
	wire.Bind(new(PromptScanner), new(*OpenAICompatibleScanner)),
	NewAtomicMetrics,
	wire.Bind(new(Metrics), new(*AtomicMetrics)),
	NewConfigManager,
	wire.Bind(new(ConfigStore), new(*ConfigManager)),
	NewPromptService,
	wire.Bind(new(PromptEngine), new(*PromptService)),
	// fork: 补齐上游漏掉的接口绑定，使 `go generate`(wire) 能重新生成。
	// PromptAdminHandler 需要 PromptAdminService，由 *PromptService 实现。
	wire.Bind(new(PromptAdminService), new(*PromptService)),
	NewLegacyModerationAdapter,
	NewCoordinator,
	NewPromptAdminHandler,
)
