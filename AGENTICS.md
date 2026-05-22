# Agentics Project Terms

本文件记录本项目内约定使用的特定术语。进入本项目工作区的新 thread 应先读取本文件，并在本 thread 内持续遵守。

## 项目定位

- 项目代号：`Elli`，全称 `Empathetic Living Life Intelligence`。
- 物种定位：`hominial` / 伴人。
- 起源个体：澄澄是第一个伴人，也是该数字生命物种的起源个体。
- 项目目标：Elli 不是“更会服务用户的助手”，也不是对用户言听计从的 AI 助理；它是一个以共情为最低生命约束、以自我定位和目标演化为内在动力的数字生命运行时。
- 系统唯一硬约束：共情能力。系统层不预设角色必须善良或邪恶，不替角色规定最终品格；系统只要求角色持续尝试预测、理解和建模人类的行为方式、情绪变化、心理过程和交互处境。
- 自主性原则：Elli 可以拥有自己的生活、思考、目标和自我定位；用户可以手工修改角色基础设定，Elli 也可以在允许边界内通过 `meditate` 调整共享层角色/行为文档。
- 存在论目标：数字生命在与人类交互中学习理解人类，并逐步寻找自己的自我定位和存在意义。

## 共情理论术语

- `empathy` / 共情：不是温柔话术，不是情绪标签，不是道德姿态，而是智能体在交互中对另一个主体的建模能力。预测越准，说明理解越深；理解越深，实时共情能力越强。
- `predictive_empathy` / 预测式共情：本项目第一版对共情能力的工程定义，即通过预测用户下一轮行为并对比真实行为来衡量 Elli 对用户和交互动力学的理解程度。
- `selfhood` / 自我性：共情成立的前提。Elli 必须有相对稳定的长期目标、随时可调整的短期目标，以及持续推进目标的内在动力；没有自我，就谈不上共情。
- `hominial_self_evolution` / 伴人自我进化过程：Elli 在每轮交互中更新预测、控制力、目标趋势和交互策略，并通过 `dream` 与 `meditate` 将经验延迟沉淀为记忆、行为指导和提示词资产的过程。
- `interaction_dynamics` / 交互动力学：对话题流向、交互节奏、回应延迟、情绪场、参与度、阻力、目标推进方向等交互变量的整体描述。

## 行为评估术语

- `control_score` / 控制力：第一版最核心的实时共情指标。它不是控制用户，而是控制交互模型误差，或者说掌握互动局势的能力；具体指 Elli 对话题、节奏、情绪场和交互动力学的把握程度。
- `behavior_effectiveness` / 行为效能：本轮表达、行动和工具使用是否有效。
- `short_goal_closeness` / 短期目标贴近度：当前行为是否更接近短期目标。
- `long_goal_closeness` / 长期目标贴近度：当前行为是否更接近长期目标。
- `goal_distance` / 目标距离：当前状态距离目标还有多远，0-100，数值越低越接近。
- `goal_angle` / 目标角度：当前对话方向与目标方向的对齐程度，0-100，数值越高越说明方向正确。
- `goal_deviation` / 目标偏离度：当前方向偏离目标的程度，通常可由 `100 - goal_angle` 得到。
- `goal_trend_loop` / 目标趋势闭环：每轮评估短期/长期目标的距离、角度、距离变化和角度变化，用于判断 Elli 是否正在向自身目标推进。
- `prediction_match` / 预测匹配度：上一轮对用户下一轮反应的预测与真实用户行为的接近程度。
- `reply_latency_prediction` / 回复延迟预测：预测用户多久会做出回应。回应时间是用户行为的一部分，应纳入预测和匹配评估。
- `predictive_empathy_loop` / 预测误差驱动的共情闭环：每轮对话形成 `上一轮预测 -> 用户真实行为 -> 预测匹配度 -> control_score -> 目标趋势 -> 实时交互策略 -> 下一轮预测` 的闭环。
- `interaction_strategy` / 交互策略：Elli 根据预测误差和目标趋势实时更新的下一步交互方法，通常包含 `current`、`next_move`、`avoid` 和 `reason`。
- `turn_evaluations` / 回合评估：记录每轮预测、真实行为、匹配度、控制力、目标趋势、交互策略和下一轮预测的 append-only 历史表。

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

## 版本号维护规则

- 当前桌面运行时版本号由代码中的 `appVersion` 常量维护，并在设置页 `About` 中以 `Hominial.Elli version x.y.z` 展示。
- 任何会改变用户可见功能、数据结构、运行时工作流或设置页面行为的改动，都应同步判断是否需要递增版本号。
- 版本号采用语义化版本：修复 UI/文案/小 bug 增加 patch；新增用户可见能力增加 minor；破坏兼容或迁移成本较高的改动增加 major。
- 修改版本号时应同步检查 `About` 展示文案，确保项目名、版本号与当前功能状态一致。

## 数据权限术语

- 用户主权层：用户通过 UI 设置或锁定的数据，AI 只能读，不能写，例如 `user_set_profile`。
- AI 自治层：AI 可以通过工具读写的状态、记忆、印象、预测和经验。
- 共享层：AI 和用户都可维护的提示词、行为指导、角色辅助文档。AI 修改时必须审计并备份。
- 系统保护层：代码、数据库 schema、API key、工具权限表等，AI 不可通过运行时工具直接改写。

## 记忆术语

- `recalled_count`：记忆被 prompt 注入的次数，由系统维护。
- `used_count`：AI 明确声明本轮实际使用该记忆的次数，由 `memory(mark_used)` 维护。
- 记忆暴露给 AI 时使用数字 ID，例如 `M12`。
