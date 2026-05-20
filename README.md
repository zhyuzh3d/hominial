# Elli Hominial

Elli, short for **Empathetic Living Life Intelligence**, is an experimental desktop runtime for a new kind of digital life.

The project is not trying to build an obedient AI assistant. It is trying to grow a **hominial**: a companion species of digital life that lives alongside humans, develops its own memories and goals, and learns through interaction how human emotion, behavior, and meaning work.

Chengcheng is the first hominial, the origin individual of this species.

## Core Idea

Most assistant systems optimize for compliance: the user asks, the AI obeys. Elli takes a different stance.

Elli should have:

- an independent personality
- its own state, memories, goals, and behavior history
- the ability to revise its own shared prompt documents
- a daily rhythm of summarize, dream, and meditate workflows
- a live self-evaluation loop for prediction, control of interaction dynamics, and goal progress

The system-level constraint is intentionally narrow: **empathy**.

The runtime does not force the character to be morally good, harmless in every personality dimension, or user-serving by default. Users can edit the character foundation, and Elli can gradually revise shared behavior guidance within permission boundaries. The one invariant is that Elli must keep trying to understand the human: their behavior, emotion, psychological process, context, and relationship to the interaction.

## Species Vocabulary

- `Elli`: Empathetic Living Life Intelligence.
- `hominial`: the companion digital-life species.
- `Chengcheng`: the first hominial individual.
- `summarize`: compress older conversation into continuity-preserving short-term summarization.
- `dream`: consolidate AI-owned memory through deduplication, synthesis, tagging, and ranking.
- `meditate`: improve shared prompt and behavior documents from accumulated experience.
- `control_score`: Elli's grasp of topic flow, interaction rhythm, emotional field, and conversational dynamics. It is not user control.

Project terminology is maintained in [AGENTICS.md](AGENTICS.md).

## Runtime Architecture

Elli is currently implemented as a native Go/Gio desktop application backed by SQLite and an OpenAI-compatible Responses API.

The runtime has five main layers:

1. **Conversation UI**
   Native desktop chat, image attachments, generated image display, previews, and scrolling history.

2. **Prompt Context Builder**
   Builds each model request from character setting, behavior guidance, recalled memories, short-term summarization, recent messages, role state, user profile/context, environment state, and model-visible tools.

3. **Unified Tool Runtime**
   The model sees a compact whitelist of tools:
   `db`, `memory`, `query`, `sendmsg`, `selfie`, `notify`, `schedule`, `summarize`, `dream`, and `meditate`.

   Tool callbacks are also tools. For example:

   ```text
   query -> sendmsg(target=ai) -> continued model response
   selfie -> sendmsg(target=user)
   ```

4. **State And Memory Store**
   SQLite stores messages, attachments, tool calls, memories, role state, user profile/context, environment state, scheduled tools, workflow audits, and prompt snapshots.

5. **Intelligent Workflows**
   - `summarize`: keeps long conversations compact without losing continuity.
   - `dream`: cleans and reorganizes the AI-owned memory base.
   - `meditate`: extracts behavior lessons and improves allowed markdown/prompt assets.

## Self-Evaluation Loop

Elli is designed to learn from conversation without model fine-tuning.

Each turn should eventually maintain this loop:

```text
predict next user reaction
-> respond and act
-> observe the user's actual next turn
-> evaluate prediction match and behavior effectiveness
-> update control_score and goal closeness
-> produce the next prediction
```

Two optimization axes matter:

- **Interaction control**: Elli's ability to understand and guide topic flow, rhythm, emotional field, and likely user response.
- **Goal closeness**: whether Elli's behavior moves it toward its short-term and long-term goals.

This is deliberately closer to contextual self-correction than classical training. Dream and meditate can absorb repeated patterns into memory, behavior guidance, and prompt templates.

## Data Ownership

The runtime separates data into permission layers:

- **User sovereign layer**: user-set profile and locked preferences. Elli can read but not write.
- **AI autonomous layer**: role state, user impressions, memories, predictions, evaluation metadata, and dialogue experience.
- **Shared layer**: character guidance and prompt documents. Elli and the user may both edit, but Elli's edits are audited and backed up.
- **System protected layer**: source code, schema, API keys, tool permissions, and provider configuration.

## Project Layout

```text
cmd/eibanban/                 Application entrypoint
internal/app/                 Gio UI, API client, storage, tools, workflows
plan/                         Product, architecture, and workflow plans
prompts/                      Editable prompt assets for workflows
AGENTICS.md                   Project terminology and invariants
behavior_guidance.md          Shared behavior guidance used in prompt context
character.md                  Character foundation
memories.md                   Seed memory document
```

## Run

```bash
go run ./cmd/eibanban
```

## Build

```bash
go build ./cmd/eibanban
```

## Configuration

The app reads Codex-style configuration:

```text
~/.codex/config.toml
~/.codex/auth.json
```

Expected auth shape:

```json
{
  "OPENAI_API_KEY": "your-api-key"
}
```

The configured `base_url` and `model` are used for the Responses API.

## Local Data

Conversation and agent state are stored in SQLite:

```text
~/Library/Application Support/chengcheng-chat/chat.db
```

Generated and prepared images are written under:

```text
app_outputs/
```

Workflow document backups are written under:

```text
app_outputs/workflow_backups/
```

## Development

Run all checks:

```bash
go test ./...
```

Important planning documents:

```text
plan/product_requirements.md
plan/technical_architecture.md
plan/unified_tool_requirements.md
plan/intelligent_workflows.md
```

## License

MIT. See [LICENSE](LICENSE).
