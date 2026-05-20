package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func maybeRefreshShortSummary(ctx context.Context, cfg Config, path string, peCfg PEConfig) error {
	db, err := openHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	summary, err := loadShortTermSummary(db, defaultThreadID)
	if err != nil {
		return err
	}
	var maxSeq, count int
	if err := db.QueryRow(`SELECT COALESCE(MAX(seq), 0), COUNT(*) FROM messages WHERE thread_id = ? AND deleted_at IS NULL`, defaultThreadID).Scan(&maxSeq, &count); err != nil {
		return err
	}
	targetSeq := maxSeq - peCfg.RecentMessagesK
	if targetSeq <= 0 || targetSeq <= summary.UpToSeq || targetSeq-summary.UpToSeq < peCfg.SummaryRefreshEvery {
		return nil
	}
	msgs, err := loadMessagesThroughSeq(db, defaultThreadID, summary.UpToSeq, targetSeq, 80)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	next, err := generateShortSummary(ctx, cfg, summary.Content, msgs)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		INSERT INTO short_term_summaries(thread_id, content, up_to_seq, source_messages, updated_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(thread_id) DO UPDATE SET content = excluded.content, up_to_seq = excluded.up_to_seq, source_messages = excluded.source_messages, updated_at = excluded.updated_at
	`, defaultThreadID, next, targetSeq, count, now)
	return err
}

func loadMessagesThroughSeq(db *sql.DB, threadID string, afterSeq, throughSeq, limit int) ([]Message, error) {
	rows, err := db.Query(`
		SELECT id, thread_id, COALESCE(seq, 0), role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL AND COALESCE(seq, 0) > ? AND COALESCE(seq, 0) <= ?
		ORDER BY COALESCE(seq, 0) DESC
		LIMIT ?
	`, threadID, afterSeq, throughSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reversed []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Seq, &m.Role, &m.Text, &created); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		reversed = append(reversed, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

func generateShortSummary(ctx context.Context, cfg Config, previous string, msgs []Message) (string, error) {
	if cfg.APIKey == "" {
		return "", nil
	}
	var transcript strings.Builder
	for _, m := range msgs {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			text = "[image message]"
		}
		fmt.Fprintf(&transcript, "%s: %s\n", m.Role, trimRunes(text, 800))
	}
	prompt := "Update the cumulative short-term memory summary. Prefer recent information, preserve stable facts, goals, unresolved tasks, emotional context, and relationship continuity. Return only the updated summary.\n\nPrevious summary:\n" + emptyDefault(previous, "(empty)") + "\n\nNew transcript:\n" + transcript.String()
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
	resp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("summary api %s: %s", resp.Status, trimForStatus(raw))
	}
	text, _, _, err := parseResponseStream(resp.Body)
	if err != nil {
		return "", err
	}
	return trimRunes(text, 5000), nil
}
