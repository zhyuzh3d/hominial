# Intelligent Content Workflows

## 1. Goal

The app needs two higher-order background workflows:

- `dream`: a lightweight memory consolidation loop that keeps the memory base compact, tagged, ranked, and useful.
- `meditate`: a deeper periodic optimization loop that extracts behavior lessons and updates AI-owned/shared prompt documents without touching source code or user-owned settings.

Both workflows use the unified tool runtime. They must be auditable, bounded, and safe to run unattended.

## 2. Workflow Safety Model

Each workflow has four stages:

1. Collect: gather recent messages, memories, state, and allowed documents.
2. Think: generate a plan using deterministic rules and, when configured, a model call.
3. Apply: execute only whitelisted operations.
4. Audit: persist inputs summarize, proposed changes, applied changes, skipped changes, and errors.

AI-generated plans are advisory until validated by local policy.

## 3. Dream

Dream operates on AI-owned memories only.

Inputs:

- active memories
- memory categories/tags
- recalled/used counts
- recent messages
- threshold and operation

Allowed actions:

- create synthesized memory
- patch category/tags/rank/confidence
- archive duplicate or low-value memories
- write audit events

Disallowed actions:

- delete rows
- edit user-set profile
- edit role source documents
- edit source code

Default behavior:

- `check`: return counts and candidate IDs.
- `run`: collect candidates, consolidate exact duplicates deterministically, then optionally apply model-generated memory patches if API is configured.
- `schedule`: ensure the hourly dream schedule exists.

## 4. Meditate

Meditation operates on shared prompt/document assets, not source code.

Allowed documents:

- `behavior_guidance.md`
- `prompts/summarize_prompt.md`
- `prompts/dream_prompt.md`
- `prompts/selfie_prompt.md`
- `prompts/meditate_prompt.md`

Protected documents:

- source code
- database schema
- API/config files
- user-set profile
- locked future documents

Inputs:

- recent messages
- recent tool events
- active memory index
- character setting
- current allowed prompt documents

Multi-step reasoning:

1. Extract behavior lessons from recent dialogue.
2. Identify stable character guidance improvements.
3. Improve summarize/dream/selfie/meditation prompt templates when evidence supports it.
4. Produce full replacement content for allowed documents.
5. Validate each document path and content size.
6. Apply accepted changes and audit all decisions.

If no API key is configured, the workflow still records a deterministic status audit and does not modify documents.

## 5. Audit Requirements

Every workflow run writes an `orchestrator_events` record with:

- workflow name
- operation
- counts
- candidate IDs
- applied changes
- skipped changes
- error or warning
- timestamp

Applied document changes also write timestamped backups under `app_outputs/workflow_backups/`.

