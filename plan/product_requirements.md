# Eibanban Comprehensive Upgrade PRD

## 1. Product Goal

Eibanban should evolve from a minimal single chat client into a stable native desktop companion system. The app must preserve the current direct conversation workflow while adding professional message presentation, high-volume history performance, persistent character cognition, and extensible function-call orchestration.

The core product idea is that the AI character lives in a coherent virtual world. It has time awareness, a changing environment, long-term and short-term goals, an evolving view of the user, and an internal feedback loop: predict the user's reaction, compare it with the actual reply, score its control and effectiveness, then adjust its next behavior.

## 2. User Experience Requirements

### 2.1 Chat Surface

- Show messages as mature chat bubbles similar to WeChat and LINE.
- Align user messages to the right and assistant messages to the left.
- Use restrained avatars or role marks, compact timestamps, readable bubble widths, and small image thumbnails.
- Preserve the existing simple layout: header/config area, scrollable message list, composer, image preview overlay.
- Do not add decorative overlays, background illustrations, or interaction layers that can block scroll, hover, or clicks.

### 2.2 Image Interaction

- Image attachments must preview before sending and appear in the message list after sending.
- Message thumbnails must be clickable.
- The enlarged preview must support drag panning and explicit controls: zoom in, zoom out, 1:1, fit, close.
- Preview controls should stay at the bottom center over the image without blocking the main app when preview is closed.

### 2.3 Large History

- The app must support tens of thousands of messages without loading the entire thread into memory.
- Startup should load the latest window only.
- Older messages should be loaded incrementally into the visible window.
- Saving a new message must append to SQLite and must not rewrite or delete unloaded history.

## 3. Prompt Engineering Requirements

Each model request should be assembled by a PE builder from these components:

- Role setting: up to 2k characters from `character.md`, covering persona, personality, history, behavior rules, appearance, and goals.
- Long-term memory recall: top `n` important memories and random `m` memories from SQLite. Importance combines recency, recall count, and rank.
- Short-term memory compression: a cumulative summary of messages older than the recent message window, updated periodically through a model summary call.
- Recent `k` messages: newest conversation messages with attachment/image references when present.
- Role context: health, mental state, mood, current action, current purpose, short-term/long-term goal scores, control score, behavior effectiveness score.
- User profile: user-set fields and character-estimated fields.
- User context: estimated user mood, action, environment, next action prediction, and prediction-vs-actual evaluation.
- Environment context: injected current date/time, weekday, approximate lunar date placeholder, virtual scene, surrounding environment, and natural random variations.
- Function list/schema: orchestrator functions that allow the character to update state and request automatic reference-image generation.

The total prompt should target roughly 24k characters. Configurable `m`, `n`, and `k` values should allow future tuning. Hard truncation is allowed for low-priority sections.

## 4. Orchestrator Requirements

The orchestrator is the hub for model-requested function calls. It must:

- Register function schemas in a central place.
- Parse function calls from Responses API output.
- Execute approved local functions.
- Persist function calls and results.
- Update long-term memories, role context, user profile, user context, environment state, and summary metadata.
- Support extension without scattering function handling throughout UI or API code.

## 5. Automatic Reference-Image Drawing

The model can call `create_reference_image` with a prompt and local reference image paths. The orchestrator should:

- Validate and preprocess the local images.
- Create a follow-up Responses API request with `image_generation`.
- Include the reference images and the model-provided drawing prompt.
- Save generated images to `app_outputs/images`.
- Append the generated image as an assistant message in the current thread.

## 6. Non-Goals For This Upgrade

- Multi-window UI.
- Full account management.
- Network-synchronized data.
- A visual prompt editor.
- Full lunar calendar accuracy; a placeholder label is acceptable until a dedicated calendar module is added.

## 7. Acceptance Criteria

- The app starts and restores recent history from SQLite.
- New messages append without damaging older rows.
- Older messages can be loaded into the window.
- PE builder injects role, memories, summary, state, user, environment, functions, and recent messages.
- Function calls are logged and handled by the orchestrator.
- `create_reference_image` can generate an assistant image message from local reference image paths.
- Chat UI uses stable left/right bubble layout and remains scrollable/clickable.
- `go test ./...` passes.
