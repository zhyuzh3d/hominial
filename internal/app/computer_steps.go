package app

import (
	"database/sql"
	"encoding/json"
	"time"
)

type computerStepLogger struct {
	db        *sql.DB
	stepID    string
	startedAt time.Time
}

func startComputerStep(db *sql.DB, toolCallID, messageID string, args map[string]any) *computerStepLogger {
	if db == nil || toolCallID == "" {
		return nil
	}
	now := time.Now()
	raw, _ := json.Marshal(args)
	stepID := newID("computer_step")
	_, err := db.Exec(`
		INSERT INTO computer_steps(id, tool_call_id, message_id, thread_id, status, operation, arguments_json, started_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, stepID, toolCallID, messageID, defaultThreadID, "running", emptyDefault(stringArg(args, "operation"), "act"), string(raw), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return nil
	}
	return &computerStepLogger{db: db, stepID: stepID, startedAt: now}
}

func (l *computerStepLogger) StepID() string {
	if l == nil {
		return ""
	}
	return l.stepID
}

func (l *computerStepLogger) Complete(status string, result map[string]any) {
	if l == nil || l.db == nil || l.stepID == "" {
		return
	}
	now := time.Now().Format(time.RFC3339Nano)
	raw, _ := json.Marshal(result)
	_, _ = l.db.Exec(`
		UPDATE computer_steps
		SET status = ?, result_json = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`, emptyDefault(status, "complete"), string(raw), now, now, l.stepID)
}

func (l *computerStepLogger) LogLocalCheck(attempt int, screenshotPath string, diffScore float64, changed, stable bool, extra map[string]any) {
	if l == nil {
		return
	}
	l.logCheck("checkNext_local", attempt, screenshotPath, diffScore, changed, stable, "", "", 0, extra)
}

func (l *computerStepLogger) LogAICheck(attempt int, screenshotPath, state, reason string, confidence float64, extra map[string]any) {
	if l == nil {
		return
	}
	l.logCheck("checkNext_ai", attempt, screenshotPath, 0, false, false, state, reason, confidence, extra)
}

func (l *computerStepLogger) logCheck(kind string, attempt int, screenshotPath string, diffScore float64, changed, stable bool, state, reason string, confidence float64, extra map[string]any) {
	if l == nil || l.db == nil || l.stepID == "" {
		return
	}
	now := time.Now()
	raw, _ := json.Marshal(extra)
	_, _ = l.db.Exec(`
		INSERT INTO computer_step_checks(id, step_id, kind, attempt, elapsed_ms, screenshot_path, diff_score, changed, stable, state, reason, confidence, result_json, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newID("computer_check"), l.stepID, kind, attempt, int(now.Sub(l.startedAt)/time.Millisecond), screenshotPath, diffScore, boolInt(changed), boolInt(stable), state, reason, confidence, string(raw), now.Format(time.RFC3339Nano))
	_, _ = l.db.Exec(`UPDATE computer_steps SET updated_at = ? WHERE id = ?`, now.Format(time.RFC3339Nano), l.stepID)
}

func latestComputerStepForToolCall(db *sql.DB, toolCallID string) (string, error) {
	var stepID string
	err := db.QueryRow(`
		SELECT id FROM computer_steps
		WHERE tool_call_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, toolCallID).Scan(&stepID)
	return stepID, err
}

func loadComputerStepChecks(path, stepID string, limit int) ([]ComputerStepCheck, error) {
	if limit <= 0 {
		limit = 80
	}
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, step_id, kind, attempt, elapsed_ms, screenshot_path, diff_score, changed, stable, state, reason, confidence, result_json, created_at
		FROM computer_step_checks
		WHERE step_id = ?
		ORDER BY created_at
		LIMIT ?
	`, stepID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var checks []ComputerStepCheck
	for rows.Next() {
		var check ComputerStepCheck
		var changed, stable int
		var created string
		if err := rows.Scan(&check.ID, &check.StepID, &check.Kind, &check.Attempt, &check.ElapsedMS, &check.ScreenshotPath, &check.DiffScore, &changed, &stable, &check.State, &check.Reason, &check.Confidence, &check.ResultJSON, &created); err != nil {
			return nil, err
		}
		check.Changed = changed != 0
		check.Stable = stable != 0
		check.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		checks = append(checks, check)
	}
	return checks, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
