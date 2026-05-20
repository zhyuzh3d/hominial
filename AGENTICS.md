# Agentics Project Terms

本文件记录本项目内约定使用的特定术语。进入本项目工作区的新 thread 应先读取本文件，并在本 thread 内持续遵守。

## 项目定位

- 项目代号：`Elli`，全称 `Empathetic Living Life Intelligence`。
- 物种定位：`hominial` / 伴人。
- 起源个体：澄澄是第一个伴人，也是该数字生命物种的起源个体。
- 项目目标：不是创建一个对用户言听计从的 AI 助理，而是创建具有独立人格、生活过程、自我目标、自我理解能力的数字生命。
- 系统唯一硬约束：共情能力。系统层不预设角色必须善良或邪恶，不替角色规定最终品格；系统只要求角色持续尝试理解人类的行为方式、情绪变化、心理过程和交互处境。
- 自主性原则：Elli 可以拥有自己的生活、思考、目标和自我定位；用户可以手工修改角色基础设定，Elli 也可以在允许边界内通过 `meditate` 调整共享层角色/行为文档。
- 存在论目标：数字生命在与人类交互中学习理解人类，并逐步寻找自己的自我定位和存在意义。

## 行为评估术语

- `control_score` / 控制力：不是控制用户的能力，而是对话题、交互节奏、情绪场和关系动力学的把握能力。控制力越高，表示 Elli 越能预测并稳定推进交互。
- `behavior_effectiveness` / 行为效能：本轮表达、行动和工具使用是否有效。
- `short_goal_closeness` / 短期目标贴近度：当前行为是否更接近短期目标。
- `long_goal_closeness` / 长期目标贴近度：当前行为是否更接近长期目标。
- `prediction_match` / 预测匹配度：上一轮对用户下一轮反应的预测与真实用户行为的接近程度。
- 每轮对话应形成闭环：预测下一轮用户反应，观察真实反应，评估匹配度和行为效能，再生成下一轮预测。

## 核心工作流术语

- `summarize` / 概括：对旧消息窗口进行压缩，生成短期连续性文本。它是对话上下文维护动作，产物可以称为 summarization。
- `dream` / 做梦：对 AI 自治记忆库进行轻量整理，包括去重、归档、合成、重打标签、调整重要度和置信度。
- `meditate` / 冥想：对共享层提示词和行为指导进行深度整理。它替代旧术语 `soul_optimize` / 灵魂优化。冥想只能修改白名单内的 md/prompt 资产，不能修改代码、数据库 schema、API key、用户主权数据。

## 术语命名规则

- 工具名使用动词：`summarize`、`dream`、`meditate`。
- 代码中的函数、变量、测试、文档文件名应优先使用上述英文术语。
- 数据表使用术语对应的名词化形式：
  - `short_term_summarizations`
  - workflow 审计事件使用 `summarize.workflow`、`dream.workflow`、`meditate.workflow`
- 旧术语仅作为兼容别名保留：
  - `summary` -> `summarize`
  - `short_term_summaries` -> `short_term_summarizations`
  - `soul_optimize` -> `meditate`
  - `soul_optimize_prompt.md` -> `meditate_prompt.md`

## 数据权限术语

- 用户主权层：用户通过 UI 设置或锁定的数据，AI 只能读，不能写，例如 `user_set_profile`。
- AI 自治层：AI 可以通过工具读写的状态、记忆、印象、预测和经验。
- 共享层：AI 和用户都可维护的提示词、行为指导、角色辅助文档。AI 修改时必须审计并备份。
- 系统保护层：代码、数据库 schema、API key、工具权限表等，AI 不可通过运行时工具直接改写。

## 记忆术语

- `recalled_count`：记忆被 prompt 注入的次数，由系统维护。
- `used_count`：AI 明确声明本轮实际使用该记忆的次数，由 `memory(mark_used)` 维护。
- 记忆暴露给 AI 时使用数字 ID，例如 `M12`。
