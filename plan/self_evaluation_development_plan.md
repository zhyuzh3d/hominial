# Self-Evaluation Development Plan

## Phase 1: Schema

- Add `turn_evaluations`.
- Store assistant message ID, previous prediction, actual user behavior, prediction match, goal vectors, strategy, next prediction, control score, behavior effectiveness, and raw JSON.
- Add indexes by thread/time and assistant message.

## Phase 2: Tool Surface

- Add model-visible `evaluate_turn`.
- Keep it compact and structured.
- The tool writes both history and current state.

## Phase 3: Runtime Computation

- Compute actual reply latency from message timestamps when possible.
- Preserve model-provided latency if deterministic computation is not available.
- Clamp scores to valid ranges.
- Convert goal vector fields to existing role state fields:
  - closeness = `100 - distance`
  - deviation = `100 - angle`

## Phase 4: Prompt Integration

- Inject recent turn evaluation summary.
- Tell Elli that `control_score` is predictive empathy over interaction dynamics, not control over the user.
- Require `evaluate_turn` after each meaningful assistant response.

## Phase 5: Dream And Meditate Integration

- Include recent turn evaluations in `meditate` context.
- Let `dream` synthesize stable `dialogue_experience` memories from repeated interaction strategy patterns.

## Phase 6: Tests

- Verify `evaluate_turn` inserts a history row.
- Verify it updates role/user current state.
- Verify latency is computed when there is a previous assistant/user pair.
- Verify dream can see turn evaluations and synthesize interaction experience.

