package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type workflowMemory struct {
	ModelID    int      `json:"id"`
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Rank       int      `json:"rank"`
	Confidence int      `json:"confidence"`
	Recalled   int      `json:"recalled_count"`
	Used       int      `json:"used_count"`
	UpdatedAt  string   `json:"updated_at"`
}

type dreamPlan struct {
	NewMemories []struct {
		Content    string   `json:"content"`
		Category   string   `json:"category"`
		Tags       []string `json:"tags"`
		Rank       int      `json:"rank"`
		Confidence int      `json:"confidence"`
	} `json:"new_memories"`
	Patches []struct {
		ID         int      `json:"id"`
		Content    string   `json:"content"`
		Category   string   `json:"category"`
		Tags       []string `json:"tags"`
		Rank       int      `json:"rank"`
		Confidence int      `json:"confidence"`
	} `json:"patches"`
	ArchiveIDs []int    `json:"archive_ids"`
	Notes      []string `json:"notes"`
}

type meditationPlan struct {
	Documents map[string]string `json:"documents"`
	Lessons   []string          `json:"lessons"`
	Notes     []string          `json:"notes"`
}

func runDreamWorkflow(ctx context.Context, db *sql.DB, cfg Config, args map[string]any) (map[string]any, error) {
	operation := emptyDefault(stringArg(args, "operation"), "check")
	threshold := intFromAny(args["threshold"], 100)
	if threshold <= 0 {
		threshold = 100
	}
	memories, err := collectDreamMemories(db, 120)
	if err != nil {
		return nil, err
	}
	candidateIDs := memoryIDs(memories)
	result := map[string]any{
		"operation":           operation,
		"threshold":           threshold,
		"active_memories":     len(memories),
		"candidate_ids":       candidateIDs,
		"needs_consolidation": len(memories) >= threshold,
	}
	switch operation {
	case "check":
		logWorkflowAudit(db, "dream", args, result, "complete")
		return result, nil
	case "schedule":
		scheduleArgs := map[string]any{
			"operation":        "create",
			"name":             "Hourly dream memory check",
			"tool":             "dream",
			"args":             map[string]any{"operation": "check", "threshold": threshold},
			"run_at":           time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			"interval_seconds": 3600,
		}
		scheduled, _, err := executeScheduleTool(db, scheduleArgs)
		result["schedule"] = scheduled
		logWorkflowAudit(db, "dream", args, result, statusFromErr(err))
		return result, err
	case "run":
	default:
		return nil, fmt.Errorf("unsupported dream operation %q", operation)
	}

	applied := map[string]any{}
	archiveIDs, duplicateNotes := archiveExactDuplicateMemories(db, memories)
	applied["deterministic_archive_ids"] = archiveIDs
	if len(duplicateNotes) > 0 {
		applied["deterministic_notes"] = duplicateNotes
	}

	if cfg.APIKey != "" && len(memories) > 0 {
		plan, err := generateDreamPlan(ctx, cfg, memories)
		if err != nil {
			applied["model_warning"] = err.Error()
		} else {
			modelApplied, err := applyDreamPlan(db, plan, candidateIDs)
			if err != nil {
				applied["model_apply_warning"] = err.Error()
			} else {
				applied["model_plan"] = modelApplied
			}
		}
	}
	result["applied"] = applied
	logWorkflowAudit(db, "dream", args, result, "complete")
	return result, nil
}

func runMeditateWorkflow(ctx context.Context, db *sql.DB, cfg Config, args map[string]any) (map[string]any, error) {
	operation := emptyDefault(stringArg(args, "operation"), "status")
	result := map[string]any{"operation": operation, "allowed_documents": allowedMeditationDocuments()}
	switch operation {
	case "status":
		result["status"] = "ready"
		logWorkflowAudit(db, "meditate", args, result, "complete")
		return result, nil
	case "schedule":
		scheduleArgs := map[string]any{
			"operation":        "create",
			"name":             "Daily meditation",
			"tool":             "meditate",
			"args":             map[string]any{"operation": "run", "reason": stringArg(args, "reason")},
			"run_at":           time.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
			"interval_seconds": 86400,
		}
		scheduled, _, err := executeScheduleTool(db, scheduleArgs)
		result["schedule"] = scheduled
		logWorkflowAudit(db, "meditate", args, result, statusFromErr(err))
		return result, err
	case "run":
	default:
		return nil, fmt.Errorf("unsupported meditate operation %q", operation)
	}

	contextPack, err := collectMeditationContext(db)
	if err != nil {
		return nil, err
	}
	result["context"] = map[string]any{
		"recent_messages": contextPack["recent_message_count"],
		"recent_events":   contextPack["recent_event_count"],
		"memory_count":    contextPack["memory_count"],
	}
	if cfg.APIKey == "" {
		result["status"] = "skipped"
		result["warning"] = "missing API key; deterministic audit recorded without document edits"
		logWorkflowAudit(db, "meditate", args, result, "complete")
		return result, nil
	}
	plan, err := generateMeditationPlan(ctx, cfg, contextPack)
	if err != nil {
		result["status"] = "failed"
		result["error"] = err.Error()
		logWorkflowAudit(db, "meditate", args, result, "failed")
		return result, err
	}
	applied, skipped, err := applyMeditationPlan(plan)
	result["applied_documents"] = applied
	result["skipped_documents"] = skipped
	result["lessons"] = plan.Lessons
	result["notes"] = plan.Notes
	result["status"] = statusFromErr(err)
	logWorkflowAudit(db, "meditate", args, result, statusFromErr(err))
	return result, err
}

func collectDreamMemories(db *sql.DB, limit int) ([]workflowMemory, error) {
	rows, err := db.Query(`
		SELECT model_id, content, category, tags_json, rank, confidence, recalled_count, used_count, updated_at
		FROM long_term_memories
		WHERE agent_id = ? AND deleted_at IS NULL AND status = 'active'
		ORDER BY used_count ASC, recalled_count DESC, updated_at ASC
		LIMIT ?
	`, defaultAgentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []workflowMemory
	for rows.Next() {
		var m workflowMemory
		var tagsJSON string
		if err := rows.Scan(&m.ModelID, &m.Content, &m.Category, &tagsJSON, &m.Rank, &m.Confidence, &m.Recalled, &m.Used, &m.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &m.Tags)
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func archiveExactDuplicateMemories(db *sql.DB, memories []workflowMemory) ([]int, []string) {
	type best struct {
		id    int
		score int
	}
	bestByContent := map[string]best{}
	var archiveIDs []int
	var notes []string
	for _, m := range memories {
		key := normalizeMemoryContent(m.Content)
		if key == "" {
			continue
		}
		score := m.Rank*1000 + m.Used*100 + m.Recalled
		if current, ok := bestByContent[key]; ok {
			if score > current.score {
				archiveIDs = append(archiveIDs, current.id)
				bestByContent[key] = best{id: m.ModelID, score: score}
			} else {
				archiveIDs = append(archiveIDs, m.ModelID)
			}
		} else {
			bestByContent[key] = best{id: m.ModelID, score: score}
		}
	}
	if len(archiveIDs) == 0 {
		return nil, nil
	}
	now := time.Now().Format(time.RFC3339Nano)
	for _, id := range archiveIDs {
		if _, err := db.Exec(`UPDATE long_term_memories SET status = 'archived', updated_at = ? WHERE model_id = ? AND agent_id = ?`, now, id, defaultAgentID); err == nil {
			notes = append(notes, fmt.Sprintf("archived exact duplicate M%d", id))
		}
	}
	return archiveIDs, notes
}

func generateDreamPlan(ctx context.Context, cfg Config, memories []workflowMemory) (dreamPlan, error) {
	var plan dreamPlan
	template := readTextDefault("prompts/dream_prompt.md", "Consolidate memories. Return JSON.")
	rawMemories, _ := json.MarshalIndent(memories, "", "  ")
	prompt := template + "\n\nMemory candidates JSON:\n" + string(rawMemories)
	text, err := callWorkflowText(ctx, cfg, prompt, 5*time.Minute)
	if err != nil {
		return plan, err
	}
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func applyDreamPlan(db *sql.DB, plan dreamPlan, allowedIDs []int) (map[string]any, error) {
	allowed := intSet(allowedIDs)
	applied := map[string]any{"new_memory_ids": []int{}, "patched_ids": []int{}, "archived_ids": []int{}, "notes": plan.Notes}
	for _, item := range plan.NewMemories {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		result, _, err := memoryUpsert(db, map[string]any{
			"content":    trimRunes(item.Content, 1200),
			"category":   trimRunes(item.Category, 80),
			"tags":       item.Tags,
			"rank":       clampInt(item.Rank, 0, 10, 3),
			"confidence": clampInt(item.Confidence, 0, 100, 70),
		})
		if err != nil {
			return applied, err
		}
		applied["new_memory_ids"] = append(applied["new_memory_ids"].([]int), result["id"].(int))
	}
	for _, patch := range plan.Patches {
		if !allowed[patch.ID] {
			continue
		}
		_, _, err := memoryPatch(db, map[string]any{
			"id":         patch.ID,
			"content":    trimRunes(patch.Content, 1200),
			"category":   trimRunes(patch.Category, 80),
			"tags":       patch.Tags,
			"rank":       patch.Rank,
			"confidence": patch.Confidence,
		})
		if err != nil {
			return applied, err
		}
		applied["patched_ids"] = append(applied["patched_ids"].([]int), patch.ID)
	}
	now := time.Now().Format(time.RFC3339Nano)
	for _, id := range plan.ArchiveIDs {
		if !allowed[id] {
			continue
		}
		if _, err := db.Exec(`UPDATE long_term_memories SET status = 'archived', updated_at = ? WHERE model_id = ? AND agent_id = ?`, now, id, defaultAgentID); err != nil {
			return applied, err
		}
		applied["archived_ids"] = append(applied["archived_ids"].([]int), id)
	}
	return applied, nil
}

func collectMeditationContext(db *sql.DB) (map[string]any, error) {
	recentMessages, err := loadRecentMessagesFromDB(db, defaultThreadID, 80)
	if err != nil {
		return nil, err
	}
	memories, err := collectDreamMemories(db, 80)
	if err != nil {
		return nil, err
	}
	events, err := recentWorkflowEvents(db, 40)
	if err != nil {
		return nil, err
	}
	docs := map[string]string{}
	for _, path := range allowedMeditationDocuments() {
		docs[path] = readTextDefault(path, "")
	}
	return map[string]any{
		"recent_message_count": len(recentMessages),
		"recent_event_count":   len(events),
		"memory_count":         len(memories),
		"recent_messages":      messagesForWorkflow(recentMessages),
		"memories":             memories,
		"events":               events,
		"documents":            docs,
	}, nil
}

func generateMeditationPlan(ctx context.Context, cfg Config, contextPack map[string]any) (meditationPlan, error) {
	var plan meditationPlan
	template := readTextDefault("prompts/meditate_prompt.md", "Improve allowed prompt documents. Return JSON.")
	rawContext, _ := json.MarshalIndent(contextPack, "", "  ")
	prompt := template + "\n\nContext JSON:\n" + string(rawContext)
	text, err := callWorkflowText(ctx, cfg, prompt, 8*time.Minute)
	if err != nil {
		return plan, err
	}
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &plan); err != nil {
		return plan, err
	}
	return plan, nil
}

func applyMeditationPlan(plan meditationPlan) ([]string, []string, error) {
	allowed := map[string]bool{}
	for _, path := range allowedMeditationDocuments() {
		allowed[path] = true
	}
	var applied []string
	var skipped []string
	for path, content := range plan.Documents {
		clean := filepath.Clean(path)
		if !allowed[clean] {
			skipped = append(skipped, path+": path not allowed")
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" || len([]rune(content)) > 24000 {
			skipped = append(skipped, path+": empty or too large")
			continue
		}
		if err := backupWorkflowDocument(clean); err != nil {
			return applied, skipped, err
		}
		if err := os.WriteFile(clean, []byte(content+"\n"), 0644); err != nil {
			return applied, skipped, err
		}
		applied = append(applied, clean)
	}
	sort.Strings(applied)
	sort.Strings(skipped)
	return applied, skipped, nil
}

func callWorkflowText(ctx context.Context, cfg Config, prompt string, timeout time.Duration) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("missing API key")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body := map[string]any{
		"model":  cfg.Model,
		"stream": true,
		"input": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": prompt,
			}},
		}},
	}
	data, _ := json.Marshal(body)
	url := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("workflow api %s: %s", resp.Status, trimForStatus(raw))
	}
	text, _, _, err := parseResponseStream(resp.Body)
	if err != nil {
		return "", err
	}
	return text, nil
}

func recentWorkflowEvents(db *sql.DB, limit int) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT function_name, arguments_json, result_json, status, created_at
		FROM orchestrator_events
		WHERE thread_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, defaultThreadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []map[string]any
	for rows.Next() {
		var name, args, result, status, created string
		if err := rows.Scan(&name, &args, &result, &status, &created); err != nil {
			return nil, err
		}
		events = append(events, map[string]any{
			"name":       name,
			"args":       jsonRawObject(args),
			"result":     jsonRawObject(result),
			"status":     status,
			"created_at": created,
		})
	}
	return events, rows.Err()
}

func messagesForWorkflow(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		out = append(out, map[string]any{
			"seq":        m.Seq,
			"role":       m.Role,
			"text":       trimRunes(m.Text, 900),
			"images":     len(m.Images),
			"created_at": m.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func logWorkflowAudit(db *sql.DB, workflow string, args map[string]any, result map[string]any, status string) {
	now := time.Now().Format(time.RFC3339Nano)
	argsJSON, _ := json.Marshal(args)
	resultJSON, _ := json.Marshal(result)
	_, _ = db.Exec(`
		INSERT INTO orchestrator_events(id, thread_id, message_id, function_name, arguments_json, result_json, status, created_at, completed_at)
		VALUES(?, ?, NULL, ?, ?, ?, ?, ?, ?)
	`, newID("evt"), defaultThreadID, workflow+".workflow", string(argsJSON), string(resultJSON), status, now, now)
}

func backupWorkflowDocument(path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(path, string(filepath.Separator), "__")
	backup := filepath.Join("app_outputs", "workflow_backups", time.Now().Format("20060102_150405")+"__"+name)
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		return err
	}
	return os.WriteFile(backup, raw, 0644)
}

func allowedMeditationDocuments() []string {
	return []string{
		"behavior_guidance.md",
		"prompts/summarize_prompt.md",
		"prompts/dream_prompt.md",
		"prompts/selfie_prompt.md",
		"prompts/meditate_prompt.md",
	}
}

func readTextDefault(path, def string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	return string(raw)
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end >= start {
		return text[start : end+1]
	}
	return text
}

func memoryIDs(memories []workflowMemory) []int {
	ids := make([]int, 0, len(memories))
	for _, m := range memories {
		ids = append(ids, m.ModelID)
	}
	return ids
}

func intSet(values []int) map[int]bool {
	set := map[int]bool{}
	for _, v := range values {
		set[v] = true
	}
	return set
}

func normalizeMemoryContent(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func clampInt(v, min, max, def int) int {
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

func statusFromErr(err error) string {
	if err != nil {
		return "failed"
	}
	return "complete"
}
