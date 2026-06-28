# Issue 标签规则

本文档定义本仓库 issue 标签的本地约定。目标是保持标签少而稳定，便于筛选、分派和自动化处理；一次性上下文优先写在 issue 标题或正文里，不为单次任务创建新标签。

## 基础规则

每个 issue 通常包含：

- 一个执行模式标签：`afk` 或 `hitl`
- 一个类型标签：`type:*`
- 一到两个领域标签：`area:*`

优先复用 issue 跟踪系统中已有标签。新增标签前，应确认该维度未来会被持续用于筛选、统计或分派。

## 执行模式

- `afk`：Agent 可以根据 issue 描述和验收标准独立实现，不需要中途等待人工做产品、架构或发布决策。
- `hitl`：需要人工参与决策，例如架构边界、产品取舍、数据迁移策略、发布策略或设计方案需要人工确认。

同一个 issue 只贴一个执行模式标签。

## 类型

- `type:feat`：新增用户可见能力，或完整的技术能力。
- `type:fix`：修复已有缺陷或行为回归。
- `type:docs`：文档变更。
- `type:test`：只补充或调整测试。
- `type:refactor`：不改变外部行为的重构。
- `type:chore`：CI、构建、依赖、工具链和维护性工作。

同一个 issue 通常只贴一个 `type:*` 标签。

## 领域

最小常用领域：

- `area:database`
- `area:postgres`
- `area:ci`
- `area:docs`

按需增加的领域：

- `area:config`
- `area:migrations`
- `area:maintenance`
- `area:concurrency`
- `area:auth`
- `area:startup`
- `area:nodes`
- `area:observations`
- `area:subscriptions`
- `area:profiles`
- `area:credentials`
- `area:evaluations`
- `area:request-logs`

领域标签应描述 issue 主要影响的系统区域。不要把过细的实现细节做成标签。

## PostgreSQL 支持任务示例

PostgreSQL 支持相关的实现 issue 建议使用：

- `afk`
- `type:feat`
- `area:database`
- `area:postgres`
- 另加一个具体模块领域，例如 `area:migrations`、`area:nodes` 或 `area:evaluations`

CI issue 建议使用：

- `afk`
- `type:chore`
- `area:ci`
- `area:postgres`

文档 issue 建议使用：

- `afk`
- `type:docs`
- `area:docs`
- `area:postgres`

## Gitea 标签说明建议

在 Gitea 仓库标签设置中，可以为常用标签填写简短的中文说明：

- `afk`：Agent 可以独立实现，不需要等待人工决策。
- `hitl`：实现前或实现过程中需要人工确认决策。
- `type:feat`：新增用户可见能力或完整技术能力。
- `type:fix`：修复缺陷或行为回归。
- `type:docs`：仅涉及文档变更。
- `type:test`：仅涉及测试变更。
- `type:refactor`：不改变外部行为的代码重构。
- `type:chore`：CI、构建、依赖、工具链或维护性工作。
- `area:database`：数据库和存储行为。
- `area:postgres`：PostgreSQL 存储支持。
- `area:ci`：持续集成。
- `area:docs`：文档。
