# Unified Tool Runtime Development Plan

## Phase 1: Documentation And Defaults

- Add a formal requirements document for the unified tool runtime.
- Update default PE settings:
  - 40 recent messages
  - 5 top memories
  - 5 random memories
  - summarize refresh in 20-message chunks after the 40-message window is exceeded.
- Document model-visible tool whitelist behavior.

Status: implemented.

## Phase 2: Schema Evolution

- Extend `long_term_memories` with:
  - numeric model-facing ID
  - category
  - tags JSON
  - confidence
  - recalled count
  - used count
  - last used timestamp
  - status
  - source message ID
- Preserve existing rows through additive migrations.
- Add schedule/task tables:
  - tool name
  - arguments JSON
  - callback JSON
  - run time / interval
  - status
  - owner
- Add parent/callback metadata to tool call logging.

Status: mostly implemented. Memory and schedule schema additions are in place. Tool/callback events are persisted; richer parent-child metadata can still be expanded.

## Phase 3: Tool Registry

- Introduce a tool registry with:
  - model-visible flag
  - schema
  - executor
  - allowed callback names
  - default callback
  - permissions
- Generate Responses API tool schemas from the model-visible whitelist only.
- Keep legacy specialized tools temporarily available through compatibility wrappers.

Status: implemented as a compact model-visible schema registry with legacy execution wrappers retained.

## Phase 4: Unified Tools

- Implement `memory`:
  - upsert
  - patch
  - mark_used
  - score
  - archive
  - restore
- Implement `query`:
  - memories
  - knowledge
  - messages
- Implement `db`:
  - read and patch/upsert for AI-owned tables
  - read-only access for user-owned tables
- Implement `sendmsg`:
  - user messages
  - internal events
  - model-continuation payload recording
- Implement `selfie` as a high-level wrapper around image generation/reference image flow.
- Implement `notify` and `schedule` storage-level support.

Status: implemented. `selfie`, `dream`, and `meditate` currently have safe executable scaffolds; deeper workflows can build on the same runtime.

## Phase 5: Callback Runtime

- Parse callback metadata from tool arguments.
- Execute parent tool.
- If callback exists, execute callback as a child tool with parent result as default payload.
- Persist parent and child call events.
- For `sendmsg(target=ai)`, record the continuation payload first. Full immediate model-continuation can be added after core persistence is stable.

Status: implemented. `sendmsg(target=ai)` now triggers immediate continuation with a bounded depth limit.

## Phase 6: Prompt Builder Integration

- Inject only model-visible tool schemas.
- Add memory category/tag index to prompt context.
- Stop treating prompt injection as memory use:
  - app updates `recalled_count`
  - model updates `used_count` via `memory(mark_used)`.
- Clarify function policy text around callbacks, `sendmsg`, user-set profile read-only rules, and memory usage marking.

Status: implemented.

## Phase 7: Summarize, Dream, And Meditate Jobs

- Adjust summarize refresh logic to keep 40 recent messages and summarize older messages in 20-message chunks.
- Implement lightweight `dream` scaffold:
  - threshold check
  - candidate selection
  - audit event
- Implement `meditate` scaffold:
  - daily schedule support
  - collect context
  - write audit event
  - restrict edits to prompt/document assets.

Status: partially implemented. Default schedules, threshold checks, and audit scaffolds exist. Full multi-step document optimization remains the next major workflow.

## Phase 8: UI And Operations

- Add compact status visibility for tool/callback errors.
- Add future UI affordances for manual schedules and user-set profile editing.
- Keep existing chat UI functional during backend migration.

Status: implemented. Scheduled user-visible messages are appended to the chat while the app is running.

## Phase 9: Verification

- Run `gofmt`.
- Run `go test ./...`.
- Manually verify:
  - normal text chat
  - memory mark-used
  - query with callback
  - user-visible `sendmsg`
  - image/selfie callback path
  - summarize threshold behavior
- schedule table initialization

Status: implemented and covered by focused unit tests.

## Next Deepening Pass

- Add a first-class typed message table or message metadata format for rich text/image/code/file payloads instead of relying on `content` plus attachments.
- Implement full `sendmsg(target=ai)` Responses API function-result semantics if provider-specific tool result messages are required later; the current implementation sends callback payload as same-turn continuation input.
- Build real dream consolidation:
  - candidate selection
  - model-assisted merge/tag/rank pass
  - reversible memory archive records
- Build real meditation:
  - multi-call workflow runner
  - md document patch generation
  - user-lock checks
  - audit diff review
- Add UI for user-set profile, schedule management, memory browser, and shared-layer document locks.
