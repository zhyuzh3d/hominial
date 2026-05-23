package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func apiTools() []map[string]any {
	tools := []map[string]any{{"type": "image_generation"}}
	tools = append(tools, modelVisibleFunctionTools()...)
	return tools
}

func modelVisibleFunctionTools() []map[string]any {
	return []map[string]any{
		functionTool("db", "Permissioned database read/write for compact AI-owned state. user_set_profile is read-only; user_estimated_profile, role_state, user_context, and environment_state are writable.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{"type": "string", "enum": []string{"read", "patch", "upsert"}},
				"table":     map[string]any{"type": "string", "enum": []string{"user_set_profile", "user_estimated_profile", "role_state", "user_context", "environment_state"}},
				"data":      map[string]any{"type": "object"},
			},
			"required": []string{"operation", "table"},
		})),
		functionTool("memory", "Manage long-term memories with numeric IDs, categories, tags, scores, and explicit usage marking.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation":  map[string]any{"type": "string", "enum": []string{"upsert", "patch", "mark_used", "score", "archive", "restore"}},
				"id":         map[string]any{"type": "integer"},
				"ids":        map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				"content":    map[string]any{"type": "string"},
				"category":   map[string]any{"type": "string"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"rank":       map[string]any{"type": "integer", "minimum": 0, "maximum": 10},
				"confidence": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"reason":     map[string]any{"type": "string"},
			},
			"required": []string{"operation"},
		})),
		functionTool("query", "Search memories, knowledge, or messages when the current prompt is insufficient. Use callback sendmsg target=ai when you need to continue with results.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":   map[string]any{"type": "string", "enum": []string{"memories", "knowledge", "messages"}},
				"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				"category": map[string]any{"type": "string"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"source", "keywords"},
		})),
		functionTool("evaluate_turn", "Append Elli's predictive empathy self-evaluation for this turn and update real-time strategy, control score, goals, and next prediction.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"previous_prediction": map[string]any{"type": "object"},
				"actual_user_behavior": map[string]any{
					"type":        "object",
					"description": "Observed user behavior. The app fills reply_latency_seconds when it can compute it.",
				},
				"prediction_match":       map[string]any{"type": "object"},
				"control_score":          map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"control_delta":          map[string]any{"type": "integer", "minimum": -100, "maximum": 100},
				"behavior_effectiveness": map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"short_goal": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":        map[string]any{"type": "string"},
						"distance":       map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
						"angle":          map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
						"delta_distance": map[string]any{"type": "integer", "minimum": -100, "maximum": 100},
						"delta_angle":    map[string]any{"type": "integer", "minimum": -100, "maximum": 100},
					},
				},
				"long_goal": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":        map[string]any{"type": "string"},
						"distance":       map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
						"angle":          map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
						"delta_distance": map[string]any{"type": "integer", "minimum": -100, "maximum": 100},
						"delta_angle":    map[string]any{"type": "integer", "minimum": -100, "maximum": 100},
					},
				},
				"interaction_strategy": map[string]any{"type": "object"},
				"next_prediction":      map[string]any{"type": "object"},
				"notes":                map[string]any{"type": "string"},
			},
			"required": []string{"actual_user_behavior", "prediction_match", "short_goal", "long_goal", "interaction_strategy", "next_prediction"},
		})),
		functionTool("sendmsg", "Send a typed message to user, AI continuation log, internal event stream, or notification stream.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":      map[string]any{"type": "string", "enum": []string{"user", "ai", "internal", "notification"}},
				"kind":        map[string]any{"type": "string", "enum": []string{"text", "image", "code", "file", "mixed", "status", "notification", "tool_result"}},
				"text":        map[string]any{"type": "string"},
				"images":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"code":        map[string]any{"type": "string"},
				"language":    map[string]any{"type": "string"},
				"attachments": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"payload":     map[string]any{"type": "object"},
				"metadata":    map[string]any{"type": "object"},
			},
			"required": []string{"target"},
		}),
		functionTool("selfie", "Generate a character image using configured character reference assets and the provided prompt. Usually callback to sendmsg target=user.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string"},
				"style":  map[string]any{"type": "string"},
			},
			"required": []string{"prompt"},
		})),
		functionTool("computer", "High-permission desktop observation and primitive mouse/keyboard control. First call operation=help to fetch the detailed API guide. Use callbacks with sendmsg target=ai when you need to continue from a screenshot result.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{"type": "string", "enum": []string{"help", "observe", "act"}},
				"x":         map[string]any{"type": "integer"},
				"y":         map[string]any{"type": "integer"},
				"width":     map[string]any{"type": "integer", "minimum": 1},
				"height":    map[string]any{"type": "integer", "minimum": 1},
				"crop_to_window": map[string]any{
					"type":        "boolean",
					"description": "For operation=observe or return_screenshot, crop to a matching app window instead of the default fullscreen screenshot.",
				},
				"activate_window": map[string]any{
					"type":        "boolean",
					"description": "When crop_to_window=true, bring the matching window to the foreground before screenshot. Defaults to true.",
				},
				"target_app": map[string]any{
					"type":        "string",
					"description": "Application/process name for crop_to_window, e.g. Google Chrome.",
				},
				"window_title_contains": map[string]any{
					"type":        "string",
					"description": "Case-insensitive window title substring for crop_to_window, e.g. WPS Office for Mac.",
				},
				"return_screenshot": map[string]any{
					"type":        "boolean",
					"description": "For operation=act, capture and return a screenshot after the action sequence. Defaults to true.",
				},
				"wait_after_ms": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     maxComputerWaitMS,
					"description": "For operation=observe or act+return_screenshot, wait before the first screenshot. Defaults to 0 for observe and 800 for act.",
				},
				"observe_retries": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxComputerObserveRetries,
					"description": "Maximum screenshot attempts before returning. Defaults to 1, or 10 when wait_until_changed=true.",
				},
				"observe_interval_ms": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     maxComputerObserveIntervalMS,
					"description": "Milliseconds between screenshot attempts while waiting for a changed screen.",
				},
				"wait_until_changed": map[string]any{
					"type":        "boolean",
					"description": "When true, wait for visual change before returning the screenshot. For act, baseline is captured before actions run.",
				},
				"change_threshold": map[string]any{
					"type":        "number",
					"minimum":     0.001,
					"maximum":     1,
					"description": "Normalized visual difference required by wait_until_changed. Defaults to 0.02.",
				},
				"check_next_ai": map[string]any{
					"type":        "boolean",
					"description": "Use a lightweight visual judge outside the main chat context during long waits.",
				},
				"ai_check_interval_ms": map[string]any{
					"type":        "integer",
					"minimum":     1000,
					"maximum":     maxComputerWaitMS,
					"description": "Minimum interval between checkNext_ai calls. Defaults to 10000.",
				},
				"max_ai_checks": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     10,
					"description": "Maximum checkNext_ai calls for this computer act. Defaults to 3.",
				},
				"wait_goal":        map[string]any{"type": "string"},
				"success_criteria": map[string]any{"type": "string"},
				"blocked_criteria": map[string]any{"type": "string"},
				"last_action":      map[string]any{"type": "string"},
				"actions": map[string]any{
					"type":        "array",
					"maxItems":    maxComputerActions,
					"description": "Primitive actions. Call operation=help for detailed field semantics.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":      map[string]any{"type": "string", "enum": []string{"move", "click", "right_click", "double_click", "key", "hotkey", "key_down", "key_up", "type", "wait", "scroll"}},
							"x":         map[string]any{"type": "integer"},
							"y":         map[string]any{"type": "integer"},
							"button":    map[string]any{"type": "string", "enum": []string{"left", "right", "middle"}},
							"double":    map[string]any{"type": "boolean"},
							"key":       map[string]any{"type": "string"},
							"modifiers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"text":      map[string]any{"type": "string"},
							"ms":        map[string]any{"type": "integer", "minimum": 0, "maximum": 5000},
							"dx":        map[string]any{"type": "integer"},
							"dy":        map[string]any{"type": "integer"},
						},
						"required": []string{"type"},
					},
				},
			},
			"required": []string{"operation"},
		})),
		functionTool("notify", "Create immediate or scheduled user notifications.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation":        map[string]any{"type": "string", "enum": []string{"send", "schedule"}},
				"text":             map[string]any{"type": "string"},
				"run_at":           map[string]any{"type": "string"},
				"interval_seconds": map[string]any{"type": "integer", "minimum": 0},
			},
			"required": []string{"operation", "text"},
		})),
		functionTool("schedule", "Create, list, update, or cancel scheduled tool calls.", withCallback(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation":        map[string]any{"type": "string", "enum": []string{"create", "list", "cancel", "pause", "resume"}},
				"id":               map[string]any{"type": "string"},
				"name":             map[string]any{"type": "string"},
				"tool":             map[string]any{"type": "string"},
				"args":             map[string]any{"type": "object"},
				"tool_callback":    callbackSchema(),
				"run_at":           map[string]any{"type": "string"},
				"interval_seconds": map[string]any{"type": "integer", "minimum": 0},
			},
			"required": []string{"operation"},
		})),
		functionTool("summarize", "Request or run conversation summarization maintenance.", withCallback(map[string]any{
			"type":       "object",
			"properties": map[string]any{"operation": map[string]any{"type": "string", "enum": []string{"refresh", "status"}}, "reason": map[string]any{"type": "string"}},
			"required":   []string{"operation"},
		})),
		functionTool("dream", "Run or schedule lightweight memory consolidation.", withCallback(map[string]any{
			"type":       "object",
			"properties": map[string]any{"operation": map[string]any{"type": "string", "enum": []string{"check", "run", "schedule"}}, "threshold": map[string]any{"type": "integer", "minimum": 1}},
			"required":   []string{"operation"},
		})),
		functionTool("meditate", "Run or schedule the daily multi-step prompt and character meditation workflow. It may only affect allowed prompt/document assets, never code.", withCallback(map[string]any{
			"type":       "object",
			"properties": map[string]any{"operation": map[string]any{"type": "string", "enum": []string{"run", "schedule", "status"}}, "reason": map[string]any{"type": "string"}},
			"required":   []string{"operation"},
		})),
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

func withCallback(parameters map[string]any) map[string]any {
	props, _ := parameters["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		parameters["properties"] = props
	}
	props["callback"] = callbackSchema()
	return parameters
}

func callbackSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool": map[string]any{"type": "string"},
			"args": map[string]any{"type": "object"},
		},
		"required": []string{"tool"},
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

func runOrchestrator(ctx context.Context, path string, cfg Config, messageID string, calls []ToolCall) (OrchestratorResult, error) {
	if len(calls) == 0 {
		return OrchestratorResult{}, nil
	}
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return OrchestratorResult{}, err
	}
	defer db.Close()
	var out OrchestratorResult
	var firstErr error
	for _, call := range calls {
		result, msg, err := executeToolCall(ctx, db, cfg, call, messageID)
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
			out.Messages = append(out.Messages, *msg)
		}
		if continuation, ok := continuationFromToolResult(call, result); ok {
			out.Continuations = append(out.Continuations, continuation)
		}
		if status == "complete" {
			callback, cbOK := callbackFromArguments(call.Arguments)
			if cbOK && strings.TrimSpace(callback.Tool) != "" {
				cbCall := callbackToolCall(call, callback, result)
				cbResult, cbMsg, cbErr := executeToolCall(ctx, db, cfg, cbCall, messageID)
				cbStatus := "complete"
				if cbErr != nil {
					cbStatus = "failed"
					cbResult = map[string]any{"error": cbErr.Error()}
					if firstErr == nil {
						firstErr = fmt.Errorf("%s callback %s: %w", call.Name, cbCall.Name, cbErr)
					}
				}
				logToolCall(db, messageID, cbCall, cbResult, cbStatus)
				if cbMsg != nil {
					out.Messages = append(out.Messages, *cbMsg)
				}
				if continuation, ok := continuationFromToolResult(cbCall, cbResult); ok {
					out.Continuations = append(out.Continuations, continuation)
				}
			}
		}
	}
	return out, firstErr
}

func executeToolCall(ctx context.Context, db *sql.DB, cfg Config, call ToolCall, messageID string) (map[string]any, *Message, error) {
	args := map[string]any{}
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return nil, nil, err
		}
	}
	delete(args, "callback")
	switch call.Name {
	case "db":
		return executeDBTool(db, args)
	case "memory":
		return executeMemoryTool(db, args)
	case "query":
		return executeQueryTool(db, args)
	case "evaluate_turn":
		return executeEvaluateTurnTool(db, args, messageID)
	case "sendmsg":
		return executeSendMsgTool(args)
	case "selfie":
		return executeSelfieTool(ctx, db, cfg, args)
	case "computer":
		return executeComputerTool(ctx, db, cfg, args, call.ID, messageID)
	case "notify":
		return executeNotifyTool(db, args)
	case "schedule":
		return executeScheduleTool(db, args)
	case "summarize", "summary":
		return executeSummarizeTool(db, args)
	case "dream":
		return executeDreamTool(ctx, db, cfg, args)
	case "meditate", "soul_optimize":
		return executeMeditateTool(ctx, db, cfg, args)
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
	case "request_summarize_refresh", "request_summary_refresh":
		return refreshShortSummarization(db)
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

func executeDBTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	op := stringArg(args, "operation")
	table := stringArg(args, "table")
	data, _ := args["data"].(map[string]any)
	switch op {
	case "read":
		switch table {
		case "user_set_profile", "user_estimated_profile":
			p, err := loadUserProfile(db)
			if err != nil {
				return nil, nil, err
			}
			if table == "user_set_profile" {
				return map[string]any{"table": table, "data": jsonRawObject(p.SetJSON)}, nil, nil
			}
			return map[string]any{"table": table, "data": jsonRawObject(p.EstimatedJSON)}, nil, nil
		case "role_state":
			s, err := loadRoleState(db)
			if err != nil {
				return nil, nil, err
			}
			return map[string]any{"table": table, "data": s}, nil, nil
		case "user_context":
			c, err := loadUserContext(db)
			if err != nil {
				return nil, nil, err
			}
			return map[string]any{"table": table, "data": c}, nil, nil
		case "environment_state":
			e, err := loadEnvironmentState(db)
			if err != nil {
				return nil, nil, err
			}
			return map[string]any{"table": table, "data": e}, nil, nil
		default:
			return nil, nil, fmt.Errorf("db read denied for table %q", table)
		}
	case "patch", "upsert":
		switch table {
		case "user_set_profile":
			return nil, nil, fmt.Errorf("user_set_profile is read-only for AI tools")
		case "user_estimated_profile":
			return updateUserProfile(db, map[string]any{"estimated": data})
		case "role_state":
			return updateRoleState(db, data)
		case "user_context":
			return updateUserContext(db, data)
		case "environment_state":
			return updateEnvironmentState(db, data)
		default:
			return nil, nil, fmt.Errorf("db write denied for table %q", table)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported db operation %q", op)
	}
}

func executeMemoryTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	switch stringArg(args, "operation") {
	case "upsert":
		return memoryUpsert(db, args)
	case "patch":
		return memoryPatch(db, args)
	case "mark_used":
		return memoryMarkUsed(db, args)
	case "score":
		return memoryScore(db, args)
	case "archive":
		return memoryStatus(db, args, "archived")
	case "restore":
		return memoryStatus(db, args, "active")
	default:
		return nil, nil, fmt.Errorf("unsupported memory operation %q", stringArg(args, "operation"))
	}
}

func memoryUpsert(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	modelID := intIDFromAny(args["id"])
	id := ""
	if modelID > 0 {
		_ = db.QueryRow(`SELECT id FROM long_term_memories WHERE model_id = ?`, modelID).Scan(&id)
	} else {
		var err error
		modelID, err = nextMemoryModelID(db)
		if err != nil {
			return nil, nil, err
		}
	}
	if id == "" {
		id = newID("mem")
	}
	now := time.Now().Format(time.RFC3339Nano)
	tagsJSON := jsonArrayString(args["tags"])
	rank := intFromAny(args["rank"], 3)
	confidence := intFromAny(args["confidence"], 70)
	_, err := db.Exec(`
		INSERT INTO long_term_memories(id, model_id, agent_id, user_id, content, category, tags_json, rank, confidence, source, status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'function', 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			category = excluded.category,
			tags_json = excluded.tags_json,
			rank = excluded.rank,
			confidence = excluded.confidence,
			status = 'active',
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`, id, modelID, defaultAgentID, defaultUserID, content, stringArg(args, "category"), tagsJSON, rank, confidence, now, now)
	return map[string]any{"id": modelID, "internal_id": id}, nil, err
}

func memoryPatch(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	modelID := intIDFromAny(args["id"])
	if modelID <= 0 {
		return nil, nil, fmt.Errorf("id is required")
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		UPDATE long_term_memories SET
			content = COALESCE(NULLIF(?, ''), content),
			category = COALESCE(NULLIF(?, ''), category),
			tags_json = CASE WHEN ? = '[]' THEN tags_json ELSE ? END,
			rank = CASE WHEN ? THEN ? ELSE rank END,
			confidence = CASE WHEN ? THEN ? ELSE confidence END,
			updated_at = ?
		WHERE model_id = ? AND agent_id = ?
	`, stringArg(args, "content"), stringArg(args, "category"), jsonArrayString(args["tags"]), jsonArrayString(args["tags"]),
		hasArg(args, "rank"), intFromAny(args["rank"], 3),
		hasArg(args, "confidence"), intFromAny(args["confidence"], 70),
		now, modelID, defaultAgentID)
	return map[string]any{"id": modelID, "updated": true}, nil, err
}

func memoryMarkUsed(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	ids := intIDsFromArgs(args)
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("id or ids is required")
	}
	now := time.Now().Format(time.RFC3339Nano)
	for _, id := range ids {
		if _, err := db.Exec(`UPDATE long_term_memories SET used_count = used_count + 1, last_used_at = ?, updated_at = ? WHERE model_id = ? AND agent_id = ?`, now, now, id, defaultAgentID); err != nil {
			return nil, nil, err
		}
	}
	return map[string]any{"ids": ids, "marked_used": true}, nil, nil
}

func memoryScore(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	modelID := intIDFromAny(args["id"])
	if modelID <= 0 {
		return nil, nil, fmt.Errorf("id is required")
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		UPDATE long_term_memories SET
			rank = CASE WHEN ? THEN ? ELSE rank END,
			confidence = CASE WHEN ? THEN ? ELSE confidence END,
			updated_at = ?
		WHERE model_id = ? AND agent_id = ?
	`, hasArg(args, "rank"), intFromAny(args["rank"], 3), hasArg(args, "confidence"), intFromAny(args["confidence"], 70), now, modelID, defaultAgentID)
	return map[string]any{"id": modelID, "updated": true}, nil, err
}

func memoryStatus(db *sql.DB, args map[string]any, status string) (map[string]any, *Message, error) {
	ids := intIDsFromArgs(args)
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("id or ids is required")
	}
	now := time.Now().Format(time.RFC3339Nano)
	for _, id := range ids {
		if _, err := db.Exec(`UPDATE long_term_memories SET status = ?, updated_at = ? WHERE model_id = ? AND agent_id = ?`, status, now, id, defaultAgentID); err != nil {
			return nil, nil, err
		}
	}
	return map[string]any{"ids": ids, "status": status}, nil, nil
}

func executeQueryTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	limit := intFromAny(args["limit"], 5)
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	keywords := stringSlice(args["keywords"])
	if len(keywords) == 0 {
		return nil, nil, fmt.Errorf("keywords are required")
	}
	q := "%" + strings.Join(keywords, "%") + "%"
	switch stringArg(args, "source") {
	case "memories", "knowledge":
		rows, err := db.Query(`
			SELECT model_id, content, category, tags_json, rank, confidence, used_count
			FROM long_term_memories
			WHERE agent_id = ? AND deleted_at IS NULL AND status = 'active' AND content LIKE ?
			ORDER BY rank DESC, used_count DESC, updated_at DESC
			LIMIT ?
		`, defaultAgentID, q, limit)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		var items []map[string]any
		for rows.Next() {
			var modelID, rank, confidence, usedCount int
			var content, category, tagsJSON string
			if err := rows.Scan(&modelID, &content, &category, &tagsJSON, &rank, &confidence, &usedCount); err != nil {
				return nil, nil, err
			}
			items = append(items, map[string]any{"id": modelID, "content": content, "category": category, "tags": jsonRawArray(tagsJSON), "rank": rank, "confidence": confidence, "used_count": usedCount})
		}
		return map[string]any{"source": stringArg(args, "source"), "items": items}, nil, rows.Err()
	case "messages":
		rows, err := db.Query(`
			SELECT seq, role, content, created_at
			FROM messages
			WHERE thread_id = ? AND deleted_at IS NULL AND content LIKE ?
			ORDER BY seq DESC
			LIMIT ?
		`, defaultThreadID, q, limit)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		var items []map[string]any
		for rows.Next() {
			var seq int
			var role, content, created string
			if err := rows.Scan(&seq, &role, &content, &created); err != nil {
				return nil, nil, err
			}
			items = append(items, map[string]any{"seq": seq, "role": role, "content": content, "created_at": created})
		}
		return map[string]any{"source": "messages", "items": items}, nil, rows.Err()
	default:
		return nil, nil, fmt.Errorf("unsupported query source %q", stringArg(args, "source"))
	}
}

func executeEvaluateTurnTool(db *sql.DB, args map[string]any, assistantMessageID string) (map[string]any, *Message, error) {
	now := time.Now().Format(time.RFC3339Nano)
	roleState, err := loadRoleState(db)
	if err != nil {
		return nil, nil, err
	}
	userContext, err := loadUserContext(db)
	if err != nil {
		return nil, nil, err
	}
	previousPrediction := objectArg(args, "previous_prediction")
	if len(previousPrediction) == 0 {
		previousPrediction = jsonRawObject(userContext.NextActionPrediction)
	}
	actualBehavior := objectArg(args, "actual_user_behavior")
	if latency, ok := computedReplyLatencySeconds(db, assistantMessageID); ok {
		actualBehavior["reply_latency_seconds"] = latency
	}
	predictionMatch := objectArg(args, "prediction_match")
	shortGoal := objectArg(args, "short_goal")
	longGoal := objectArg(args, "long_goal")
	interactionStrategy := objectArg(args, "interaction_strategy")
	nextPrediction := objectArg(args, "next_prediction")

	controlScore := roleState.ControlScore
	if hasArg(args, "control_score") {
		controlScore = clampInt(intFromAny(args["control_score"], roleState.ControlScore), 0, 100, roleState.ControlScore)
	} else if hasArg(args, "control_delta") {
		controlScore = clampInt(roleState.ControlScore+intFromAny(args["control_delta"], 0), 0, 100, roleState.ControlScore)
	} else if overall, ok := intFromObject(predictionMatch, "overall"); ok {
		controlScore = clampInt((roleState.ControlScore+overall)/2, 0, 100, roleState.ControlScore)
	}
	effectiveness := clampInt(intFromAny(args["behavior_effectiveness"], roleState.BehaviorEffectiveness), 0, 100, roleState.BehaviorEffectiveness)

	seq := 0
	if strings.TrimSpace(assistantMessageID) != "" {
		_ = db.QueryRow(`SELECT COALESCE(seq, 0) FROM messages WHERE id = ?`, assistantMessageID).Scan(&seq)
	}
	id := newID("eval")
	prevJSON := jsonObjectString(previousPrediction)
	actualJSON := jsonObjectString(actualBehavior)
	matchJSON := jsonObjectString(predictionMatch)
	shortJSON := jsonObjectString(shortGoal)
	longJSON := jsonObjectString(longGoal)
	strategyJSON := jsonObjectString(interactionStrategy)
	nextJSON := jsonObjectString(nextPrediction)
	rawJSON := jsonObjectString(args)

	if _, err := db.Exec(`
		INSERT INTO turn_evaluations(id, thread_id, assistant_message_id, seq, previous_prediction_json, actual_behavior_json,
			prediction_match_json, control_score, behavior_effectiveness, short_goal_json, long_goal_json,
			interaction_strategy_json, next_prediction_json, raw_json, created_at)
		VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, defaultThreadID, assistantMessageID, seq, prevJSON, actualJSON, matchJSON, controlScore, effectiveness, shortJSON, longJSON, strategyJSON, nextJSON, rawJSON, now); err != nil {
		return nil, nil, err
	}

	roleMetadata := jsonRawObject(roleState.MetadataJSON)
	roleMetadata["current_short_goal"] = shortGoal
	roleMetadata["current_long_goal"] = longGoal
	roleMetadata["interaction_strategy"] = interactionStrategy
	roleMetadata["last_turn_evaluation_id"] = id
	roleUpdate := map[string]any{
		"control_score":          controlScore,
		"behavior_effectiveness": effectiveness,
		"metadata":               roleMetadata,
	}
	if distance, ok := intFromObject(shortGoal, "distance"); ok {
		roleUpdate["short_goal_closeness"] = clampInt(100-distance, 0, 100, 50)
	}
	if angle, ok := intFromObject(shortGoal, "angle"); ok {
		roleUpdate["short_goal_deviation"] = clampInt(100-angle, 0, 100, 0)
	}
	if distance, ok := intFromObject(longGoal, "distance"); ok {
		roleUpdate["long_goal_closeness"] = clampInt(100-distance, 0, 100, 50)
	}
	if angle, ok := intFromObject(longGoal, "angle"); ok {
		roleUpdate["long_goal_deviation"] = clampInt(100-angle, 0, 100, 0)
	}
	if _, _, err := updateRoleState(db, roleUpdate); err != nil {
		return nil, nil, err
	}

	evaluation := map[string]any{
		"id":                     id,
		"previous_prediction":    previousPrediction,
		"actual_user_behavior":   actualBehavior,
		"prediction_match":       predictionMatch,
		"short_goal":             shortGoal,
		"long_goal":              longGoal,
		"interaction_strategy":   interactionStrategy,
		"next_prediction":        nextPrediction,
		"control_score":          controlScore,
		"behavior_effectiveness": effectiveness,
	}
	userUpdate := map[string]any{
		"last_prediction":        prevJSON,
		"next_action_prediction": nextJSON,
		"evaluation":             evaluation,
	}
	if mood, ok := actualBehavior["mood"].(string); ok {
		userUpdate["mood"] = mood
	}
	if action, ok := actualBehavior["action"].(string); ok {
		userUpdate["action"] = action
	}
	if _, _, err := updateUserContext(db, userUpdate); err != nil {
		return nil, nil, err
	}
	return map[string]any{
		"id":                     id,
		"seq":                    seq,
		"control_score":          controlScore,
		"behavior_effectiveness": effectiveness,
		"reply_latency_seconds":  actualBehavior["reply_latency_seconds"],
	}, nil, nil
}

func computedReplyLatencySeconds(db *sql.DB, assistantMessageID string) (int, bool) {
	if strings.TrimSpace(assistantMessageID) == "" {
		return 0, false
	}
	var assistantSeq int
	if err := db.QueryRow(`SELECT COALESCE(seq, 0) FROM messages WHERE id = ?`, assistantMessageID).Scan(&assistantSeq); err != nil || assistantSeq <= 0 {
		return 0, false
	}
	var userSeq int
	var userCreated string
	if err := db.QueryRow(`
		SELECT COALESCE(seq, 0), created_at
		FROM messages
		WHERE thread_id = ? AND role = 'user' AND COALESCE(seq, 0) < ?
		ORDER BY COALESCE(seq, 0) DESC
		LIMIT 1
	`, defaultThreadID, assistantSeq).Scan(&userSeq, &userCreated); err != nil || userSeq <= 0 {
		return 0, false
	}
	var prevAssistantCreated string
	if err := db.QueryRow(`
		SELECT created_at
		FROM messages
		WHERE thread_id = ? AND role = 'assistant' AND COALESCE(seq, 0) < ?
		ORDER BY COALESCE(seq, 0) DESC
		LIMIT 1
	`, defaultThreadID, userSeq).Scan(&prevAssistantCreated); err != nil {
		return 0, false
	}
	userTime, err := time.Parse(time.RFC3339Nano, userCreated)
	if err != nil {
		return 0, false
	}
	prevAssistantTime, err := time.Parse(time.RFC3339Nano, prevAssistantCreated)
	if err != nil {
		return 0, false
	}
	seconds := int(userTime.Sub(prevAssistantTime).Seconds())
	if seconds < 0 {
		return 0, false
	}
	return seconds, true
}

func executeSendMsgTool(args map[string]any) (map[string]any, *Message, error) {
	target := emptyDefault(stringArg(args, "target"), "internal")
	kind := emptyDefault(stringArg(args, "kind"), "text")
	text := stringArg(args, "text")
	if text == "" {
		if payload, ok := args["payload"]; ok {
			raw, _ := json.MarshalIndent(payload, "", "  ")
			text = string(raw)
		}
	}
	images := stringSlice(args["images"])
	attachments := stringSlice(args["attachments"])
	result := map[string]any{"target": target, "kind": kind, "text": text, "images": images, "attachments": attachments}
	if payload, ok := args["payload"].(map[string]any); ok {
		result["payload"] = payload
	}
	if target != "user" {
		return result, nil, nil
	}
	return result, &Message{Role: "assistant", Text: text, Images: images, Attachments: attachments, CreatedAt: time.Now()}, nil
}

func executeSelfieTool(ctx context.Context, db *sql.DB, cfg Config, args map[string]any) (map[string]any, *Message, error) {
	prompt := strings.TrimSpace(stringArg(args, "prompt"))
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}
	refs := stringSlice(args["reference_paths"])
	if len(refs) == 0 {
		refs = defaultSelfieReferences(db)
	}
	if len(refs) == 0 {
		return map[string]any{"prompt": prompt, "warning": "no reference image configured"}, nil, nil
	}
	msg, err := callReferenceImage(ctx, cfg, prompt, refs)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{"prompt": prompt, "generated_images": msg.Images}, nil, nil
}

func executeNotifyTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	if stringArg(args, "operation") == "send" {
		text := stringArg(args, "text")
		if strings.TrimSpace(text) == "" {
			return nil, nil, fmt.Errorf("text is required")
		}
		return map[string]any{"sent": true, "text": text}, &Message{Role: "assistant", Text: text, CreatedAt: time.Now()}, nil
	}
	scheduleArgs := map[string]any{
		"operation":        "create",
		"name":             "AI notification",
		"tool":             "notify",
		"args":             map[string]any{"operation": "send", "text": stringArg(args, "text")},
		"run_at":           stringArg(args, "run_at"),
		"interval_seconds": intFromAny(args["interval_seconds"], 0),
	}
	return executeScheduleTool(db, scheduleArgs)
}

func executeScheduleTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	switch stringArg(args, "operation") {
	case "create":
		id := newID("sched")
		now := time.Now().Format(time.RFC3339Nano)
		toolName := stringArg(args, "tool")
		if toolName == "" {
			return nil, nil, fmt.Errorf("tool is required")
		}
		if toolName == "computer" {
			return nil, nil, fmt.Errorf("computer tool calls cannot be scheduled")
		}
		toolArgs, _ := json.Marshal(args["args"])
		toolCallback, _ := json.Marshal(args["tool_callback"])
		_, err := db.Exec(`
			INSERT INTO scheduled_tool_calls(id, owner, name, tool_name, arguments_json, callback_json, run_at, interval_seconds, status, next_run_at, created_at, updated_at)
			VALUES(?, 'ai', ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)
		`, id, emptyDefault(stringArg(args, "name"), toolName), toolName, emptyDefault(string(toolArgs), "{}"), stringOrDefault(string(toolCallback), "{}"), stringArg(args, "run_at"), intFromAny(args["interval_seconds"], 0), stringArg(args, "run_at"), now, now)
		return map[string]any{"id": id, "scheduled": true}, nil, err
	case "list":
		rows, err := db.Query(`SELECT id, name, tool_name, run_at, interval_seconds, status FROM scheduled_tool_calls ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		var items []map[string]any
		for rows.Next() {
			var id, name, toolName, runAt, status string
			var interval int
			if err := rows.Scan(&id, &name, &toolName, &runAt, &interval, &status); err != nil {
				return nil, nil, err
			}
			items = append(items, map[string]any{"id": id, "name": name, "tool": toolName, "run_at": runAt, "interval_seconds": interval, "status": status})
		}
		return map[string]any{"items": items}, nil, rows.Err()
	case "cancel", "pause", "resume":
		id := stringArg(args, "id")
		if id == "" {
			return nil, nil, fmt.Errorf("id is required")
		}
		status := map[string]string{"cancel": "cancelled", "pause": "paused", "resume": "active"}[stringArg(args, "operation")]
		_, err := db.Exec(`UPDATE scheduled_tool_calls SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Format(time.RFC3339Nano), id)
		return map[string]any{"id": id, "status": status}, nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported schedule operation %q", stringArg(args, "operation"))
	}
}

func executeSummarizeTool(db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	if stringArg(args, "operation") == "status" {
		s, err := loadShortTermSummarization(db, defaultThreadID)
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"up_to_seq": s.UpToSeq, "source_messages": s.SourceMessages, "updated_at": s.UpdatedAt}, nil, nil
	}
	return refreshShortSummarization(db)
}

func executeDreamTool(ctx context.Context, db *sql.DB, cfg Config, args map[string]any) (map[string]any, *Message, error) {
	result, err := runDreamWorkflow(ctx, db, cfg, args)
	return result, nil, err
}

func executeMeditateTool(ctx context.Context, db *sql.DB, cfg Config, args map[string]any) (map[string]any, *Message, error) {
	result, err := runMeditateWorkflow(ctx, db, cfg, args)
	return result, nil, err
}

func callbackFromArguments(arguments string) (ToolCallback, bool) {
	if strings.TrimSpace(arguments) == "" {
		return ToolCallback{}, false
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return ToolCallback{}, false
	}
	raw, ok := args["callback"]
	if !ok || raw == nil {
		return ToolCallback{}, false
	}
	data, _ := json.Marshal(raw)
	var callback ToolCallback
	if err := json.Unmarshal(data, &callback); err != nil {
		return ToolCallback{}, false
	}
	return callback, callback.Tool != ""
}

func callbackToolCall(parent ToolCall, callback ToolCallback, result map[string]any) ToolCall {
	args := map[string]any{}
	for k, v := range callback.Args {
		args[k] = v
	}
	if _, ok := args["payload"]; !ok {
		args["payload"] = result
	}
	if callback.Tool == "sendmsg" {
		if _, ok := args["images"]; !ok {
			if imgs := stringSlice(result["generated_images"]); len(imgs) > 0 {
				args["images"] = imgs
				if _, hasKind := args["kind"]; !hasKind {
					args["kind"] = "image"
				}
			} else if imgs := stringSlice(result["images"]); len(imgs) > 0 {
				args["images"] = imgs
				if _, hasKind := args["kind"]; !hasKind {
					args["kind"] = "image"
				}
			} else if screenshot, _ := result["screenshot_path"].(string); strings.TrimSpace(screenshot) != "" {
				args["images"] = []string{screenshot}
				if _, hasKind := args["kind"]; !hasKind {
					args["kind"] = "image"
				}
			}
		}
	}
	data, _ := json.Marshal(args)
	return ToolCall{
		ID:        newID("cb"),
		Name:      callback.Tool,
		Arguments: string(data),
		Status:    "pending",
	}
}

func continuationFromToolResult(call ToolCall, result map[string]any) (ToolContinuation, bool) {
	if call.Name != "sendmsg" || result == nil {
		return ToolContinuation{}, false
	}
	target, _ := result["target"].(string)
	if target != "ai" {
		return ToolContinuation{}, false
	}
	text, _ := result["text"].(string)
	images := stringSlice(result["images"])
	payload, _ := result["payload"].(map[string]any)
	if strings.TrimSpace(text) == "" && len(payload) == 0 && len(images) == 0 {
		return ToolContinuation{}, false
	}
	return ToolContinuation{
		SourceCallID:  call.ID,
		Text:          text,
		Images:        images,
		Payload:       payload,
		Informational: isInformationalToolContinuation(payload),
	}, true
}

func isInformationalToolContinuation(payload map[string]any) bool {
	return isComputerHelpPayload(payload)
}

func isComputerHelpPayload(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	tool, _ := payload["tool"].(string)
	if tool == "computer" && payload["operations"] != nil && payload["actions"] != nil {
		return true
	}
	return false
}

func intIDFromAny(v any) int {
	if s, ok := v.(string); ok {
		s = strings.TrimPrefix(strings.TrimSpace(s), "M")
		var id int
		_, _ = fmt.Sscanf(s, "%d", &id)
		return id
	}
	return intFromAny(v, 0)
}

func intIDsFromArgs(args map[string]any) []int {
	seen := map[int]bool{}
	var ids []int
	add := func(id int) {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(intIDFromAny(args["id"]))
	switch values := args["ids"].(type) {
	case []any:
		for _, v := range values {
			add(intIDFromAny(v))
		}
	case []int:
		for _, v := range values {
			add(v)
		}
	}
	return ids
}

func jsonArrayString(v any) string {
	if v == nil {
		return "[]"
	}
	data, err := json.Marshal(v)
	if err != nil || string(data) == "null" {
		return "[]"
	}
	return string(data)
}

func jsonRawObject(src string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(emptyDefault(src, "{}")), &out)
	return out
}

func jsonRawArray(src string) []any {
	var out []any
	_ = json.Unmarshal([]byte(emptyDefault(src, "[]")), &out)
	return out
}

func objectArg(args map[string]any, key string) map[string]any {
	if value, ok := args[key].(map[string]any); ok {
		return value
	}
	if value, ok := args[key]; ok {
		raw, _ := json.Marshal(value)
		out := map[string]any{}
		if json.Unmarshal(raw, &out) == nil {
			return out
		}
	}
	return map[string]any{}
}

func intFromObject(obj map[string]any, key string) (int, bool) {
	value, ok := obj[key]
	if !ok {
		return 0, false
	}
	return intFromAny(value, 0), true
}

func defaultSelfieReferences(db *sql.DB) []string {
	if db != nil {
		if canon := strings.TrimSpace(loadCompanionProfile(db).CanonImage); canon != "" {
			return []string{canon}
		}
	}
	candidates := []string{
		"character.png",
		"character.jpg",
		"app_outputs/prepared",
		"outputs",
	}
	var refs []string
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			refs = append(refs, candidate)
			continue
		}
		_ = filepath.WalkDir(candidate, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || len(refs) >= 3 {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
				refs = append(refs, path)
			}
			return nil
		})
		if len(refs) > 0 {
			break
		}
	}
	return refs
}

func executeDueScheduledTools(ctx context.Context, path string, cfg Config, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 10
	}
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := time.Now()
	rows, err := db.Query(`
		SELECT id, tool_name, arguments_json, callback_json, interval_seconds
		FROM scheduled_tool_calls
		WHERE status = 'active'
			AND COALESCE(next_run_at, run_at, '') != ''
			AND COALESCE(next_run_at, run_at) <= ?
		ORDER BY COALESCE(next_run_at, run_at), created_at
		LIMIT ?
	`, now.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type dueCall struct {
		id       string
		tool     string
		args     string
		callback string
		interval int
	}
	var due []dueCall
	for rows.Next() {
		var call dueCall
		if err := rows.Scan(&call.id, &call.tool, &call.args, &call.callback, &call.interval); err != nil {
			return nil, err
		}
		due = append(due, call)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var messages []Message
	for _, item := range due {
		arguments := mergeScheduledCallback(item.args, item.callback)
		call := ToolCall{ID: newID("schedcall"), Name: item.tool, Arguments: arguments, Status: "pending"}
		if item.tool == "computer" {
			logToolCall(db, "schedule:"+item.id, call, map[string]any{"error": "computer tool calls cannot be scheduled"}, "failed")
			_, _ = db.Exec(`UPDATE scheduled_tool_calls SET status = ?, last_run_at = ?, updated_at = ? WHERE id = ?`, "paused", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), item.id)
			continue
		}
		result, msg, err := executeToolCall(ctx, db, cfg, call, "schedule:"+item.id)
		status := "complete"
		if err != nil {
			status = "failed"
			result = map[string]any{"error": err.Error()}
		}
		logToolCall(db, "schedule:"+item.id, call, result, status)
		if msg != nil {
			messages = append(messages, *msg)
		}
		if status == "complete" {
			if callback, ok := callbackFromArguments(call.Arguments); ok && strings.TrimSpace(callback.Tool) != "" {
				cbCall := callbackToolCall(call, callback, result)
				cbResult, cbMsg, cbErr := executeToolCall(ctx, db, cfg, cbCall, "schedule:"+item.id)
				cbStatus := "complete"
				if cbErr != nil {
					cbStatus = "failed"
					cbResult = map[string]any{"error": cbErr.Error()}
				}
				logToolCall(db, "schedule:"+item.id, cbCall, cbResult, cbStatus)
				if cbMsg != nil {
					messages = append(messages, *cbMsg)
				}
			}
		}
		nextStatus := "completed"
		nextRun := ""
		if item.interval > 0 {
			nextStatus = "active"
			nextRun = now.Add(time.Duration(item.interval) * time.Second).Format(time.RFC3339Nano)
		}
		_, _ = db.Exec(`UPDATE scheduled_tool_calls SET status = ?, last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?`, nextStatus, now.Format(time.RFC3339Nano), nextRun, now.Format(time.RFC3339Nano), item.id)
	}
	return messages, nil
}

func mergeScheduledCallback(argsJSON, callbackJSON string) string {
	args := map[string]any{}
	_ = json.Unmarshal([]byte(emptyDefault(argsJSON, "{}")), &args)
	callbackJSON = strings.TrimSpace(callbackJSON)
	if callbackJSON != "" && callbackJSON != "{}" && callbackJSON != "null" {
		var callback map[string]any
		if json.Unmarshal([]byte(callbackJSON), &callback) == nil && len(callback) > 0 {
			args["callback"] = callback
		}
	}
	raw, _ := json.Marshal(args)
	return string(raw)
}

func stringOrDefault(s, def string) string {
	if strings.TrimSpace(s) == "" || strings.TrimSpace(s) == "null" {
		return def
	}
	return s
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

func refreshShortSummarization(db *sql.DB) (map[string]any, *Message, error) {
	var maxSeq, count int
	_ = db.QueryRow(`SELECT COALESCE(MAX(seq), 0), COUNT(*) FROM messages WHERE thread_id = ? AND deleted_at IS NULL`, defaultThreadID).Scan(&maxSeq, &count)
	now := time.Now().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO short_term_summarizations(thread_id, content, up_to_seq, source_messages, updated_at)
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

func floatFromAny(v any, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			return f
		}
	}
	return def
}

func clampFloat(v, min, max, def float64) float64 {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
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
