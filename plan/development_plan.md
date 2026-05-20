# Development Plan

## Phase 1: Stabilize Persistence

- Add message `Seq` to the app model.
- Implement recent-window and older-window SQLite queries.
- Implement single-message append/update saves.
- Keep JSON migration and clear behavior safe.

## Phase 2: Prompt Engineering Foundation

- Add PE data types and default config.
- Extend SQLite schema for memories, summarizations, role state, user profile/context, environment state, function events, and prompt snapshots.
- Seed long-term memories from `memories.md` when the memory table is empty.
- Build the system prompt from role, memory recall, summarize, state, user, environment, and function guidance.

## Phase 3: Orchestrator

- Add function schemas.
- Parse Responses API function calls.
- Execute and persist state update functions.
- Implement `create_reference_image`.

## Phase 4: Chat UI And Window Loading

- Upgrade message bubbles to left/right professional layout.
- Add compact timestamps and smaller thumbnails.
- Add `Load older` support for the current message window.
- Preserve the existing composer and preview behavior.

## Phase 5: Verification

- Run `gofmt`.
- Run `go test ./...`.
- Run the app locally.
- Verify startup, history restore, send, attachments, generated images, preview, older-message loading, and no scroll/click blocking.
