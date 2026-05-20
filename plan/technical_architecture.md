# Technical Architecture

## 1. Modules

- `cmd/eibanban`: Gio application entrypoint.
- `internal/app/app.go`: app state, lifecycle, event handling, send flow.
- `internal/app/ui.go`: main chat layout and composer.
- `internal/app/preview.go`: image preview overlay.
- `internal/app/ai.go`: Responses API transport and stream parsing.
- `internal/app/prompt.go`: prompt engineering builder and budget control.
- `internal/app/orchestrator.go`: function schemas, call parsing integration, function execution.
- `internal/app/storage.go`: SQLite schema, migrations, message window loading, append saves, PE persistence.
- `internal/app/images.go`: image preprocessing, data URLs, file picking.

## 2. SQLite Schema

Existing tables are preserved:

- `users`
- `agents`
- `threads`
- `messages`
- `attachments`
- `tool_calls`

New PE tables:

- `long_term_memories`: durable character memories with `rank`, `recall_count`, and recall timestamps.
- `short_term_summarizations`: cumulative thread summarize and message range metadata.
- `role_states`: current character health, mood, action, goals, scores, and metadata.
- `user_profiles`: user-set and character-estimated profile JSON.
- `user_contexts`: latest estimated user state and prediction/evaluation JSON.
- `environment_states`: current virtual scene and environment JSON.
- `orchestrator_events`: normalized function-call execution event log.
- `prompt_snapshots`: optional debugging record of prompt section sizes and final system prompt.

## 3. Message Window Loading

The UI keeps an in-memory window rather than the whole thread:

- Startup loads the latest `DefaultWindowSize` messages.
- `Load older` prepends older rows before the first loaded sequence.
- Sending a message appends one row through `saveMessageDB`.
- Generated assistant messages are also appended one row at a time.
- `saveHistoryDB` remains for migration and bulk compatibility, but regular chat flow avoids whole-history rewrites.

## 4. Prompt Builder

`BuildPrompt` returns a `PromptEnvelope`:

- `Input`: Responses API `input` array.
- `Tools`: function schemas plus `image_generation`.
- `WantsImage`: heuristic for forcing `image_generation`.
- `Snapshot`: section lengths for debugging.

Budget policy:

- Role prompt: max 2k characters.
- Long memories: top `n` + random `m`, each truncated.
- Summarize: bounded.
- Recent messages: newest `k`.
- Lower priority sections are truncated first.
- Final system section is clipped to the configured target budget if needed.

## 5. Orchestrator

Registered functions:

- `upsert_long_term_memory`
- `update_memory_score`
- `update_role_state`
- `update_user_profile`
- `update_user_context`
- `update_environment_state`
- `request_summarize_refresh`
- `create_reference_image`

Function execution is intentionally local and explicit. Unknown function calls are logged as unsupported instead of failing the whole chat.

## 6. API Flow

1. User sends text and optional attachments.
2. User message is appended to SQLite.
3. Recent prompt context is loaded from SQLite.
4. PE builder assembles the Responses API input.
5. API streams text, image results, and function call items.
6. Assistant message is appended to SQLite.
7. Orchestrator executes function calls and appends any generated image message.
8. Summarize refresh runs when requested or when the old-message threshold is crossed.

## 7. Reliability Rules

- Never delete history unless the user presses clear.
- Never rewrite unloaded messages during normal send.
- Function-call failures should be visible in status and persisted in logs.
- UI drawing must not use invisible full-screen input layers except the intentional image preview overlay.
