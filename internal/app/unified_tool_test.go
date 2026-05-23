package app

import (
	"context"
	"image"
	"image/color"
	"os"
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

func TestComputerToolHelpPermissionsAndContinuationImage(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	oldBackend := currentComputerBackend
	currentComputerBackend = fakeComputerBackend{}
	defer func() { currentComputerBackend = oldBackend }()

	help, _, err := executeComputerTool(context.Background(), db, map[string]any{"operation": "help"})
	if err != nil {
		t.Fatalf("computer help: %v", err)
	}
	if help["enabled"].(bool) {
		t.Fatal("computer use should default to disabled")
	}
	helpResult, _, err := executeSendMsgTool(map[string]any{
		"target":  "ai",
		"kind":    "tool_result",
		"payload": help,
	})
	if err != nil {
		t.Fatal(err)
	}
	helpContinuation, ok := continuationFromToolResult(ToolCall{ID: "call_help", Name: "sendmsg"}, helpResult)
	if !ok || !helpContinuation.Informational {
		t.Fatalf("expected informational computer help continuation, got %#v", helpContinuation)
	}
	if !continuationsAreInformational([]ToolContinuation{helpContinuation}) {
		t.Fatalf("expected help continuation to be non-billable")
	}
	withReminder := addComputerHelpReminder([]ToolContinuation{{Text: "next screenshot result"}})
	if len(withReminder) != 1 || !strings.Contains(withReminder[0].Text, "Computer API guide has already been fetched") {
		t.Fatalf("expected computer help reminder, got %#v", withReminder)
	}
	if _, _, err := executeComputerTool(context.Background(), db, map[string]any{"operation": "observe"}); err == nil {
		t.Fatal("expected observe to be denied while disabled")
	}
	if err := saveAppSetting(db, "runtime_settings", RuntimeSettings{ComputerUseEnabled: true}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}
	observed, _, err := executeComputerTool(context.Background(), db, map[string]any{"operation": "observe"})
	if err != nil {
		t.Fatalf("computer observe: %v", err)
	}
	path, _ = observed["screenshot_path"].(string)
	if path == "" {
		t.Fatalf("missing screenshot path: %#v", observed)
	}
	defer os.Remove(path)
	result, _, err := executeSendMsgTool(map[string]any{
		"target": "ai",
		"kind":   "tool_result",
		"images": []any{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	continuation, ok := continuationFromToolResult(ToolCall{ID: "call_img", Name: "sendmsg"}, result)
	if !ok || len(continuation.Images) != 1 {
		t.Fatalf("expected image continuation, got %#v", continuation)
	}
	input := continuationToAPIInput(continuation)
	parts, _ := input["content"].([]map[string]any)
	foundImage := false
	for _, part := range parts {
		if part["type"] == "input_image" {
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatalf("expected input_image part, got %#v", input)
	}

	windowBackend := &fakeWindowComputerBackend{}
	currentComputerBackend = windowBackend
	windowed, msg, err := executeComputerTool(context.Background(), db, map[string]any{
		"operation":             "observe",
		"crop_to_window":        true,
		"target_app":            "Google Chrome",
		"window_title_contains": "WPS Office for Mac",
	})
	if err != nil {
		t.Fatalf("computer window observe: %v", err)
	}
	windowPath, _ := windowed["screenshot_path"].(string)
	if windowPath == "" {
		t.Fatalf("missing window screenshot path: %#v", windowed)
	}
	defer os.Remove(windowPath)
	if windowed["window"] == nil {
		t.Fatalf("expected window metadata, got %#v", windowed)
	}
	if windowBackend.activated != 1 {
		t.Fatalf("expected window activation before screenshot, got %d", windowBackend.activated)
	}
	if windowBackend.screenshotOpts.Width != 30 || windowBackend.screenshotOpts.Height != 40 {
		t.Fatalf("expected cropped screenshot opts, got %#v", windowBackend.screenshotOpts)
	}
	if msg == nil || !strings.Contains(msg.Text, "window") {
		t.Fatalf("expected user-visible window result message, got %#v", msg)
	}

	changingBackend := &fakeChangingComputerBackend{}
	currentComputerBackend = changingBackend
	acted, _, err := executeComputerTool(context.Background(), db, map[string]any{
		"operation":           "act",
		"actions":             []any{map[string]any{"type": "move", "x": float64(1), "y": float64(1)}},
		"return_screenshot":   true,
		"wait_after_ms":       float64(0),
		"observe_retries":     float64(2),
		"observe_interval_ms": float64(0),
		"wait_until_changed":  true,
	})
	if err != nil {
		t.Fatalf("computer act wait loop: %v", err)
	}
	actPath, _ := acted["screenshot_path"].(string)
	if actPath == "" {
		t.Fatalf("missing act screenshot path: %#v", acted)
	}
	defer os.Remove(actPath)
	observation, _ := acted["observation"].(map[string]any)
	if observation["changed"] != true || observation["attempts"] != 1 {
		t.Fatalf("expected changed observation on first post-action screenshot, got %#v", observation)
	}
	if changingBackend.screenshotCount != 2 || changingBackend.executed != 1 {
		t.Fatalf("expected baseline plus post-action screenshot, got screenshots=%d executed=%d", changingBackend.screenshotCount, changingBackend.executed)
	}

	currentComputerBackend = failingComputerBackend{}
	failed, _, err := executeComputerTool(context.Background(), db, map[string]any{"operation": "observe"})
	if err != nil {
		t.Fatalf("computer observe failures should be model-visible results, got error: %v", err)
	}
	if failed["ok"] != false || failed["diagnosis"] == nil {
		t.Fatalf("expected structured failure result, got %#v", failed)
	}
}

type fakeComputerBackend struct{}

func (fakeComputerBackend) Name() string { return "fake" }

func (fakeComputerBackend) ScreenInfo() (ComputerScreenInfo, error) {
	return ComputerScreenInfo{Width: 12, Height: 8, Scale: 1}, nil
}

func (fakeComputerBackend) Screenshot(context.Context, ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error) {
	return image.NewRGBA(image.Rect(0, 0, 12, 8)), ComputerScreenInfo{Width: 12, Height: 8, Scale: 1}, nil
}

func (fakeComputerBackend) Execute(context.Context, []ComputerAction) error { return nil }

type fakeChangingComputerBackend struct {
	executed        int
	screenshotCount int
}

func (fakeChangingComputerBackend) Name() string { return "fake-changing" }

func (fakeChangingComputerBackend) ScreenInfo() (ComputerScreenInfo, error) {
	return ComputerScreenInfo{Width: 12, Height: 8, Scale: 1}, nil
}

func (b *fakeChangingComputerBackend) Screenshot(context.Context, ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error) {
	b.screenshotCount++
	c := color.Black
	if b.executed > 0 {
		c = color.White
	}
	return solidTestImage(12, 8, c), ComputerScreenInfo{Width: 12, Height: 8, Scale: 1}, nil
}

func (b *fakeChangingComputerBackend) Execute(context.Context, []ComputerAction) error {
	b.executed++
	return nil
}

func solidTestImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

type fakeWindowComputerBackend struct {
	activated      int
	screenshotOpts ComputerScreenshotOptions
}

func (fakeWindowComputerBackend) Name() string { return "fake-window" }

func (fakeWindowComputerBackend) ScreenInfo() (ComputerScreenInfo, error) {
	return ComputerScreenInfo{Width: 100, Height: 80, Scale: 1}, nil
}

func (b *fakeWindowComputerBackend) Screenshot(_ context.Context, opts ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error) {
	b.screenshotOpts = opts
	w, h := opts.Width, opts.Height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 80
	}
	return image.NewRGBA(image.Rect(0, 0, w, h)), ComputerScreenInfo{Width: 100, Height: 80, Scale: 1}, nil
}

func (fakeWindowComputerBackend) LocateWindow(context.Context, ComputerScreenshotOptions) (ComputerWindowInfo, error) {
	return ComputerWindowInfo{App: "Google Chrome", Title: "WPS Office for Mac", X: 10, Y: 20, Width: 30, Height: 40}, nil
}

func (b *fakeWindowComputerBackend) ActivateWindow(context.Context, ComputerScreenshotOptions, ComputerWindowInfo) error {
	b.activated++
	return nil
}

func (fakeWindowComputerBackend) Execute(context.Context, []ComputerAction) error { return nil }

type failingComputerBackend struct{}

func (failingComputerBackend) Name() string { return "failing" }

func (failingComputerBackend) ScreenInfo() (ComputerScreenInfo, error) {
	return ComputerScreenInfo{}, nil
}

func (failingComputerBackend) Screenshot(context.Context, ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error) {
	return nil, ComputerScreenInfo{}, os.ErrPermission
}

func (failingComputerBackend) Execute(context.Context, []ComputerAction) error {
	return os.ErrPermission
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

func TestLegacyLongTermMemoriesMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		CREATE TABLE long_term_memories (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			user_id TEXT,
			content TEXT NOT NULL,
			rank INTEGER NOT NULL DEFAULT 1,
			recall_count INTEGER NOT NULL DEFAULT 0,
			last_recalled_at TEXT,
			source TEXT NOT NULL DEFAULT 'manual',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO long_term_memories(id, agent_id, content, created_at, updated_at) VALUES('legacy_memory', ?, 'legacy content', ?, ?)`, defaultAgentID, now, now); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := initHistoryDB(path); err != nil {
		t.Fatalf("init migrated legacy memory db: %v", err)
	}
	db, err = openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var modelID int
	var category, status string
	if err := db.QueryRow(`SELECT model_id, category, status FROM long_term_memories WHERE id = 'legacy_memory'`).Scan(&modelID, &category, &status); err != nil {
		t.Fatal(err)
	}
	if modelID != 1 || category != "" || status != "active" {
		t.Fatalf("unexpected migrated memory row: modelID=%d category=%q status=%q", modelID, category, status)
	}
	if _, err := executeDueScheduledTools(context.Background(), path, Config{}, 10); err != nil {
		t.Fatalf("scheduler should work after legacy memory migration: %v", err)
	}
}

func TestExecuteDueScheduledToolsInitializesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	messages, err := executeDueScheduledTools(context.Background(), path, Config{}, 10)
	if err != nil {
		t.Fatalf("execute due scheduled tools should initialize schema: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no due messages from seeded future schedules, got %d", len(messages))
	}
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM scheduled_tool_calls`).Scan(&count); err != nil {
		t.Fatalf("scheduled_tool_calls should exist after scheduler scan: %v", err)
	}
	if count == 0 {
		t.Fatal("expected default scheduled tools to be seeded")
	}
}

func TestUISettingsPersistAndEnterPromptContext(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	settings := UISettings{
		User: ProfileSettings{
			FullName:    "张老师",
			Nickname:    "老师",
			Description: "共同研发 Elli",
		},
		Companion: ProfileSettings{
			FullName:    "苏澄",
			Nickname:    "小猫",
			CanonImage:  "character.png",
			Story:       "第一位伴人。",
			Personality: "独立，好奇，共情。",
			Habits:      "每轮对话后评估预测误差。",
		},
		System: Config{BaseURL: "https://example.test/v1", Model: "test-model", APIKey: "local-key"},
	}
	if err := saveUISettings(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadUISettings(path, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.User.FullName != "张老师" || loaded.Companion.Nickname != "小猫" || loaded.System.Model != "test-model" {
		t.Fatalf("unexpected loaded settings: %#v", loaded)
	}
	ctx, err := loadPromptContext(path, defaultPEConfig())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.CompanionProfile.FullName != "苏澄" {
		t.Fatalf("expected companion profile in prompt context, got %#v", ctx.CompanionProfile)
	}
	envelope, err := buildPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(envelope.SystemPrompt, "full_name=苏澄") || !strings.Contains(envelope.SystemPrompt, "canon_image=character.png") || !strings.Contains(envelope.SystemPrompt, "user_set_profile") {
		t.Fatalf("expected saved profiles in system prompt:\n%s", envelope.SystemPrompt)
	}
}

func TestRuntimeSettingsSchedulesAndPEConfig(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	settings := UISettings{
		System: Config{BaseURL: "https://example.test/v1", Model: "test-model"},
		Runtime: RuntimeSettings{
			ContextMessagesK:      24,
			MemoryTopN:            7,
			MemoryRandomM:         2,
			SummarizeThreshold:    55,
			DreamTriggerThreshold: 88,
			DailyMeditateEnabled:  false,
		},
	}
	if err := saveUISettings(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadUISettings(path, Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := peConfigFromRuntime(loaded.Runtime)
	if cfg.RecentMessagesK != 24 || cfg.LongMemoryTopN != 7 || cfg.LongMemoryRandomM != 2 || cfg.SummarizeThreshold != 55 {
		t.Fatalf("unexpected PE config: %#v", cfg)
	}
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var args, status string
	if err := db.QueryRow(`SELECT arguments_json FROM scheduled_tool_calls WHERE id = 'default_dream_hourly'`).Scan(&args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(args, `"threshold":88`) {
		t.Fatalf("expected dream threshold in schedule args, got %s", args)
	}
	if err := db.QueryRow(`SELECT status FROM scheduled_tool_calls WHERE id = 'default_meditate_daily'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "paused" {
		t.Fatalf("expected daily meditate paused, got %q", status)
	}
}

func TestSettingsMemoryKnowledgeEntriesAndWorkflowLogs(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	knowledgeID, err := saveMemoryEntry(path, "knowledge", LongTermMemory{
		Content:    "Elli 的核心指标是 predictive empathy。",
		TagsJSON:   `["architecture","knowledge"]`,
		Rank:       8,
		Confidence: 92,
		Status:     "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if knowledgeID <= 0 {
		t.Fatalf("expected knowledge id, got %d", knowledgeID)
	}
	if _, err := saveMemoryEntry(path, "knowledge", LongTermMemory{
		ModelID:    knowledgeID,
		Content:    "Elli 的核心指标是 control_score。",
		Category:   "knowledge",
		TagsJSON:   `["metric"]`,
		Rank:       9,
		Confidence: 95,
		Status:     "active",
	}); err != nil {
		t.Fatal(err)
	}
	knowledge, err := loadMemoryEntries(path, "knowledge", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(knowledge) != 1 || knowledge[0].ModelID != knowledgeID || !strings.Contains(knowledge[0].Content, "control_score") {
		t.Fatalf("unexpected knowledge entries: %#v", knowledge)
	}
	if err := archiveMemoryEntry(path, knowledgeID); err != nil {
		t.Fatal(err)
	}
	knowledge, err = loadMemoryEntries(path, "knowledge", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(knowledge) != 0 {
		t.Fatalf("expected archived knowledge hidden, got %#v", knowledge)
	}

	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	logWorkflowAudit(db, "dream", map[string]any{"operation": "check"}, map[string]any{"needs_consolidation": false}, "complete")
	logWorkflowAudit(db, "meditate", map[string]any{"operation": "status"}, map[string]any{"status": "ready"}, "complete")
	db.Close()
	logs, err := loadWorkflowLogs(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Name == "" || !strings.Contains(logs[0].Name+logs[1].Name, "dream.workflow") {
		t.Fatalf("unexpected workflow logs: %#v", logs)
	}
}

func TestEvaluateTurnUpdatesStateAndLatency(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	base := time.Now().Add(-10 * time.Minute)
	prevAssistant := Message{Role: "assistant", Text: "上一轮回复", CreatedAt: base}
	if err := saveMessageDB(path, &prevAssistant); err != nil {
		t.Fatal(err)
	}
	user := Message{Role: "user", Text: "用户真实反应", CreatedAt: base.Add(2 * time.Minute)}
	if err := saveMessageDB(path, &user); err != nil {
		t.Fatal(err)
	}
	assistant := Message{Role: "assistant", Text: "本轮回复", CreatedAt: base.Add(3 * time.Minute)}
	if err := saveMessageDB(path, &assistant); err != nil {
		t.Fatal(err)
	}
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	result, _, err := executeEvaluateTurnTool(db, map[string]any{
		"previous_prediction":    map[string]any{"response_type": "continue", "confidence": 70},
		"actual_user_behavior":   map[string]any{"response_type": "continue", "topic": "架构", "mood": "focused", "action": "refine"},
		"prediction_match":       map[string]any{"overall": 82, "topic": 80, "latency": 70},
		"control_score":          82,
		"behavior_effectiveness": 79,
		"short_goal":             map[string]any{"content": "定义自评闭环", "distance": 20, "angle": 85},
		"long_goal":              map[string]any{"content": "形成 hominial 运行时", "distance": 55, "angle": 75},
		"interaction_strategy":   map[string]any{"current": "精确定义机制", "next_move": "落 schema", "avoid": "表面化共情"},
		"next_prediction":        map[string]any{"response_type": "ask_implementation", "reply_latency": map[string]any{"bucket": "fast", "seconds_min": 30, "seconds_max": 240}, "confidence": 76},
	}, assistant.ID)
	if err != nil {
		t.Fatalf("evaluate turn: %v", err)
	}
	if result["reply_latency_seconds"] != 120 {
		t.Fatalf("expected latency 120, got %#v", result["reply_latency_seconds"])
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM turn_evaluations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one turn evaluation, got %d", count)
	}
	roleState, err := loadRoleState(db)
	if err != nil {
		t.Fatal(err)
	}
	if roleState.ControlScore != 82 || roleState.ShortGoalCloseness != 80 || roleState.ShortGoalDeviation != 15 {
		t.Fatalf("unexpected role state: %#v", roleState)
	}
	userContext, err := loadUserContext(db)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(userContext.NextActionPrediction, "ask_implementation") || !strings.Contains(userContext.EvaluationJSON, "reply_latency_seconds") {
		t.Fatalf("unexpected user context: %#v", userContext)
	}
}

func TestDreamSynthesizesDialogueExperienceFromTurnEvaluations(t *testing.T) {
	path, cleanup := testDB(t)
	defer cleanup()
	db, err := openHistoryDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 3; i++ {
		_, _, err := executeEvaluateTurnTool(db, map[string]any{
			"actual_user_behavior":   map[string]any{"response_type": "deepen", "topic": "系统架构"},
			"prediction_match":       map[string]any{"overall": 82},
			"control_score":          80 + i,
			"behavior_effectiveness": 78,
			"short_goal":             map[string]any{"content": "推进设计", "distance": 25, "angle": 80},
			"long_goal":              map[string]any{"content": "形成 hominial", "distance": 60, "angle": 78},
			"interaction_strategy":   map[string]any{"current": "先统一概念，再进入实现细节"},
			"next_prediction":        map[string]any{"response_type": "ask_implementation", "confidence": 75},
		}, "")
		if err != nil {
			t.Fatalf("evaluate %d: %v", i, err)
		}
	}
	result, _, err := executeDreamTool(nil, db, Config{}, map[string]any{"operation": "run", "threshold": float64(100)})
	if err != nil {
		t.Fatal(err)
	}
	applied, _ := result["applied"].(map[string]any)
	if applied["dialogue_experience_memory_id"] == nil {
		t.Fatalf("expected dialogue experience synthesis, got %#v", result)
	}
}
