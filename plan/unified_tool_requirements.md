# Unified Tool Runtime Requirements

## 1. Purpose

The application should treat every model-requested action as a tool call. A tool call may update private state, query memory, generate assets, notify the user, schedule future work, or continue the current turn through a callback. The runtime should be small, composable, permissioned, and easy to expose to the model without filling the prompt with dozens of narrow tools.

The model-visible surface is a whitelist. Only tools intended for direct model use are injected into the conversation context. Internal tools may exist but are never exposed unless explicitly allowed by the active tool policy.

## 2. Core Principles

- One architecture: all side effects, lookups, state updates, scheduled jobs, and asset generation are tool calls.
- Callback as tool: a callback is another tool invocation, usually `sendmsg`.
- Compact surface: prefer a small number of parameterized tools over many specific tools.
- Permissioned data: tables and fields have read/write capabilities. User-owned data is not writable by the model.
- Rich messages: assistant output is a message object, not only a string. Text, images, code, files, notifications, and metadata must be representable.
- Dynamic memory: memories support numeric IDs, free model-managed categories, and tags.
- Model autonomy with audit: the model can operate in AI-owned and shared layers, while every write is logged and reversible in principle.
- Deterministic logic remains deterministic: app-computed values may be updated by code, but subjective usage, salience, and interpretation should be supplied by the model through tools.

## 3. Round Output Model

Each assistant turn should be interpreted as a set of user-visible messages plus tool calls.

```json
{
  "messages": [
    {
      "target": "user",
      "kind": "text",
      "content": "我在。",
      "attachments": [],
      "metadata": {}
    }
  ],
  "tool_calls": [
    {
      "tool": "memory",
      "args": {
        "operation": "mark_used",
        "ids": [12, 18]
      },
      "callback": null
    }
  ]
}
```

The current Responses API function-call stream is the transport for this model. The app may continue to persist ordinary assistant text, but internally message records should move toward a typed message format.

## 4. Model-Visible Tool Whitelist

Default model-visible tools:

- `db`: permissioned reads and writes for compact state tables.
- `memory`: create, patch, tag, score, mark used, merge, archive, and restore memories.
- `query`: search `memories`, `knowledge`, or `messages`.
- `sendmsg`: send a message to the user, to the model, or to the internal event stream.
- `selfie`: generate a character image using configured reference assets and a prompt.
- `notify`: send or schedule user notifications.
- `schedule`: create, list, update, or cancel scheduled tool calls.
- `summarize`: request or run conversation summarization maintenance.
- `dream`: run lightweight memory consolidation.
- `meditate`: run the daily multi-step prompt and character meditation workflow.

Internal-only tools may exist for implementation, but they should not be injected into the prompt unless promoted to the whitelist.

## 5. Callback Contract

Every tool call may include an optional callback:

```json
{
  "tool": "query",
  "args": {
    "source": "memories",
    "keywords": ["生日", "拍照"],
    "limit": 5
  },
  "callback": {
    "tool": "sendmsg",
    "args": {
      "target": "ai",
      "mode": "tool_result"
    }
  }
}
```

Callback execution is itself a tool call. The parent tool produces a result object; the callback may receive that result as its default payload unless explicit callback parameters override or transform it.

Common `sendmsg` targets:

- `user`: append a user-visible message.
- `ai`: send tool result back into the model for continued generation.
- `internal`: append an internal event for later context or auditing.
- `notification`: emit or persist a notification event.

## 6. Data Ownership Layers

### AI Autonomous Layer

The model may read and write:

- role state
- user context
- user estimated profile
- AI-owned memories and knowledge
- dialogue experience
- prediction and evaluation metadata
- tags and categories

### Shared AI/User Layer

The model may propose or write depending on user lock settings:

- character setting documents
- behavior guidance
- prompt templates
- selfie templates
- summarize and dream prompts

Every shared-layer write must be audited with source evidence and reason.

### User Sovereign Layer

The model may read but not write:

- UI-configured user profile (`user_set_profile`)
- user privacy settings
- user-locked character rules
- user-managed schedules marked manual-only

### System Protected Layer

The model cannot directly write:

- source code
- database schema
- API keys
- provider/model configuration
- tool registry permissions

## 7. Database Tool Requirements

`db` should expose compact permissioned operations:

- `read`
- `insert`
- `update`
- `upsert`
- `patch`
- `delete` or `archive` where allowed

The app validates:

- table permission
- operation permission
- field whitelist
- JSON shape
- row ownership
- idempotency key when provided

The model cannot write `user_set_profile`. It can write `user_estimated_profile` or equivalent estimated JSON fields.

## 8. Memory Requirements

Memories should use AI-friendly numeric IDs while preserving stable internal IDs if needed. Each memory should support:

- numeric ID
- content
- category
- tags
- importance/rank
- confidence
- recalled count
- used count
- last recalled time
- last used time
- source and source message
- status
- metadata JSON

The distinction between recall and use is required:

- `recalled_count`: the app injected the memory into prompt context.
- `used_count`: the model explicitly marked the memory as used through the `memory` tool.

Context should include the current category/tag index in compact form so the model can reuse existing taxonomy or create new categories when useful.

Default memory recall:

- top 5 important memories
- random 5 memories

## 9. Query Requirements

`query` supports only:

- `memories`
- `knowledge`
- `messages`

Profile and world/environment state are loaded into the prompt as current context and should not need query tools.

Query parameters:

- source
- keywords
- limit, default 5
- category/tag filters where applicable
- time range where applicable

The typical callback is `sendmsg` with target `ai`, allowing the model to continue the current turn after retrieval.

## 10. Context And Compression Defaults

Prompt context defaults:

- recent messages: 40
- memory recall: 5 top + 5 random
- summarize refresh: when uncompressed messages exceed 40, compress the older 20

Conversation summarization preserves continuity. Long-term memory preserves stable facts, events, skills, preferences, and experience. These should be related but not conflated.

## 11. Notify And Scheduling

The runtime should support:

- immediate notification
- scheduled notification
- recurring notification
- scheduled internal tool call
- list/update/cancel schedule

User-created schedules and model-created schedules should be distinguishable. Schedules can trigger tools such as `notify`, `dream`, or `meditate`.

## 12. Dream Workflow

Dream is a lightweight scheduled consolidation process:

- default check interval: 1 hour
- default threshold: 100 unconsolidated memories
- compress older unconsolidated memories over threshold
- merge duplicates
- retag and recategorize
- update importance and confidence
- regenerate compact category/tag indexes

Dream should not rewrite source code or protected settings.

## 13. Meditation Workflow

Meditation is a daily multi-step internal workflow, not a single model call.

Steps:

1. collect recent dialogue and memory changes
2. extract behavior lessons
3. update behavior guidance
4. review character setting
5. optimize prompt templates
6. produce and apply an allowed document patch
7. write an audit event

It may edit shared prompt/character documents where allowed, but never source code or protected configuration.

## 14. Reliability

The runtime must:

- persist every tool call and callback
- keep parent/child call relationships
- record status and error messages
- deduplicate by call ID or idempotency key
- avoid breaking user-visible chat when noncritical state tools fail
- make unsupported tools visible in audit logs

