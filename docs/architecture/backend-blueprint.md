# 后端文件与代码蓝图

返回 [架构入口](../../ARCHITECTURE.md)。本文明确一个文件里面应当放什么，并给出关键代码与 SQL。个人资料与 AI 配置扩展见 [专项设计](profile-and-ai-settings.md)，凭据生命周期以该文档为准。

代码块是实施模板：`model.go`、`ports.go`、`submit.go`、`tx.go` 展示完整文件形状；涉及 sqlc 生成类型、业务 SQL 和进程 wiring 的部分明确写为流程或片段。它们不构成已经实现、通过集成验证的后端。Go module 当前为 `applyflow/backend`；发布到仓库时统一处理 module/import 路径。

## 适用范围更新

核心产品已改为 [Application Studio](application-studio.md)：文件/来源/简历/文档/task 模块以及新生成事务见该文。本文第 2–12 节的 analysis 专用代码与 SQL 保留作可靠后台任务的教学示例和后续技能分析参考，**不按其表名再建立第二套任务运行时**。正式实现将 status/lease/fence/outbox 统一落在 job_tasks/task_outbox，技能结果用 task_id 关联；队列 payload 改为 task_id，SSE 为 task_state。文档结果还需 workspace/document/task 锁序、候选 revision 与 CAS，不能只替换示例 SQL 的表名。

第 1、13 节的文件职责与生命周期规则继续有效。新增应用的任务种类分别校验 schema，不把 JD 提取、简历生成与导出塞进一个 Provider.Analyse 方法。当前 Go module 已有基础迁移程序，以下示例仍未成为业务代码。

## 1. 单个 Go 文件的组织规则

按以下顺序组织：package → imports → constants/errors → types/interfaces → constructor → exported methods → private helpers。无需为了顺序把有关联的短类型强行拆开。

| 文件 | 应包含 | 不应包含 |
|---|---|---|
| `model.go` | 业务状态、输入输出值、纯函数 | JSON 绑定、SQL、环境读取 |
| `ports.go` | 用例消费的最小接口、语义契约 | 一百个 CRUD 方法的万能接口 |
| `submit.go` | Submit 用例、业务校验 | Gin Context、Redis Client、显式 SQL |
| `execute.go` | 执行状态编排、取消、错误分类 | HTTP status、前端展示文本 |
| `handler.go` | decode → invoke → map response | SQL、业务事务、goroutine 启动任务 |
| `dto.go` | HTTP JSON 字段与映射 | 原样暴露数据库模型 |
| `*_store.go` | SQL 调用、原子事务、SQL 错误翻译 | 调模型、发邮件、等待队列 |
| `client.go` | 外部协议、响应校验、provider 错误 | application 状态更新 |
| `bootstrap/*.go` | 构造对象、配置注入、关闭注册 | 业务判断 |

函数围绕一个用例或一个状态转换。不要以固定行数为硬规则；当一个文件同时承担 HTTP、SQL 和业务决策时，即使只有 150 行也应该拆分。

context 是方法第一个参数，不存入长期存活的 service。所有跨进程操作接受 context 并有 deadline。错误使用 `%w` 保留错误链；仅在请求/任务边界记录一次完整错误。

## 2. `analysis/model.go`：明确的数据契约

```go
package analysis

import (
	"encoding/json"
	"errors"
	"time"
)

type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	RetryWait Status = "retry_wait"
	Completed Status = "completed"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
)

func (s Status) Terminal() bool {
	return s == Completed || s == Failed || s == Cancelled
}

var (
	ErrInvalid         = errors.New("invalid input")
	ErrNotFound        = errors.New("resource not found")
	ErrKeyConflict     = errors.New("idempotency key reused with different input")
	ErrVersionConflict = errors.New("source version changed")
	ErrAlreadyActive   = errors.New("analysis already active")
	ErrQuotaExceeded   = errors.New("analysis quota exceeded")
	ErrNotClaimable    = errors.New("task not claimable")
	ErrLeaseLost       = errors.New("execution lease lost")
	ErrCancelRequested = errors.New("cancellation requested")
)

type SubmitInput struct {
	OwnerID                 string
	ApplicationID           string
	IdempotencyKey          string
	ExpectedJDVersion       int64
	ExpectedProfileVersion  int64
	ExpectedAIConfigVersion int64
}

type AcceptCommand struct {
	Input       SubmitInput
	RequestHash [32]byte
}

type Snapshot struct {
	ID              string
	ApplicationID   string
	Status          Status
	Stage           string
	StateVersion    int64
	JDVersion       int64
	ProfileVersion  int64
	CancelRequested bool
	ErrorCode       string // 仅允许公开的稳定错误码
	UpdatedAt       time.Time
}

type Accepted struct {
	Snapshot Snapshot
	Replay   bool
}

type Claim struct {
	ID            string
	OwnerID       string
	Fence         int64
	Attempt       int
	JD            string
	Profile       json.RawMessage
	PromptVersion string
	SchemaVersion string
	ProviderModel string
	AIRevisionID  string
	CredentialID  string
	LeaseUntil    time.Time
}

// Extraction 是 provider 已做结构校验的输出，不包含用户能力判断。
type Extraction struct {
	SchemaVersion string
	JSON          json.RawMessage
	Skills        []ExtractedSkill
}

type ExtractedSkill struct {
	Name        string
	Requirement string
	Evidence    string
}

type ValidatedResult struct {
	SchemaVersion string
	JSON          json.RawMessage
	Skills        []MatchedSkill
}

type MatchedSkill struct {
	SkillID     string // 未映射的名称保留在 JSON 的 unresolved 字段
	Requirement string // required / preferred
	Evidence    string
	Assessment  string // self_assessed_match / review / new / not_assessed
}

type Failure struct {
	PublicCode string
	Retryable  bool
	RetryAfter time.Duration
}
```

`Snapshot` 不包含 JD、profile 或 provider 原始响应。详情结果走单独授权接口投影。`ValidatedResult` 只能在 JSON Schema、长度、枚举、证据检查成功后构建；Go 的导出类型本身不提供这一安全保证，`Complete` 仍需要最低限度的 schema/version 防御校验。

## 3. `analysis/ports.go`：接口描述原子语义

```go
package analysis

import (
	"context"
	"time"
)

// Accept 必须在一个事务中完成幂等、权限、版本、配额、任务及 outbox。
type AcceptStore interface {
	Accept(ctx context.Context, cmd AcceptCommand) (Accepted, error)
}

// 用户请求只能通过 OwnedSnapshot 读取；跨租户资源与不存在统一返回 ErrNotFound。
type SnapshotReader interface {
	OwnedSnapshot(ctx context.Context, ownerID, analysisID string) (Snapshot, error)
}

type ExecutionStore interface {
	Claim(ctx context.Context, analysisID string, lease time.Duration) (Claim, error)
	Renew(ctx context.Context, claim Claim, lease time.Duration) error
	SetStage(ctx context.Context, claim Claim, stage string) error
	Complete(ctx context.Context, claim Claim, result ValidatedResult) (Snapshot, error)
	// 仅 fence/lease 仍有效时提交失败/重试；数据库计算 next_attempt_at。
	RecordFailure(ctx context.Context, claim Claim, failure Failure) (Snapshot, error)
	FinishCancellation(ctx context.Context, claim Claim) (Snapshot, error)
}

// Resolve 返回 canonical skill ID；未识别的名称不自动创建全局技能。
type SkillCatalog interface {
	Resolve(ctx context.Context, names []string) (map[string]string, error)
}

type Provider interface {
	Analyse(ctx context.Context, claim Claim) (Extraction, error)
}

// 发布失败不回滚已经提交的状态；消费者使用数据库快照补偿。
type ProgressNotifier interface {
	Notify(ctx context.Context, analysisID string, version int64) error
}
```

provider adapter 从 Claim 只提取 JD、模型和 prompt/schema 版本发送到外部服务；profile 快照留在本地用于确定性匹配，不作为提取请求发送。证据必须能对应到 JD 原文，无法映射的技能保留 unresolved，不编造 canonical ID。

不要把整个 Store 接口传给只需要读取状态的 SSE handler。上述 execution 方法都必须说明受影响行数为 0 的含义；`Complete` 不能以没有 error 的 `Exec` 当作成功。

## 4. `analysis/submit.go`：用例代码

```go
package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

type Submitter struct {
	store AcceptStore
}

func NewSubmitter(store AcceptStore) *Submitter {
	return &Submitter{store: store}
}

func (s *Submitter) Submit(ctx context.Context, in SubmitInput) (Accepted, error) {
	if in.OwnerID == "" || in.ApplicationID == "" ||
		in.ExpectedJDVersion < 1 || in.ExpectedProfileVersion < 1 ||
		in.ExpectedAIConfigVersion < 1 ||
		len(in.IdempotencyKey) < 16 || len(in.IdempotencyKey) > 128 ||
		strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
		return Accepted{}, ErrInvalid
	}

	// 固定字段与顺序。不得加入当前时间、随机 ID、当前数据库版本。
	// 客户端指定的前置版本使同一次重试始终有相同语义。
	canonical, err := json.Marshal(struct {
		Operation       string `json:"operation"`
		ApplicationID   string `json:"application_id"`
		JDVersion       int64  `json:"jd_version"`
		ProfileVersion  int64  `json:"profile_version"`
		AIConfigVersion int64  `json:"ai_config_version"`
	}{
		Operation:       "analysis.submit.v1",
		ApplicationID:   in.ApplicationID,
		JDVersion:       in.ExpectedJDVersion,
		ProfileVersion:  in.ExpectedProfileVersion,
		AIConfigVersion: in.ExpectedAIConfigVersion,
	})
	if err != nil {
		return Accepted{}, fmt.Errorf("encode submission fingerprint: %w", err)
	}

	accepted, err := s.store.Accept(ctx, AcceptCommand{
		Input: in, RequestHash: sha256.Sum256(canonical),
	})
	if err != nil {
		return Accepted{}, fmt.Errorf("accept analysis: %w", err)
	}
	return accepted, nil
}
```

handler 额外检查 UUID 格式、JSON 未知字段、body 大小及 key 的 ASCII 格式。业务用例仍保留必需字段检查。原始 key 可保存为 HMAC/摘要以减少日志误用，任何情况下都不将它输出到常规 access log。

## 5. `transport/http/analyses/handler.go`：只做协议转换

结构与方法签名：

```go
// package analyses；片段，省略具体 imports、DTO 和 response helpers。
type SubmitUseCase interface {
    Submit(context.Context, analysis.SubmitInput) (analysis.Accepted, error)
}

type Handler struct {
    submit SubmitUseCase
    read   analysis.SnapshotReader
}

func NewHandler(submit SubmitUseCase, read analysis.SnapshotReader) *Handler {
    return &Handler{submit: submit, read: read}
}
```

`Submit(c *gin.Context)` 按顺序执行：

1. 从认证 middleware 写入的 typed identity 读取 owner，绝不从 JSON 的 `user_id` 读取。
2. 验证 application UUID、`Idempotency-Key`、Content-Type 和 body 上限。
3. 严格解码 `{expected_jd_version, expected_profile_version, expected_ai_config_version}`，拒绝未知字段和尾随 JSON。
4. 从 `c.Request.Context()` 派生短 deadline，调用 Submit。
5. 通过统一错误 mapper 输出，不直接向用户返回 `err.Error()`。
6. 新接受的任务返回 `202`；同 key 重放活动任务也为 `202`，重放终态任务为 `200`。设置 `Location: /api/analyses/{id}`。

不要 `go service.Submit(...)`。请求取消可以中止尚未提交的事务；事务已经提交的任务独立继续运行。若提交结果因网络中断不确定，客户端用相同 key 重试确认。

响应 DTO 单独定义并带 JSON tags：`analysis_id`、`application_id`、`status`、`stage`、`state_version`、`replayed`。时间全部显式格式化/序列化为 RFC3339，不让 provider DTO 泄漏到公共协议。

## 6. `postgres/tx.go`：事务辅助

sqlc 可将查询对象绑定到事务；具体生成方式按项目配置确定。[sqlc 事务说明](https://docs.sqlc.dev/en/latest/howto/transactions.html)

```go
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		// 请求 context 可能已经取消；清理使用独立且有界的 context。
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup) // 已 commit/rollback 时 ErrTxClosed 可忽略
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
```

这是基础 helper，不能自动重跑任意 callback。需要重试 serialization/deadlock 时，仅重试无外部副作用的完整短事务，设次数上限并尊重 deadline。不要重试“结果未知”的 commit 并假定前一次没有成功；提交用例通过幂等记录确认。

## 7. `postgres/analysis_accept.go`：接受事务的精确顺序

该文件实现 `AcceptStore`，持有 pool、ID 生成器和固定的 AnalysisPolicy；通过编译期断言确认接口实现。ID、provider/prompt/schema 版本由服务端产生。

```text
BEGIN
  1. SELECT user FOR UPDATE                 -- 序列化该用户的配额和 profile 变更
     SELECT ai_settings WHERE user_id=? FOR UPDATE -- 配置模式与版本
  2. SELECT application WHERE owner_id=? FOR UPDATE
       不存在或 archived/deleting → NotFound/不可分析
  3. 查 (owner_id, operation, idempotency_key)
       已存在且 hash 不同 → KeyConflict
       已存在且 hash 相同 → 返回原 analysis 的当前状态，COMMIT
       注意：先重放，再检查来源版本是否已经变化
  4. 检查 expected_jd_version、expected_profile_version、expected_ai_config_version
       检查已启用的 AI 来源，锁定 revision/credential 引用（不复制密钥）
       不符 → VersionConflict，不创建新任务
  5. 检查 JD 非空、字节长度、用户活动任务数、当日已预留用量
       额度和来源快照在同一事务中确定
  6. INSERT analyses，复制 JD、profile、来源版本、provider/prompt/schema 版本
  7. INSERT analysis_outbox，payload 只有 analysis ID 与消息 schema version
  8. INSERT idempotency_requests，保存 request_hash → analysis_id
  9. 记录用量预留；生成 initial snapshot
COMMIT
```

`users` 行锁的代价是同一用户的提交串行化，个人求职平台可以接受；不同用户仍可并发。修改 profile 和提交使用相同用户锁，保证版本与 snapshot 对应。部分唯一索引负责跨请求最后一道并发保护。

首次提交 key 的有效保留期建议至少 24 小时且覆盖任务最长寿命与客户端重试窗口。清理后再次使用该 key 被视为新请求，API 文档明确这一边界。

初期硬 admission 限制采用“每用户最多 N 个活动任务 + 每日保守预留 token 预算”；外部返回实际用量后以唯一 `(analysis_id, attempt)` 账目结算。不确定是否计费的崩溃尝试保留预留金额直到核对，不能提前释放而突破预算。全局积压达到告警阈值时可暂停新分析，已有 outbox 不丢弃。

SQL 错误按 constraint name 映射：active 唯一冲突 → `ErrAlreadyActive`；版本前置条件失败 → `ErrVersionConflict`。不能把所有 unique violation 都映射为同一种业务错误。

## 8. migrations 与 queries：关键 SQL

下面是迁移/查询片段，**不是完整迁移文件**。字段全集以 model 和产品数据模型为输入；完整 migration 还须包含 users/applications 外键目标、检查约束与所有查询使用的字段。

```sql
-- applications 必须有可被复合外键引用的唯一约束。
ALTER TABLE applications ADD CONSTRAINT applications_id_owner_unique
    UNIQUE (id, owner_id);

ALTER TABLE analyses ADD CONSTRAINT analysis_application_owner_fk
    FOREIGN KEY (application_id, owner_id)
    REFERENCES applications (id, owner_id);

CREATE UNIQUE INDEX analyses_one_active_per_application
    ON analyses (application_id)
    WHERE status IN ('queued', 'running', 'retry_wait');

CREATE INDEX analyses_recovery_due
    ON analyses (next_delivery_at, id)
    WHERE status = 'queued';
CREATE INDEX analyses_retry_due
    ON analyses (next_attempt_at, id)
    WHERE status = 'retry_wait';
CREATE INDEX analyses_expired_lease
    ON analyses (lease_until, id)
    WHERE status = 'running';
CREATE INDEX applications_owner_updated
    ON applications (owner_id, updated_at DESC, id DESC);

CREATE UNIQUE INDEX idempotency_owner_operation_key
    ON idempotency_requests (owner_id, operation, key_hash);
CREATE UNIQUE INDEX outbox_analysis_generation
    ON analysis_outbox (analysis_id, delivery_generation);
```

补充数据库约束：status/stage 合法枚举，version/attempt 非负，running 必须有 lease，terminal 必须有 finished_at，completed 必须有合法 schema version 与结果。Outbox 有 owner-free内部 ID、analysis ID、generation、claim token、claim expiry、published_at、next_dispatch_at。所有账户敏感关联使用复合 owner 外键或在同一事务验证归属。

### `queries/analysis.sql`：原子认领

```sql
-- name: ClaimAnalysis :one
UPDATE analyses
SET status = 'running',
    stage = 'analysing',
    attempt_count = attempt_count + 1,
    fencing_token = fencing_token + 1,
    state_version = state_version + 1,
    lease_until = clock_timestamp() + sqlc.arg(lease_seconds)::int * interval '1 second',
    updated_at = clock_timestamp()
WHERE id = sqlc.arg(id)
  AND status = 'queued'
  AND cancel_requested = false
  AND attempt_count < max_attempts
RETURNING *;
```

0 行意味着不能执行，可能是重复投递、已取消或已被其他 worker 认领；正常 ACK 并记录分类指标，不调用 provider。过期 running 任务必须先由 scanner 转移状态，不能直接被普通消费者抢占。

### 续租

```sql
-- name: RenewAnalysisLease :execrows
UPDATE analyses
SET lease_until = clock_timestamp() + sqlc.arg(lease_seconds)::int * interval '1 second'
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND fencing_token = sqlc.arg(fence)
  AND cancel_requested = false
  AND lease_until > clock_timestamp();
```

0 行触发停止 provider 请求。另行读取受保护的任务状态区分 cancellation/lease lost；失败时不能假定自己仍然拥有任务。租约时间以数据库时间为准，避免多机时钟差异。

### 完成结果

```sql
-- name: CompleteAnalysis :one
UPDATE analyses
SET status = 'completed', stage = 'completed',
    result_json = sqlc.arg(result_json),
    state_version = state_version + 1,
    finished_at = clock_timestamp(), updated_at = clock_timestamp(),
    lease_until = NULL
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND fencing_token = sqlc.arg(fence)
  AND cancel_requested = false
  AND lease_until > clock_timestamp()
RETURNING *;
```

`Complete` 事务先执行这条 conditional update，0 行即终止。成功后写入 `analysis_skills`、用量结算记录，再 commit。不要先在事务外写技能，再尝试标记完成。数据库错误回滚两者。

读取“当前结果”使用 `analysis.jd_version = application.jd_version AND status='completed'`，按完成时间与 ID 排序。旧任务完成不更新 application 的 `current_analysis_id`，避免反向锁和覆盖。

### 状态编辑

Application 普通编辑使用 `WHERE owner_id=$owner AND id=$id AND version=$expected`。已落地的 migration 00003 通过数据库触发器提升 version/jd_version 并原子写入状态历史；repository 不重复提升版本或写历史。API 使用 `If-Match` 或明确 `expected_version`，首版统一采用后者。陈旧编辑返回 `409`，前端先刷新再由用户决定。

## 9. `background/dispatcher.go`：outbox 投递协议

单轮执行：

```text
短事务 A：挑选到期且未发布/claim 过期的 outbox
          FOR UPDATE SKIP LOCKED LIMIT batch
          设置 claim_token、claim_until，返回批次，COMMIT
事务外：  Enqueue(payload={v:1, analysis_id})
短事务 B：成功后按 id + claim_token 标记 published_at
          失败按 claim_token 更新 next_dispatch_at（backoff + jitter）
```

批次内发送设置小并发和 deadline，claim 时长覆盖最坏批次发送时间或支持续租。过期 dispatcher 不能更新新 dispatcher 的认领记录。发送成功但标记失败会重复投递，这是允许的。

`SKIP LOCKED` 适合这里的队列式选取，不用于业务列表的一致性查询。[PostgreSQL SELECT 锁说明](https://www.postgresql.org/docs/current/sql-select.html)

不把 analysis ID 固定作为永不变化的 Asynq task ID，否则后续重新投递可能被残留任务记录挡住。可以使用 outbox event ID 标识一次投递；业务去重只依赖 analysis 的原子 claim。消息里不放快照、token 或外部 API key。

## 10. `analysis/execute.go`：任务执行生命周期

执行流程如下，实际实现拆为小方法，避免一个超长函数：

```text
Validate message version/ID
  → provider shared rate-limit admission（有界等待，不持 DB 锁）
  → Claim analysis with DB lease
  → create task context from worker lifecycle, with execution timeout
  → start one lease-renewal loop
  → resolve owned pinned AI revision; reject revoked credentials
  → inject ephemeral decrypted credential into provider adapter
  → call Provider.Analyse(taskCtx, immutable claim)
  → resolve canonical skills and run deterministic matching in analysis/result.go
  → validate stored result schema, yielding ValidatedResult
  → stop + join renewal loop (保证没有后台 goroutine 泄漏)
  → classify provider result/error and latest cancellation/lease outcome
  → use fresh bounded persistence context for the final DB operation
  → Complete / FinishCancellation / RecordFailure
  → notify state version after commit (best effort)
  → ACK queue delivery
```

final persistence context 独立于已经超时的 provider context，但仍有很短 deadline；它不能绕过 lease/fence 条件。结束续租到提交之间保留足够租约余量，提交时数据库再次校验。provider 返回成功也不能跳过 cancellation 检查。

示例运行参数仅作起始配置：provider 60 秒、task 90 秒、lease 45 秒、每 10 秒续租、persist 3 秒、max attempts 3。启动校验续租周期显著小于 lease，task deadline 覆盖 provider + decode + commit；依据真实 provider 延迟调优。

**重试责任只有一处：**

- Asynq 投递明确设置 `MaxRetry(0)`；错误归档保留用于运维观察。[Asynq MaxRetry 文档](https://pkg.go.dev/github.com/hibiken/asynq#MaxRetry)
- 外部调用的暂时错误 → DB 写 `retry_wait` 与 next_attempt_at；写成功后 consumer 返回 nil。
- DB 本身不可用、无法记录结果 → consumer 返回 error；由 DB scanner 在 lease 到期后恢复业务任务。
- 429 尊重有界 Retry-After；5xx/网络错误有限重试；格式/权限/配置错误按类型失败并告警，不能无限消耗预算。
- HTTP SDK 内建重试关闭，或统一纳入一次 attempt 的总时间/费用预算；默认关闭，避免隐含倍增。
- 业务尝试次数包含已 claim 后崩溃的尝试；超出上限即 failed。

限流等待在 claim 前进行。如果暂时无 provider 配额，将 queued 任务的 next_delivery_at 延后后 ACK，由 scanner 重投；若该更新失败返回 error。队列 transport 丢失不会丢掉 DB 中的 queued 任务。每个副本的 concurrency 相加才是全局并发；请求频率限制和并发限制分别配置。

### 取消的线性化点

取消和完成都通过数据库行状态竞争：

- queued/retry_wait：取消事务直接写 cancelled。
- running：置 cancel_requested=true、state_version++，续租循环随后停止外部请求并提交 cancelled。
- 取消先提交：Complete 的 `cancel_requested=false` 条件失败。
- 完成先提交：取消返回 completed，不谎报 cancelled。
- Worker 已失联：scanner 在租约到期后发现 cancellation flag，转 cancelled，不重试。

取消完成后外部平台仍可能计费；UI 表示本系统不会接受该结果，不承诺外部平台撤销了计算。

## 11. `background/recovery.go`：恢复所有被遗留的工作

使用小批次、`SKIP LOCKED` 和 conditional update，多副本 scanner 可以并发运行。每次扫描有时间与行数上限；单个坏记录不能阻塞整个批次。

| 数据状态 | 恢复动作 |
|---|---|
| 未发布且 claim 过期的 outbox | dispatcher 重新认领 |
| running + lease 过期 + cancel_requested | cancelled，提升 state_version |
| running + lease 过期 + attempts 已达上限 | failed，公开错误码 execution_exhausted |
| running + lease 过期，仍可重试 | retry_wait，记录 next_attempt_at，废止旧 fence |
| retry_wait 到期 | queued，generation++，原子创建新 outbox |
| queued 长期未被认领 | generation++，原子创建新的 delivery outbox，推进 next_delivery_at |

queued 超时重投可能在积压时制造重复消息，因此 next_delivery_at 采用有界退避，控制每轮重投数量，同时对 oldest queued age 告警。不得按秒无限生成 outbox。累计排队/执行超过任务总寿命时转 failed；示例总寿命 30 分钟，按实际产品需求调整。

故障恢复与 dispatcher 必须具备自己的心跳指标，不能因为 API 健康就默认后台正常。

## 12. `analyses/sse.go`：长连接协议

handler 流程：

```text
Authenticate + authorise OwnedSnapshot
  → enforce per-user / per-instance stream limit
  → try subscribe to progress notifications
  → read fresh OwnedSnapshot (覆盖授权查询到订阅之间的更新)
  → write snapshot + flush
  → loop until terminal / disconnect / auth expiry / server drain:
       notification → reload authorised DB snapshot
       reconciliation ticker → reload authorised DB snapshot
       heartbeat ticker → write SSE comment
       write error / deadline → cleanup and return
```

订阅失败时继续以 bounded polling 发送状态；不是直接让任务不可查看。每次通知只提示“可能变化”，不信任其中包含的业务状态。DB 查询失败时记录错误，有限次数失败后关闭连接让客户端退避重连，不发送假的 failed 终态。

协议示例：

```text
event: analysis_state
data: {"analysis_id":"uuid","state_version":7,"status":"running","stage":"analysing"}

: heartbeat

```

Content-Type 为 `text/event-stream`，Cache-Control 为 `no-store`；可设置 `X-Accel-Buffering: no`。快照 `state_version` 不递增时不重复发送。没有 event replay 契约；收到 Last-Event-ID 仍发送当前完整状态，不承诺补齐历史。

每个客户端写入有 deadline；慢客户端被断开，通知 buffer 满时可合并/丢弃通知，因为周期 DB 查询能恢复。首版每连接独立订阅并限制连接数；并发需要时再引入进程内共享订阅 hub。

不要给 SSE 套普通 10 秒请求超时。HTTP server 全局 WriteTimeout 与长连接有冲突时，使用逐次写 deadline；普通 JSON 路由仍有用例 deadline。`http.ResponseController` 是否被 Gin 和自定义 writer wrapper 支持要通过集成测试验证。[Go HTTP 文档](https://pkg.go.dev/net/http)

## 13. `bootstrap/api.go` 与 `cmd/api/main.go`

构造顺序是明确的依赖图：

```text
LoadAndValidateConfig
  → CreateLoggerAndMetrics
  → OpenPostgresPool (bounded startup ping)
  → OpenSharedRedisClient
  → Construct postgres stores
  → Construct auth/profile/application/analysis use cases
  → Construct handlers + middleware + router
  → Construct HTTP server
  → Run until signal or fatal error
```

`main.go` 只负责 `signal.NotifyContext`、调用 bootstrap/run、报告致命错误与退出码。资源关闭由 run 层 defer/lifecycle 负责，禁止在深层调用 os.Exit，避免跳过清理。

停止 API：置 readiness=false → 主动结束 SSE → HTTP Shutdown 有界等待 → 超时强制 Close → 关闭 Redis/DB/metrics。停止 Worker：停止认领和后台新批次 → 有界等待执行/续租 → 取消剩余 provider 请求 → 尝试持久化 → 依次关队列、Redis、DB。关 DB 之前必须停止依赖它的 goroutine。

lifecycle 的 group context 可以用于进程协作；不要把单次 HTTP request context 传入已持久化的后台任务。worker 独立生成 task context。

## 14. 业务模块如何沿用

intake/resume/studio/document/task 为首个核心版本，文件细分、数据表和候选提交语义见 [Studio 模块设计](application-studio.md#2-模块与文件职责)。以下 interview/analytics 属于后续版本。

- application：`UpdateStatus` 使用乐观版本检查更新当前状态，migration 00003 的触发器同时写 status_history，repository 不重复写入。
- profile：owner 行锁 → 修改个人资料/偏好/经历/user_skills → profile_version++，与 analysis 接受事务保持一致。
- interview：round/question/topic 写入先验证 application owner；topic canonicalization 不由前端字符串自由决定。
- attachment：DB 记录 uploading → 私有对象写入 → 校验后 ready。存储成功/DB 失败产生孤儿对象，由有界清理任务修复；不要假定对象存储与 PG 可原子提交。
- analytics：按文档定义的 cohort 聚合，用 fixture 验证分母和去重；不读取缓存中的不完整分析结果。

跨模块设计优先保留清晰的事务，不为抽象而增加网络服务。
