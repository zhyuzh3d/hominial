package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func apiTools() []map[string]any {
	return []map[string]any{
		{"type": "image_generation"},
		functionTool("upsert_long_term_memory", "Create or update an important durable memory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
				"rank":    map[string]any{"type": "integer", "minimum": 0, "maximum": 10},
			},
			"required": []string{"content"},
		}),
		functionTool("update_memory_score", "Update memory rank or recall score.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string"},
				"rank":  map[string]any{"type": "integer", "minimum": 0, "maximum": 10},
				"delta": map[string]any{"type": "integer"},
			},
			"required": []string{"id"},
		}),
		functionTool("update_role_state", "Update the character's private state, goals, and self-evaluation scores.", scoreSchema(map[string]any{
			"health":        map[string]any{"type": "string"},
			"mental":        map[string]any{"type": "string"},
			"mood":          map[string]any{"type": "string"},
			"action":        map[string]any{"type": "string"},
			"short_purpose": map[string]any{"type": "string"},
			"metadata":      map[string]any{"type": "object"},
		})),
		functionTool("update_user_profile", "Update user-set or character-estimated profile fields.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"set":       map[string]any{"type": "object"},
				"estimated": map[string]any{"type": "object"},
			},
		}),
		functionTool("update_user_context", "Update the character's current evaluation and prediction of the user.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mood":                   map[string]any{"type": "string"},
				"action":                 map[string]any{"type": "string"},
				"environment":            map[string]any{"type": "string"},
				"next_action_prediction": map[string]any{"type": "string"},
				"last_prediction":        map[string]any{"type": "string"},
				"evaluation":             map[string]any{"type": "object"},
			},
		}),
		functionTool("update_environment_state", "Update the character's virtual scene and surroundings.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scene":        map[string]any{"type": "string"},
				"surroundings": map[string]any{"type": "string"},
				"metadata":     map[string]any{"type": "object"},
			},
		}),
		functionTool("request_summary_refresh", "Ask the local app to refresh the short-term summary.", map[string]any{
			"type":       "object",
			"properties": map[string]any{"reason": map[string]any{"type": "string"}},
		}),
		functionTool("create_reference_image", "Generate an image using local reference image paths and a drawing prompt.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":          map[string]any{"type": "string"},
				"reference_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"prompt", "reference_paths"},
		}),
	}
}

func functionTool(name, description string, parameters map[string]any) map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        name,
		"description": description,
		"parameters":  parameters,
	}
}

func scoreSchema(extra map[string]any) map[string]any {
	props := map[string]any{
		"short_goal_closeness":   map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"short_goal_deviation":   map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"long_goal_closeness":    map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"long_goal_deviation":    map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"behavior_effectiveness": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
		"control_score":          map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
	}
	for k, v := range extra {
		props[k] = v
	}
	return map[string]any{"type": "object", "properties": props}
}

func parseToolCallMap(m map[string]any) ToolCall {
	typ, _ := m["type"].(string)
	if typ != "function_call" && typ != "tool_call" {
		return ToolCall{}
	}
	name, _ := m["name"].(string)
	if name == "" {
		if fn, ok := m["function"].(map[string]any); ok {
			name, _ = fn["name"].(string)
		}
	}
	if name == "" {
		return ToolCall{}
	}
	args := ""
	for _, key := range []string{"arguments", "arguments_json", "input"} {
		if s, _ := m[key].(string); s != "" {
			args = s
			break
		}
	}
	if args == "" {
		if fn, ok := m["function"].(map[string]any); ok {
			if s, _ := fn["arguments"].(string); s != "" {
				args = s
			}
		}
	}
	if args == "" {
		if raw, err := json.Marshal(m["arguments"]); err == nil && string(raw) != "null" {
			args = string(raw)
		}
	}
	id, _ := m["call_id"].(string)
	if id == "" {
		id, _ = m["id"].(string)
	}
	if id == "" {
		id = newID("call")
	}
	return ToolCall{ID: id, Name: name, Arguments: args, Status: "pending"}
}

func dedupeToolCalls(in []ToolCall) []ToolCall {
	seen := map[string]bool{}
	var out []ToolCall
	for _, c := range in {
		key := c.ID + ":" + c.Name + ":" + c.Arguments
		if c.Name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func runOrchestrator(ctx context.Context, path string, cfg Config, messageID string, calls []ToolCall) ([]Message, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var generated []Message
	var firstErr error
	for _, call := range calls {
		result, msg, err := executeToolCall(ctx, db, cfg, call)
		status := "complete"
		if err != nil {
			status = "failed"
			result = map[string]any{"error": err.Error()}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", call.Name, err)
			}
		}
		logToolCall(db, messageID, call, result, status)
		if msg != nil {
			generated = append(generated, *msg)
		}
	}
	return generated, firstErr
}

func executeToolCall(ctx context.Context, db *sql.DB, cfg Config, call ToolCall) (map[string]any, *Message, error) {
	args := map[string]any{}
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return nil, nil, err
		}
	}
	switch call.Name {
	case "upsert_long_term_memory":
		return upsertLongTermMemory(db, args)
	case "update_memory_score":
		return updateMemoryScore(db, args)
	case "update_role_state":
		return updateRoleState(db, args)
	case "update_user_profile":
		return updateUserProfile(db, args)
	case "update_user_context":
		return updateUserContext(db, args)
	case "update_environment_state":
		return updateEnvironmentState(db, args)
	case "request_summary_refresh":
		return refreshShortSummary(db)
	case "create_reference_image":
		prompt, _ := args["prompt"].(string)
		refs := stringSlice(args["reference_paths"])
		if strings.TrimSpace(prompt) == "" || len(refs) == 0 {
			return nil, nil, fmt.Errorf("prompt and reference_paths are required")
		}
		msg, err := callReferenceImage(ctx, cfg, prompt, refs)
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"generated_images": msg.Images}, &msg, nil
	default:
		return nil, nil, fmt.Errorf("unsupported function %q", call.Name)
	}
}

func upsertLongTermMemory(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	id, _ := args["id"].(string)
	if id == "" {
		id = newID("mem")
	}
	rank := intFromAny(args["rank"], 3)
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO long_term_memories(id, agent_id, user_id, content, rank, source, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, 'function', ?, ?)
		ON CONFLICT(id) DO UPDATE SET content = excluded.content, rank = excluded.rank, updated_at = excluded.updated_at, deleted_at = NULL
	`, id, defaultAgentID, defaultUserID, content, rank, now, now)
	return map[string]any{"id": id}, nil, err
}

func updateMemoryScore(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return nil, nil, fmt.Errorf("id is required")
	}
	rank := intFromAny(args["rank"], -1)
	delta := intFromAny(args["delta"], 0)
	now := time.Now().Format(time.RFC3339Nano)
	if rank >= 0 {
		if _, err := db.Exec(`UPDATE long_term_memories SET rank = ?, recall_count = recall_count + ?, updated_at = ? WHERE id = ?`, rank, delta, now, id); err != nil {
			return nil, nil, err
		}
	} else if delta != 0 {
		if _, err := db.Exec(`UPDATE long_term_memories SET recall_count = recall_count + ?, updated_at = ? WHERE id = ?`, delta, now, id); err != nil {
			return nil, nil, err
		}
	}
	return map[string]any{"id": id}, nil, nil
}

func updateRoleState(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	now := time.Now().Format(time.RFC3339Nano)
	metadata := jsonObjectString(args["metadata"])
	_, err := db.Exec(`
		UPDATE role_states SET
			health = COALESCE(NULLIF(?, ''), health),
			mental = COALESCE(NULLIF(?, ''), mental),
			mood = COALESCE(NULLIF(?, ''), mood),
			action = COALESCE(NULLIF(?, ''), action),
			short_purpose = COALESCE(NULLIF(?, ''), short_purpose),
			short_goal_closeness = CASE WHEN ? THEN ? ELSE short_goal_closeness END,
			short_goal_deviation = CASE WHEN ? THEN ? ELSE short_goal_deviation END,
			long_goal_closeness = CASE WHEN ? THEN ? ELSE long_goal_closeness END,
			long_goal_deviation = CASE WHEN ? THEN ? ELSE long_goal_deviation END,
			behavior_effectiveness = CASE WHEN ? THEN ? ELSE behavior_effectiveness END,
			control_score = CASE WHEN ? THEN ? ELSE control_score END,
			metadata_json = CASE WHEN ? = '{}' THEN metadata_json ELSE ? END,
			updated_at = ?
		WHERE agent_id = ?
	`, stringArg(args, "health"), stringArg(args, "mental"), stringArg(args, "mood"), stringArg(args, "action"), stringArg(args, "short_purpose"),
		hasArg(args, "short_goal_closeness"), intFromAny(args["short_goal_closeness"], 50),
		hasArg(args, "short_goal_deviation"), intFromAny(args["short_goal_deviation"], 0),
		hasArg(args, "long_goal_closeness"), intFromAny(args["long_goal_closeness"], 50),
		hasArg(args, "long_goal_deviation"), intFromAny(args["long_goal_deviation"], 0),
		hasArg(args, "behavior_effectiveness"), intFromAny(args["behavior_effectiveness"], 50),
		hasArg(args, "control_score"), intFromAny(args["control_score"], 50),
		metadata, metadata, now, defaultAgentID)
	return map[string]any{"updated": true}, nil, err
}

func updateUserProfile(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	now := time.Now().Format(time.RFC3339Nano)
	setJSON := jsonObjectString(args["set"])
	estimatedJSON := jsonObjectString(args["estimated"])
	_, err := db.Exec(`
		UPDATE user_profiles SET
			set_json = CASE WHEN ? = '{}' THEN set_json ELSE ? END,
			estimated_json = CASE WHEN ? = '{}' THEN estimated_json ELSE ? END,
			updated_at = ?
		WHERE user_id = ?
	`, setJSON, setJSON, estimatedJSON, estimatedJSON, now, defaultUserID)
	return map[string]any{"updated": true}, nil, err
}

func updateUserContext(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	now := time.Now().Format(time.RFC3339Nano)
	eval := jsonObjectString(args["evaluation"])
	_, err := db.Exec(`
		UPDATE user_contexts SET
			mood = COALESCE(NULLIF(?, ''), mood),
			action = COALESCE(NULLIF(?, ''), action),
			environment = COALESCE(NULLIF(?, ''), environment),
			next_action_prediction = COALESCE(NULLIF(?, ''), next_action_prediction),
			last_prediction = COALESCE(NULLIF(?, ''), last_prediction),
			evaluation_json = CASE WHEN ? = '{}' THEN evaluation_json ELSE ? END,
			updated_at = ?
		WHERE user_id = ?
	`, stringArg(args, "mood"), stringArg(args, "action"), stringArg(args, "environment"), stringArg(args, "next_action_prediction"), stringArg(args, "last_prediction"), eval, eval, now, defaultUserID)
	return map[string]any{"updated": true}, nil, err
}

func updateEnvironmentState(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	now := time.Now().Format(time.RFC3339Nano)
	metadata := jsonObjectString(args["metadata"])
	_, err := db.Exec(`
		UPDATE environment_states SET
			scene = COALESCE(NULLIF(?, ''), scene),
			surroundings = COALESCE(NULLIF(?, ''), surroundings),
			metadata_json = CASE WHEN ? = '{}' THEN metadata_json ELSE ? END,
			updated_at = ?
		WHERE thread_id = ?
	`, stringArg(args, "scene"), stringArg(args, "surroundings"), metadata, metadata, now, defaultThreadID)
	return map[string]any{"updated": true}, nil, err
}

func refreshShortSummary(db *sql.DB) (map[string]any, *Message, error) {
	var maxSeq, count int
	_ = db.QueryRow(`SELECT COALESCE(MAX(seq), 0), COUNT(*) FROM messages WHERE thread_id = ? AND deleted_at IS NULL`, defaultThreadID).Scan(&maxSeq, &count)
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO short_term_summaries(thread_id, content, up_to_seq, source_messages, updated_at)
		VALUES(?, '', ?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET up_to_seq = excluded.up_to_seq, source_messages = excluded.source_messages, updated_at = excluded.updated_at
	`, defaultThreadID, maxSeq, count, now)
	return map[string]any{"queued": true, "up_to_seq": maxSeq}, nil, err
}

func logToolCall(db *sql.DB, messageID string, call ToolCall, result map[string]any, status string) {
	now := time.Now().Format(time.RFC3339Nano)
	resultJSON, _ := json.Marshal(result)
	_, _ = db.Exec(`
		INSERT INTO tool_calls(id, message_id, thread_id, agent_id, name, arguments_json, result_json, status, started_at, completed_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, call.ID, emptyDefault(messageID, "unknown"), defaultThreadID, defaultAgentID, call.Name, emptyDefault(call.Arguments, "{}"), string(resultJSON), status, now, now)
	_, _ = db.Exec(`
		INSERT INTO orchestrator_events(id, thread_id, message_id, function_name, arguments_json, result_json, status, created_at, completed_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newID("evt"), defaultThreadID, messageID, call.Name, emptyDefault(call.Arguments, "{}"), string(resultJSON), status, now, now)
}

func stringArg(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func hasArg(args map[string]any, key string) bool {
	_, ok := args[key]
	return ok
}

func intFromAny(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	}
	return def
}

func jsonObjectString(v any) string {
	if v == nil {
		return "{}"
	}
	data, err := json.Marshal(v)
	if err != nil || string(data) == "null" {
		return "{}"
	}
	return string(data)
}

func stringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		var out []string
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
