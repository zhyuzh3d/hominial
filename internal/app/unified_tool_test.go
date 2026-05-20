package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDB(t *testing.T) (string, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chat.db")
	if err := initHistoryDB(path); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return path, func() {}
}

func TestUnifiedMemoryToolAndQuery(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	result, _, err := executeMemoryTool(db, map[string]any{
		"operation":  "upsert",
		"content":    "用户喜欢在晚上讨论产品架构。",
		"category":   "preference",
		"tags":       []any{"产品", "晚上"},
		"rank":       7,
		"confidence": 90,
	})
	if err != nil {
		t.Fatalf("memory upsert: %v", err)
	}
	id, ok := result["id"].(int)
	if !ok || id <= 0 {
		t.Fatalf("bad model id: %#v", result["id"])
	}
	if _, _, err := executeMemoryTool(db, map[string]any{"operation": "mark_used", "ids": []any{float64(id)}}); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	query, _, err := executeQueryTool(db, map[string]any{"source": "memories", "keywords": []any{"产品"}, "limit": float64(5)})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	items, _ := query["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("expected one query item, got %#v", query["items"])
	}
	if items[0]["used_count"].(int) != 1 {
		t.Fatalf("expected used_count 1, got %#v", items[0]["used_count"])
	}
}

func TestDBToolPermissionsAndSendMsg(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, _, err := executeDBTool(db, map[string]any{"operation": "patch", "table": "user_set_profile", "data": map[string]any{"age": 24}}); err == nil {
		t.Fatal("expected user_set_profile write to be denied")
	}
	if _, _, err := executeDBTool(db, map[string]any{"operation": "patch", "table": "user_estimated_profile", "data": map[string]any{"age_guess": 24}}); err != nil {
		t.Fatalf("estimated profile write: %v", err)
	}

	_, msg, err := executeSendMsgTool(map[string]any{"target": "user", "kind": "text", "text": "图片生成好了"})
	if err != nil {
		t.Fatalf("sendmsg: %v", err)
	}
	if msg == nil || msg.Role != "assistant" || !strings.Contains(msg.Text, "图片生成好了") {
		t.Fatalf("unexpected message: %#v", msg)
	}
}

func TestScheduleToolCreateListCancel(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	runAt := time.Now().Add(time.Hour).Format(time.RFC3339Nano)
	result, _, err := executeScheduleTool(db, map[string]any{
		"operation":        "create",
		"name":             "hourly reminder",
		"tool":             "notify",
		"args":             map[string]any{"operation": "send", "text": "休息一下"},
		"run_at":           runAt,
		"interval_seconds": float64(3600),
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	id, _ := result["id"].(string)
	if id == "" {
		t.Fatalf("missing schedule id: %#v", result)
	}
	list, _, err := executeScheduleTool(db, map[string]any{"operation": "list"})
	if err != nil {
		t.Fatalf("list schedule: %v", err)
	}
	items, _ := list["items"].([]map[string]any)
	found := false
	for _, item := range items {
		if item["id"] == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created schedule not found in %#v", list["items"])
	}
	if _, _, err := executeScheduleTool(db, map[string]any{"operation": "cancel", "id": id}); err != nil {
		t.Fatalf("cancel schedule: %v", err)
	}
}

func TestContinuationFromSendMsgResult(t *testing.T) {
	result, msg, err := executeSendMsgTool(map[string]any{
		"target":  "ai",
		"kind":    "tool_result",
		"payload": map[string]any{"items": []any{"memory hit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		t.Fatalf("target=ai should not create a user-visible message: %#v", msg)
	}
	continuation, ok := continuationFromToolResult(ToolCall{ID: "call_1", Name: "sendmsg"}, result)
	if !ok {
		t.Fatal("expected continuation")
	}
	if continuation.SourceCallID != "call_1" || continuation.Payload["items"] == nil {
		t.Fatalf("bad continuation: %#v", continuation)
	}
	input := continuationToAPIInput(continuation)
	if input == nil {
		t.Fatal("expected continuation API input")
	}
}

func TestExecuteDueScheduledTools(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, _, err := executeScheduleTool(db, map[string]any{
		"operation": "create",
		"name":      "due notification",
		"tool":      "notify",
		"args":      map[string]any{"operation": "send", "text": "到时间啦"},
		"run_at":    now,
	}); err != nil {
		t.Fatalf("create due schedule: %v", err)
	}
	messages, err := executeDueScheduledTools(nil, path, Config{}, 10)
	if err != nil {
		t.Fatalf("execute due: %v", err)
	}
	found := false
	for _, msg := range messages {
		if strings.Contains(msg.Text, "到时间啦") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected due notification message, got %#v", messages)
	}
}

func TestDreamWorkflowArchivesExactDuplicates(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 2; i++ {
		if _, _, err := executeMemoryTool(db, map[string]any{
			"operation": "upsert",
			"content":   "用户喜欢讨论 AI 工具架构。",
			"category":  "topic",
			"rank":      float64(4 + i),
		}); err != nil {
			t.Fatalf("upsert duplicate %d: %v", i, err)
		}
	}
	result, _, err := executeDreamTool(nil, db, Config{}, map[string]any{"operation": "run", "threshold": float64(1)})
	if err != nil {
		t.Fatalf("dream run: %v", err)
	}
	applied, _ := result["applied"].(map[string]any)
	archived, _ := applied["deterministic_archive_ids"].([]int)
	if len(archived) != 1 {
		t.Fatalf("expected one duplicate archive, got %#v", result)
	}
}

func TestMeditateNoAPIAuditsWithoutEditing(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	result, _, err := executeMeditateTool(nil, db, Config{}, map[string]any{"operation": "run", "reason": "test"})
	if err != nil {
		t.Fatalf("meditate no api: %v", err)
	}
	if result["status"] != "skipped" {
		t.Fatalf("expected skipped status without API key, got %#v", result)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM orchestrator_events WHERE function_name = 'meditate.workflow'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected meditation workflow audit event")
	}
}

func TestTerminologyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE short_term_summaries (
			thread_id TEXT PRIMARY KEY,
			content TEXT NOT NULL DEFAULT '',
			up_to_seq INTEGER NOT NULL DEFAULT 0,
			source_messages INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO short_term_summaries(thread_id, content, up_to_seq, source_messages, updated_at) VALUES(?, 'legacy', 7, 3, ?)`, defaultThreadID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE scheduled_tool_calls (
			id TEXT PRIMARY KEY,
			owner TEXT NOT NULL DEFAULT 'ai',
			name TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			arguments_json TEXT NOT NULL DEFAULT '{}',
			callback_json TEXT NOT NULL DEFAULT '{}',
			run_at TEXT,
			interval_seconds INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			last_run_at TEXT,
			next_run_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO scheduled_tool_calls(id, owner, name, tool_name, arguments_json, created_at, updated_at) VALUES('default_soul_daily', 'system', 'Daily legacy', 'soul_optimize', '{"operation":"status"}', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := initHistoryDB(path); err != nil {
		t.Fatalf("init migrated db: %v", err)
	}
	db, err = openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var content string
	if err := db.QueryRow(`SELECT content FROM short_term_summarizations WHERE thread_id = ?`, defaultThreadID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if content != "legacy" {
		t.Fatalf("expected migrated summarization content, got %q", content)
	}
	var tool string
	if err := db.QueryRow(`SELECT tool_name FROM scheduled_tool_calls WHERE id = 'default_meditate_daily'`).Scan(&tool); err != nil {
		t.Fatal(err)
	}
	if tool != "meditate" {
		t.Fatalf("expected meditate tool, got %q", tool)
	}
}
