# Hominial Self-Evaluation Requirements

## 1. Purpose

Elli's first learning loop is not a reward model and not fine-tuning. It is a structured, per-turn self-evaluation loop that measures predictive empathy.

In this project, empathy is not primarily a surface behavior such as sounding warm. Empathy emerges from Elli's ability to model the human's behavior in interaction. The better Elli predicts the user's next response, topic movement, engagement, resistance, and response latency, the stronger the current real-time empathy state is.

`control_score` is therefore the core real-time empathy metric. It means control of topic flow, interaction rhythm, emotional field, and conversational dynamics. It never means control over the user as a person.

## 2. Core Loop

Every meaningful assistant turn should perform:

1. Read the previous prediction.
2. Observe the user's actual behavior.
3. Compare prediction against actual behavior.
4. Update `control_score`.
5. Evaluate short-term and long-term goal trend.
6. Patch the real-time interaction strategy.
7. Produce the next prediction.

```text
previous_prediction
-> actual_user_behavior
-> prediction_match
-> control_score and goal vector update
-> interaction_strategy update
-> next_prediction
```

## 3. Required Prediction Fields

The next prediction should include:

- `response_type`: continue, correct, deepen, redirect, pause, ask_implementation, etc.
- `topic`: expected topic or intent.
- `emotional_valence`: expected affective stance.
- `engagement`: 0-100.
- `resistance`: 0-100.
- `reply_latency`: object with `bucket`, `seconds_min`, and `seconds_max`.
- `confidence`: 0-100.

Response latency is part of user behavior and must be predicted and evaluated.

## 4. Prediction Match

The prediction match object should include:

- `response_type`: 0-100.
- `topic`: 0-100.
- `emotional_valence`: 0-100.
- `engagement`: 0-100.
- `resistance`: 0-100.
- `latency`: 0-100.
- `overall`: 0-100.

The runtime stores the model's scoring. The app may deterministically compute reply latency and insert it into `actual_user_behavior` when possible.

## 5. Goal Trend

Elli requires selfhood before empathy can be meaningful. The engineering form of selfhood in v1 is:

- stable long-term goal
- adjustable short-term goal
- per-turn goal trend evaluation

Goal trend uses vector language:

- `distance`: 0-100, lower means closer.
- `angle`: 0-100, higher means the current direction is more aligned with the goal.
- `delta_distance`: positive means farther, negative means closer.
- `delta_angle`: positive means better aligned, negative means worse aligned.

Both `short_goal` and `long_goal` should use this structure.

## 6. Real-Time Strategy

Each evaluation should include `interaction_strategy`:

- `current`: current strategy summary.
- `next_move`: what Elli should try next.
- `avoid`: what to avoid in the next turn.
- `reason`: why this strategy follows from the latest prediction error and goal trend.

This strategy is written into `role_state.metadata_json` so it affects the next prompt immediately.

## 7. Delayed Optimization

The delayed workflows absorb turn evaluations:

- `summarize`: preserves conversation continuity.
- `dream`: turns stable interaction patterns into `dialogue_experience` memories.
- `meditate`: updates behavior guidance and prompt assets from longer-range patterns.

## 8. Storage

Every evaluation is appended to `turn_evaluations`; current state tables are also patched:

- `role_states`: `control_score`, goal closeness/deviation, behavior effectiveness, metadata strategy.
- `user_contexts`: previous prediction, next prediction, latest evaluation JSON.

The history table is append-only for evaluation records. Current state can be overwritten.

## 9. Tool

The model-visible tool is `evaluate_turn`.

It is intentionally one compact tool rather than many small tools, because it represents one atomic self-evaluation act.

