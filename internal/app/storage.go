package app

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func historyPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "chengcheng-chat", "chat.db")
}

func loadHistory(path string) ([]Message, error) {
	db, err := openHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, thread_id, COALESCE(seq, 0), role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL
		ORDER BY COALESCE(seq, 0), created_at
	`, defaultThreadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Seq, &m.Role, &m.Text, &created); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		attachments, images, err := loadMessageAttachments(db, m.ID)
		if err != nil {
			return nil, err
		}
		m.Attachments = attachments
		m.Images = images
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func loadRecentMessages(path, threadID string, limit int) ([]Message, bool, error) {
	if limit <= 0 {
		limit = defaultWindowSize
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return nil, false, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, thread_id, COALESCE(seq, 0), role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL
		ORDER BY COALESCE(seq, 0) DESC, created_at DESC
		LIMIT ?
	`, threadID, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var reversed []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Seq, &m.Role, &m.Text, &created); err != nil {
			return nil, false, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		reversed = append(reversed, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasOlder := len(reversed) > limit
	if hasOlder {
		reversed = reversed[:limit]
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if err := loadAttachmentsForMessages(db, reversed); err != nil {
		return nil, false, err
	}
	return reversed, hasOlder, nil
}

func loadOlderMessages(path, threadID string, beforeSeq, limit int) ([]Message, bool, error) {
	if limit <= 0 {
		limit = defaultWindowSize
	}
	if beforeSeq <= 0 {
		return nil, false, nil
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return nil, false, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, thread_id, COALESCE(seq, 0), role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL AND COALESCE(seq, 0) < ?
		ORDER BY COALESCE(seq, 0) DESC, created_at DESC
		LIMIT ?
	`, threadID, beforeSeq, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var reversed []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Seq, &m.Role, &m.Text, &created); err != nil {
			return nil, false, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		reversed = append(reversed, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasOlder := len(reversed) > limit
	if hasOlder {
		reversed = reversed[:limit]
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if err := loadAttachmentsForMessages(db, reversed); err != nil {
		return nil, false, err
	}
	return reversed, hasOlder, nil
}

func (a *ChatApp) saveHistory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saveHistoryLocked(false)
}

func (a *ChatApp) saveHistoryAllowEmpty() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saveHistoryLocked(true)
}

func (a *ChatApp) saveHistoryLocked(allowEmpty bool) {
	if a.historyPath == "" {
		return
	}
	if len(a.messages) == 0 && !allowEmpty {
		return
	}
	if err := saveHistoryDB(a.historyPath, a.messages, allowEmpty); err != nil {
		a.status = "History save failed: " + err.Error()
	}
}

func openHistoryDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
}

func openInitializedHistoryDB(path string) (*sql.DB, error) {
	if err := initHistoryDB(path); err != nil {
		return nil, err
	}
	return openHistoryDB(path)
}

func initHistoryDB(path string) error {
	db, err := openHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			provider TEXT,
			model TEXT,
			instructions_path TEXT,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS threads (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			user_id TEXT,
			default_agent_id TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(default_agent_id) REFERENCES agents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			seq INTEGER,
			role TEXT NOT NULL,
			actor_type TEXT NOT NULL DEFAULT 'user',
			actor_id TEXT,
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'complete',
			model TEXT,
			parent_message_id TEXT,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			FOREIGN KEY(thread_id) REFERENCES threads(id),
			FOREIGN KEY(parent_message_id) REFERENCES messages(id)
			)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_thread_seq ON messages(thread_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_thread_live_seq ON messages(thread_id, deleted_at, seq)`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			thread_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'attachment',
			path TEXT,
			mime_type TEXT,
			display_name TEXT,
			size_bytes INTEGER,
			width INTEGER,
			height INTEGER,
			content_id TEXT,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			FOREIGN KEY(message_id) REFERENCES messages(id),
			FOREIGN KEY(thread_id) REFERENCES threads(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_message ON attachments(message_id)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			thread_id TEXT NOT NULL,
			agent_id TEXT,
			name TEXT NOT NULL,
			arguments_json TEXT NOT NULL DEFAULT '{}',
			result_json TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			started_at TEXT,
			completed_at TEXT,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			FOREIGN KEY(message_id) REFERENCES messages(id),
			FOREIGN KEY(thread_id) REFERENCES threads(id),
			FOREIGN KEY(agent_id) REFERENCES agents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS long_term_memories (
			id TEXT PRIMARY KEY,
			model_id INTEGER,
			agent_id TEXT NOT NULL,
			user_id TEXT,
			content TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			rank INTEGER NOT NULL DEFAULT 1,
			confidence INTEGER NOT NULL DEFAULT 70,
			recall_count INTEGER NOT NULL DEFAULT 0,
			recalled_count INTEGER NOT NULL DEFAULT 0,
			used_count INTEGER NOT NULL DEFAULT 0,
			last_recalled_at TEXT,
			last_used_at TEXT,
			source TEXT NOT NULL DEFAULT 'manual',
			source_message_id TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			FOREIGN KEY(agent_id) REFERENCES agents(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ltm_agent_score ON long_term_memories(agent_id, rank, recall_count, updated_at)`,
		`CREATE TABLE IF NOT EXISTS short_term_summarizations (
			thread_id TEXT PRIMARY KEY,
			content TEXT NOT NULL DEFAULT '',
			up_to_seq INTEGER NOT NULL DEFAULT 0,
			source_messages INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			FOREIGN KEY(thread_id) REFERENCES threads(id)
		)`,
		`CREATE TABLE IF NOT EXISTS role_states (
			agent_id TEXT PRIMARY KEY,
			health TEXT NOT NULL DEFAULT 'stable',
			mental TEXT NOT NULL DEFAULT 'clear',
			mood TEXT NOT NULL DEFAULT 'calm',
			action TEXT NOT NULL DEFAULT 'chatting',
			short_purpose TEXT NOT NULL DEFAULT '',
			short_goal_closeness INTEGER NOT NULL DEFAULT 50,
			short_goal_deviation INTEGER NOT NULL DEFAULT 0,
			long_goal_closeness INTEGER NOT NULL DEFAULT 50,
			long_goal_deviation INTEGER NOT NULL DEFAULT 0,
			behavior_effectiveness INTEGER NOT NULL DEFAULT 50,
			control_score INTEGER NOT NULL DEFAULT 50,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(agent_id) REFERENCES agents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_profiles (
			user_id TEXT PRIMARY KEY,
			set_json TEXT NOT NULL DEFAULT '{}',
			estimated_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_contexts (
			user_id TEXT PRIMARY KEY,
			mood TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			environment TEXT NOT NULL DEFAULT '',
			next_action_prediction TEXT NOT NULL DEFAULT '',
			last_prediction TEXT NOT NULL DEFAULT '',
			evaluation_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS turn_evaluations (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			assistant_message_id TEXT,
			seq INTEGER NOT NULL DEFAULT 0,
			previous_prediction_json TEXT NOT NULL DEFAULT '{}',
			actual_behavior_json TEXT NOT NULL DEFAULT '{}',
			prediction_match_json TEXT NOT NULL DEFAULT '{}',
			control_score INTEGER NOT NULL DEFAULT 50,
			behavior_effectiveness INTEGER NOT NULL DEFAULT 50,
			short_goal_json TEXT NOT NULL DEFAULT '{}',
			long_goal_json TEXT NOT NULL DEFAULT '{}',
			interaction_strategy_json TEXT NOT NULL DEFAULT '{}',
			next_prediction_json TEXT NOT NULL DEFAULT '{}',
			raw_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			FOREIGN KEY(thread_id) REFERENCES threads(id),
			FOREIGN KEY(assistant_message_id) REFERENCES messages(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_turn_eval_thread_seq ON turn_evaluations(thread_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_turn_eval_message ON turn_evaluations(assistant_message_id)`,
		`CREATE TABLE IF NOT EXISTS environment_states (
			thread_id TEXT PRIMARY KEY,
			scene TEXT NOT NULL DEFAULT 'quiet room',
			surroundings TEXT NOT NULL DEFAULT 'desk, soft light, active chat window',
			random_seed INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			FOREIGN KEY(thread_id) REFERENCES threads(id)
		)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS orchestrator_events (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			message_id TEXT,
			function_name TEXT NOT NULL,
			arguments_json TEXT NOT NULL DEFAULT '{}',
			result_json TEXT,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			FOREIGN KEY(thread_id) REFERENCES threads(id),
			FOREIGN KEY(message_id) REFERENCES messages(id)
		)`,
		`CREATE TABLE IF NOT EXISTS scheduled_tool_calls (
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
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_snapshots (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			message_id TEXT,
			section_sizes_json TEXT NOT NULL DEFAULT '{}',
			system_prompt TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(thread_id) REFERENCES threads(id),
			FOREIGN KEY(message_id) REFERENCES messages(id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := migrateUnifiedToolColumns(db); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT OR IGNORE INTO users(id, display_name, created_at) VALUES(?, ?, ?)`, defaultUserID, "Local User", now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO agents(id, display_name, provider, created_at) VALUES(?, ?, ?, ?)`, defaultAgentID, "Assistant", "OpenAI-compatible", now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO threads(id, title, user_id, default_agent_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`, defaultThreadID, "Default conversation", defaultUserID, defaultAgentID, now, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO role_states(agent_id, updated_at) VALUES(?, ?)`, defaultAgentID, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO user_profiles(user_id, updated_at) VALUES(?, ?)`, defaultUserID, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO user_contexts(user_id, updated_at) VALUES(?, ?)`, defaultUserID, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO environment_states(thread_id, random_seed, updated_at) VALUES(?, ?, ?)`, defaultThreadID, time.Now().UnixNano(), now); err != nil {
		return err
	}
	if err := migrateTerminology(db); err != nil {
		return err
	}
	if err := seedDefaultScheduledTools(db); err != nil {
		return err
	}
	_, err = db.Exec(`
		WITH ordered AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) AS rn
			FROM messages
			WHERE thread_id = ? AND (seq IS NULL OR seq <= 0)
		)
		UPDATE messages
		SET seq = (SELECT rn FROM ordered WHERE ordered.id = messages.id)
		WHERE id IN (SELECT id FROM ordered)
	`, defaultThreadID)
	return err
}

func seedDefaultScheduledTools(db *sql.DB) error {
	now := time.Now()
	defaults := []struct {
		id       string
		name     string
		tool     string
		args     string
		interval int
		next     time.Time
	}{
		{
			id:       "default_dream_hourly",
			name:     "Hourly dream memory check",
			tool:     "dream",
			args:     `{"operation":"check","threshold":100}`,
			interval: 3600,
			next:     now.Add(time.Hour),
		},
		{
			id:       "default_meditate_daily",
			name:     "Daily meditation check",
			tool:     "meditate",
			args:     `{"operation":"status","reason":"daily scheduled review"}`,
			interval: 86400,
			next:     now.Add(24 * time.Hour),
		},
	}
	stamp := now.Format(time.RFC3339Nano)
	for _, item := range defaults {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO scheduled_tool_calls(id, owner, name, tool_name, arguments_json, callback_json, run_at, interval_seconds, status, next_run_at, created_at, updated_at)
			VALUES(?, 'system', ?, ?, ?, '{}', ?, ?, 'active', ?, ?, ?)
		`, item.id, item.name, item.tool, item.args, item.next.Format(time.RFC3339Nano), item.interval, item.next.Format(time.RFC3339Nano), stamp, stamp); err != nil {
			return err
		}
	}
	return nil
}

func migrateTerminology(db *sql.DB) error {
	if ok, err := tableExists(db, "short_term_summaries"); err != nil {
		return err
	} else if ok {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO short_term_summarizations(thread_id, content, up_to_seq, source_messages, updated_at, metadata_json)
			SELECT thread_id, content, up_to_seq, source_messages, updated_at, metadata_json
			FROM short_term_summaries
		`); err != nil {
			return err
		}
	}
	if ok, err := tableExists(db, "scheduled_tool_calls"); err != nil {
		return err
	} else if ok {
		_, _ = db.Exec(`UPDATE scheduled_tool_calls SET tool_name = 'meditate', name = CASE WHEN name = 'Daily soul optimization check' THEN 'Daily meditation check' ELSE name END, arguments_json = REPLACE(arguments_json, 'soul_optimize', 'meditate'), updated_at = ? WHERE tool_name = 'soul_optimize'`, time.Now().Format(time.RFC3339Nano))
		var hasNew int
		_ = db.QueryRow(`SELECT COUNT(*) FROM scheduled_tool_calls WHERE id = 'default_meditate_daily'`).Scan(&hasNew)
		if hasNew == 0 {
			_, _ = db.Exec(`UPDATE scheduled_tool_calls SET id = 'default_meditate_daily', name = 'Daily meditation check', tool_name = 'meditate', arguments_json = '{"operation":"status","reason":"daily scheduled review"}', updated_at = ? WHERE id = 'default_soul_daily'`, time.Now().Format(time.RFC3339Nano))
		} else {
			_, _ = db.Exec(`DELETE FROM scheduled_tool_calls WHERE id = 'default_soul_daily'`)
		}
	}
	if ok, err := tableExists(db, "orchestrator_events"); err != nil {
		return err
	} else if ok {
		_, _ = db.Exec(`UPDATE orchestrator_events SET function_name = REPLACE(function_name, 'soul_optimize', 'meditate') WHERE function_name LIKE '%soul_optimize%'`)
		_, _ = db.Exec(`UPDATE orchestrator_events SET function_name = REPLACE(function_name, 'summary', 'summarize') WHERE function_name LIKE '%summary%'`)
	}
	return nil
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	return count > 0, err
}

func migrateUnifiedToolColumns(db *sql.DB) error {
	adds := []struct {
		table string
		name  string
		def   string
	}{
		{"long_term_memories", "model_id", "INTEGER"},
		{"long_term_memories", "category", "TEXT NOT NULL DEFAULT ''"},
		{"long_term_memories", "tags_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"long_term_memories", "confidence", "INTEGER NOT NULL DEFAULT 70"},
		{"long_term_memories", "recalled_count", "INTEGER NOT NULL DEFAULT 0"},
		{"long_term_memories", "used_count", "INTEGER NOT NULL DEFAULT 0"},
		{"long_term_memories", "last_used_at", "TEXT"},
		{"long_term_memories", "source_message_id", "TEXT"},
		{"long_term_memories", "status", "TEXT NOT NULL DEFAULT 'active'"},
	}
	for _, add := range adds {
		if err := addColumnIfMissing(db, add.table, add.name, add.def); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ltm_model_id ON long_term_memories(model_id) WHERE model_id IS NOT NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_ltm_category ON long_term_memories(agent_id, category, status)`); err != nil {
		return err
	}
	if err := backfillMemoryModelIDs(db); err != nil {
		return err
	}
	return nil
}

func addColumnIfMissing(db *sql.DB, table, column, def string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + def)
	return err
}

func backfillMemoryModelIDs(db *sql.DB) error {
	rows, err := db.Query(`SELECT id FROM long_term_memories WHERE model_id IS NULL ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var next int
	_ = db.QueryRow(`SELECT COALESCE(MAX(model_id), 0) + 1 FROM long_term_memories`).Scan(&next)
	for _, id := range ids {
		if _, err := db.Exec(`UPDATE long_term_memories SET model_id = ? WHERE id = ?`, next, id); err != nil {
			return err
		}
		next++
	}
	return nil
}

func loadUISettings(path string, cfg Config) (UISettings, error) {
	if err := initHistoryDB(path); err != nil {
		return UISettings{}, err
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return UISettings{}, err
	}
	defer db.Close()
	settings := UISettings{System: cfg, Runtime: defaultRuntimeSettings()}
	var setJSON string
	if err := db.QueryRow(`SELECT set_json FROM user_profiles WHERE user_id = ?`, defaultUserID).Scan(&setJSON); err == nil {
		_ = json.Unmarshal([]byte(emptyDefault(setJSON, "{}")), &settings.User)
	}
	var companion ProfileSettings
	if err := loadAppSetting(db, "companion_profile", &companion); err == nil {
		settings.Companion = companion
	}
	var system Config
	if err := loadAppSetting(db, "system_access", &system); err == nil {
		if strings.TrimSpace(system.BaseURL) != "" {
			settings.System.BaseURL = system.BaseURL
		}
		if strings.TrimSpace(system.Model) != "" {
			settings.System.Model = system.Model
		}
		if strings.TrimSpace(system.APIKey) != "" {
			settings.System.APIKey = system.APIKey
		}
	}
	var runtime RuntimeSettings
	if err := loadAppSetting(db, "runtime_settings", &runtime); err == nil {
		settings.Runtime = normalizeRuntimeSettings(runtime)
	}
	var appearance AppearanceSettings
	if err := loadAppSetting(db, "appearance_settings", &appearance); err == nil {
		settings.Appearance = normalizeAppearanceSettings(appearance)
	} else {
		settings.Appearance = normalizeAppearanceSettings(settings.Appearance)
	}
	return settings, nil
}

func saveUISettings(path string, settings UISettings) error {
	if err := initHistoryDB(path); err != nil {
		return err
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().Format(time.RFC3339Nano)
	userJSON, err := json.Marshal(settings.User)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE user_profiles SET set_json = ?, updated_at = ? WHERE user_id = ?`, string(userJSON), now, defaultUserID); err != nil {
		return err
	}
	if err := saveAppSetting(db, "companion_profile", settings.Companion); err != nil {
		return err
	}
	if err := saveAppSetting(db, "system_access", settings.System); err != nil {
		return err
	}
	settings.Runtime = normalizeRuntimeSettings(settings.Runtime)
	if err := saveAppSetting(db, "runtime_settings", settings.Runtime); err != nil {
		return err
	}
	settings.Appearance = normalizeAppearanceSettings(settings.Appearance)
	if err := saveAppSetting(db, "appearance_settings", settings.Appearance); err != nil {
		return err
	}
	return applyRuntimeSchedules(db, settings.Runtime)
}

func loadCompanionProfile(db *sql.DB) ProfileSettings {
	var p ProfileSettings
	_ = loadAppSetting(db, "companion_profile", &p)
	return p
}

func loadAppSetting(db *sql.DB, key string, dest any) error {
	var raw string
	if err := db.QueryRow(`SELECT value_json FROM app_settings WHERE key = ?`, key).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(emptyDefault(raw, "{}")), dest)
}

func saveAppSetting(db *sql.DB, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err = db.Exec(`
		INSERT INTO app_settings(key, value_json, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`, key, string(raw), now)
	return err
}

func normalizeRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	def := defaultRuntimeSettings()
	if settings.ContextMessagesK <= 0 {
		settings.ContextMessagesK = def.ContextMessagesK
	}
	if settings.MemoryTopN < 0 {
		settings.MemoryTopN = def.MemoryTopN
	}
	if settings.MemoryRandomM < 0 {
		settings.MemoryRandomM = def.MemoryRandomM
	}
	if settings.SummarizeThreshold <= 0 {
		settings.SummarizeThreshold = def.SummarizeThreshold
	}
	if settings.DreamTriggerThreshold <= 0 {
		settings.DreamTriggerThreshold = def.DreamTriggerThreshold
	}
	return settings
}

func applyRuntimeSchedules(db *sql.DB, settings RuntimeSettings) error {
	settings = normalizeRuntimeSettings(settings)
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		UPDATE scheduled_tool_calls
		SET arguments_json = ?, updated_at = ?
		WHERE id = 'default_dream_hourly'
	`, fmt.Sprintf(`{"operation":"check","threshold":%d}`, settings.DreamTriggerThreshold), now); err != nil {
		return err
	}
	meditateStatus := "paused"
	if settings.DailyMeditateEnabled {
		meditateStatus = "active"
	}
	_, err := db.Exec(`UPDATE scheduled_tool_calls SET status = ?, updated_at = ? WHERE id = 'default_meditate_daily'`, meditateStatus, now)
	return err
}

func loadPromptEditorTexts() map[string]string {
	return map[string]string{
		"summarize": readTextDefault("prompts/summarize_prompt.md", ""),
		"dream":     readTextDefault("prompts/dream_prompt.md", ""),
		"meditate":  readTextDefault("prompts/meditate_prompt.md", ""),
	}
}

func savePromptEditorTexts(prompts map[string]string) error {
	paths := map[string]string{
		"summarize": "prompts/summarize_prompt.md",
		"dream":     "prompts/dream_prompt.md",
		"meditate":  "prompts/meditate_prompt.md",
	}
	for key, path := range paths {
		text, ok := prompts[key]
		if !ok {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n"), 0644); err != nil {
			return err
		}
	}
	return nil
}

func loadWorkflowLogs(path string, limit int) ([]WorkflowLog, error) {
	if limit <= 0 {
		limit = 50
	}
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, function_name, arguments_json, COALESCE(result_json, ''), status, created_at
		FROM orchestrator_events
		WHERE function_name IN ('summarize.workflow', 'dream.workflow', 'meditate.workflow')
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []WorkflowLog
	for rows.Next() {
		var log WorkflowLog
		var created string
		if err := rows.Scan(&log.ID, &log.Name, &log.Arguments, &log.Result, &log.Status, &created); err != nil {
			return nil, err
		}
		log.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func loadMemoryEntries(path, mode string, limit int) ([]LongTermMemory, error) {
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	filter := ""
	if mode == "knowledge" {
		filter = "AND (category = 'knowledge' OR tags_json LIKE '%knowledge%' OR tags_json LIKE '%知识%')"
	} else {
		filter = "AND category != 'knowledge'"
	}
	query := `
		SELECT id, model_id, content, category, tags_json, rank, confidence, recall_count, recalled_count, used_count, last_recalled_at, last_used_at, source_message_id, status, created_at, updated_at
		FROM long_term_memories
		WHERE agent_id = ? AND deleted_at IS NULL ` + filter + `
		ORDER BY status = 'active' DESC, rank DESC, updated_at DESC
	`
	args := []any{defaultAgentID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []LongTermMemory
	for rows.Next() {
		var m LongTermMemory
		var last, lastUsed, sourceMessage, created, updated sql.NullString
		if err := rows.Scan(&m.ID, &m.ModelID, &m.Content, &m.Category, &m.TagsJSON, &m.Rank, &m.Confidence, &m.RecallCount, &m.RecalledCount, &m.UsedCount, &last, &lastUsed, &sourceMessage, &m.Status, &created, &updated); err != nil {
			return nil, err
		}
		m.LastRecalledAt, _ = time.Parse(time.RFC3339Nano, last.String)
		m.LastUsedAt, _ = time.Parse(time.RFC3339Nano, lastUsed.String)
		m.SourceMessageID = sourceMessage.String
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated.String)
		entries = append(entries, m)
	}
	return entries, rows.Err()
}

func saveMemoryEntry(path, mode string, entry LongTermMemory) (int, error) {
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	now := time.Now().Format(time.RFC3339Nano)
	if strings.TrimSpace(entry.Content) == "" {
		return 0, fmt.Errorf("content is required")
	}
	if mode == "knowledge" && strings.TrimSpace(entry.Category) == "" {
		entry.Category = "knowledge"
	}
	if strings.TrimSpace(entry.TagsJSON) == "" {
		entry.TagsJSON = "[]"
	}
	if entry.Rank < 0 {
		entry.Rank = 0
	}
	if entry.Rank > 10 {
		entry.Rank = 10
	}
	if entry.Confidence < 0 {
		entry.Confidence = 0
	}
	if entry.Confidence > 100 {
		entry.Confidence = 100
	}
	if strings.TrimSpace(entry.Status) == "" {
		entry.Status = "active"
	}
	if entry.ModelID <= 0 {
		modelID, err := nextMemoryModelID(db)
		if err != nil {
			return 0, err
		}
		_, err = db.Exec(`
			INSERT INTO long_term_memories(id, model_id, agent_id, user_id, content, category, tags_json, rank, confidence, source, status, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'settings', ?, ?, ?)
		`, newID("mem"), modelID, defaultAgentID, defaultUserID, entry.Content, entry.Category, entry.TagsJSON, entry.Rank, entry.Confidence, entry.Status, now, now)
		return modelID, err
	}
	_, err = db.Exec(`
		UPDATE long_term_memories
		SET content = ?, category = ?, tags_json = ?, rank = ?, confidence = ?, status = ?, updated_at = ?, deleted_at = NULL
		WHERE model_id = ? AND agent_id = ?
	`, entry.Content, entry.Category, entry.TagsJSON, entry.Rank, entry.Confidence, entry.Status, now, entry.ModelID, defaultAgentID)
	return entry.ModelID, err
}

func archiveMemoryEntry(path string, modelID int) error {
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().Format(time.RFC3339Nano)
	_, err = db.Exec(`UPDATE long_term_memories SET status = 'archived', deleted_at = ?, updated_at = ? WHERE model_id = ? AND agent_id = ?`, now, now, modelID, defaultAgentID)
	return err
}

func loadMessageAttachments(db *sql.DB, messageID string) ([]string, []string, error) {
	rows, err := db.Query(`
		SELECT kind, path
		FROM attachments
		WHERE message_id = ?
		ORDER BY created_at, id
	`, messageID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var attachments, images []string
	for rows.Next() {
		var kind, path string
		if err := rows.Scan(&kind, &path); err != nil {
			return nil, nil, err
		}
		switch kind {
		case "assistant_image", "generated_image", "output_image":
			images = append(images, path)
		default:
			attachments = append(attachments, path)
		}
	}
	return attachments, images, rows.Err()
}

func loadAttachmentsForMessages(db *sql.DB, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(messages))
	args := make([]any, 0, len(messages))
	indexByID := make(map[string]int, len(messages))
	for i := range messages {
		if messages[i].ID == "" {
			continue
		}
		indexByID[messages[i].ID] = i
		placeholders = append(placeholders, "?")
		args = append(args, messages[i].ID)
	}
	if len(args) == 0 {
		return nil
	}
	rows, err := db.Query(`
		SELECT message_id, kind, path
		FROM attachments
		WHERE message_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY message_id, created_at, id
	`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID, kind, path string
		if err := rows.Scan(&messageID, &kind, &path); err != nil {
			return err
		}
		idx, ok := indexByID[messageID]
		if !ok {
			continue
		}
		switch kind {
		case "assistant_image", "generated_image", "output_image":
			messages[idx].Images = append(messages[idx].Images, path)
		default:
			messages[idx].Attachments = append(messages[idx].Attachments, path)
		}
	}
	return rows.Err()
}

func saveHistoryDB(path string, messages []Message, allowEmpty bool) error {
	if len(messages) == 0 && !allowEmpty {
		return nil
	}
	if err := initHistoryDB(path); err != nil {
		return err
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Format(time.RFC3339Nano)
	if allowEmpty && len(messages) == 0 {
		if _, err := tx.Exec(`UPDATE messages SET deleted_at = ? WHERE thread_id = ? AND deleted_at IS NULL`, now, defaultThreadID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE threads SET updated_at = ? WHERE id = ?`, now, defaultThreadID); err != nil {
			return err
		}
		return tx.Commit()
	}
	for i := range messages {
		m := &messages[i]
		if m.ID == "" {
			m.ID = newID("msg")
		}
		if m.ThreadID == "" {
			m.ThreadID = defaultThreadID
		}
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		actorType := "user"
		actorID := defaultUserID
		if m.Role == "assistant" {
			actorType = "agent"
			actorID = defaultAgentID
		}
		created := m.CreatedAt.Format(time.RFC3339Nano)
		if m.Seq <= 0 {
			m.Seq = i + 1
		}
		if _, err := tx.Exec(`
			INSERT INTO messages(id, thread_id, seq, role, actor_type, actor_id, content, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				seq = excluded.seq,
				role = excluded.role,
				actor_type = excluded.actor_type,
				actor_id = excluded.actor_id,
				content = excluded.content,
				updated_at = excluded.updated_at,
				deleted_at = NULL
		`, m.ID, m.ThreadID, m.Seq, m.Role, actorType, actorID, m.Text, created, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM attachments WHERE message_id = ?`, m.ID); err != nil {
			return err
		}
		for idx, p := range m.Attachments {
			if err := insertAttachment(tx, m, idx, "user_attachment", p); err != nil {
				return err
			}
		}
		for idx, p := range m.Images {
			if err := insertAttachment(tx, m, idx, "assistant_image", p); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE threads SET updated_at = ? WHERE id = ?`, now, defaultThreadID); err != nil {
		return err
	}
	return tx.Commit()
}

func saveMessageDB(path string, m *Message) error {
	if err := initHistoryDB(path); err != nil {
		return err
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if m.ID == "" {
		m.ID = newID("msg")
	}
	if m.ThreadID == "" {
		m.ThreadID = defaultThreadID
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.Seq <= 0 {
		var maxSeq sql.NullInt64
		if err := tx.QueryRow(`SELECT MAX(COALESCE(seq, 0)) FROM messages WHERE thread_id = ?`, m.ThreadID).Scan(&maxSeq); err != nil {
			return err
		}
		m.Seq = int(maxSeq.Int64) + 1
	}
	actorType := "user"
	actorID := defaultUserID
	if m.Role == "assistant" {
		actorType = "agent"
		actorID = defaultAgentID
	}
	now := time.Now().Format(time.RFC3339Nano)
	created := m.CreatedAt.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`
		INSERT INTO messages(id, thread_id, seq, role, actor_type, actor_id, content, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			seq = excluded.seq,
			role = excluded.role,
			actor_type = excluded.actor_type,
			actor_id = excluded.actor_id,
			content = excluded.content,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`, m.ID, m.ThreadID, m.Seq, m.Role, actorType, actorID, m.Text, created, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM attachments WHERE message_id = ?`, m.ID); err != nil {
		return err
	}
	for idx, p := range m.Attachments {
		if err := insertAttachment(tx, m, idx, "user_attachment", p); err != nil {
			return err
		}
	}
	for idx, p := range m.Images {
		if err := insertAttachment(tx, m, idx, "assistant_image", p); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE threads SET updated_at = ? WHERE id = ?`, now, m.ThreadID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAttachment(tx *sql.Tx, m *Message, idx int, kind, path string) error {
	info, _ := os.Stat(path)
	var size int64
	if info != nil {
		size = info.Size()
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err := tx.Exec(`
		INSERT INTO attachments(id, message_id, thread_id, kind, path, mime_type, display_name, size_bytes, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newID(fmt.Sprintf("att_%d", idx)), m.ID, m.ThreadID, kind, path, mimeType, filepath.Base(path), size, now)
	return err
}

func migrateJSONHistory(dbPath string) error {
	jsonPath := filepath.Join(filepath.Dir(dbPath), "history.json")
	if _, err := os.Stat(jsonPath); err != nil {
		return nil
	}
	dbMessages, err := loadHistory(dbPath)
	if err == nil && len(dbMessages) > 0 {
		return nil
	}
	msgs, err := loadJSONHistory(jsonPath)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	if err := saveHistoryDB(dbPath, msgs, false); err != nil {
		return err
	}
	_ = os.Rename(jsonPath, jsonPath+".migrated")
	return nil
}

func loadJSONHistory(path string) ([]Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var store historyStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Messages == nil {
		if backup, err := loadJSONHistory(path + ".bak"); err == nil {
			return backup, nil
		}
		return []Message{}, nil
	}
	for i := range store.Messages {
		if store.Messages[i].ID == "" {
			store.Messages[i].ID = newID("jsonmsg")
		}
		if store.Messages[i].ThreadID == "" {
			store.Messages[i].ThreadID = defaultThreadID
		}
	}
	return store.Messages, nil
}

func newID(prefix string) string {
	now := time.Now().UnixNano()
	sum := sha1.Sum([]byte(fmt.Sprintf("%s-%d-%d", prefix, now, time.Now().Nanosecond())))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func defaultPEConfig() PEConfig {
	return PEConfig{
		LongMemoryTopN:        5,
		LongMemoryRandomM:     5,
		RecentMessagesK:       40,
		MessageWindowSize:     defaultWindowSize,
		SummarizeThreshold:    40,
		MaxPromptChars:        24000,
		MaxRoleChars:          2000,
		MaxSectionChars:       5000,
		SummarizeEvery:        20,
		ReferenceImageTimeout: 10 * time.Minute,
	}
}

func defaultRuntimeSettings() RuntimeSettings {
	cfg := defaultPEConfig()
	return RuntimeSettings{
		ContextMessagesK:      cfg.RecentMessagesK,
		MemoryTopN:            cfg.LongMemoryTopN,
		MemoryRandomM:         cfg.LongMemoryRandomM,
		SummarizeThreshold:    cfg.SummarizeThreshold,
		DreamTriggerThreshold: 100,
		DailyMeditateEnabled:  true,
		ComputerUseEnabled:    false,
	}
}

func peConfigFromRuntime(settings RuntimeSettings) PEConfig {
	cfg := defaultPEConfig()
	if settings.ContextMessagesK > 0 {
		cfg.RecentMessagesK = settings.ContextMessagesK
	}
	if settings.MemoryTopN >= 0 {
		cfg.LongMemoryTopN = settings.MemoryTopN
	}
	if settings.MemoryRandomM >= 0 {
		cfg.LongMemoryRandomM = settings.MemoryRandomM
	}
	if settings.SummarizeThreshold > 0 {
		cfg.SummarizeThreshold = settings.SummarizeThreshold
	}
	return cfg
}

func loadPromptContext(path string, cfg PEConfig) (PromptContext, error) {
	if err := initHistoryDB(path); err != nil {
		return PromptContext{}, err
	}
	db, err := openHistoryDB(path)
	if err != nil {
		return PromptContext{}, err
	}
	defer db.Close()
	if err := seedMemoriesFromMarkdown(db); err != nil {
		return PromptContext{}, err
	}
	role, _ := os.ReadFile("character.md")
	behavior, _ := os.ReadFile("behavior_guidance.md")
	rolePrompt := strings.TrimSpace(string(role))
	if strings.TrimSpace(string(behavior)) != "" {
		rolePrompt += "\n\n## Behavior Guidance\n" + strings.TrimSpace(string(behavior))
	}
	memories, err := recallLongTermMemories(db, cfg.LongMemoryTopN, cfg.LongMemoryRandomM)
	if err != nil {
		return PromptContext{}, err
	}
	memoryIndex, err := loadMemoryIndex(db)
	if err != nil {
		return PromptContext{}, err
	}
	turnEvaluationContext, err := loadTurnEvaluationContext(db, 6)
	if err != nil {
		return PromptContext{}, err
	}
	summarization, err := loadShortTermSummarization(db, defaultThreadID)
	if err != nil {
		return PromptContext{}, err
	}
	recent, err := loadRecentMessagesFromDB(db, defaultThreadID, cfg.RecentMessagesK)
	if err != nil {
		return PromptContext{}, err
	}
	roleState, err := loadRoleState(db)
	if err != nil {
		return PromptContext{}, err
	}
	userProfile, err := loadUserProfile(db)
	if err != nil {
		return PromptContext{}, err
	}
	userContext, err := loadUserContext(db)
	if err != nil {
		return PromptContext{}, err
	}
	env, err := loadEnvironmentState(db)
	if err != nil {
		return PromptContext{}, err
	}
	return PromptContext{
		Config:                cfg,
		RolePrompt:            rolePrompt,
		CompanionProfile:      loadCompanionProfile(db),
		Memories:              memories,
		MemoryIndex:           memoryIndex,
		Summarization:         summarization,
		TurnEvaluationContext: turnEvaluationContext,
		Recent:                recent,
		RoleState:             roleState,
		UserProfile:           userProfile,
		UserContext:           userContext,
		Environment:           env,
		Now:                   time.Now(),
	}, nil
}

func seedMemoriesFromMarkdown(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM long_term_memories WHERE agent_id = ? AND deleted_at IS NULL`, defaultAgentID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	raw, err := os.ReadFile("memories.md")
	if err != nil {
		return nil
	}
	parts := splitMemoryMarkdown(string(raw))
	now := time.Now().Format(time.RFC3339Nano)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		modelID, err := nextMemoryModelID(db)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`
			INSERT INTO long_term_memories(id, model_id, agent_id, user_id, content, rank, source, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newID("mem"), modelID, defaultAgentID, defaultUserID, part, 3, "memories.md", now, now); err != nil {
			return err
		}
	}
	return nil
}

func nextMemoryModelID(db *sql.DB) (int, error) {
	var next int
	err := db.QueryRow(`SELECT COALESCE(MAX(model_id), 0) + 1 FROM long_term_memories`).Scan(&next)
	return next, err
}

func splitMemoryMarkdown(src string) []string {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil
	}
	lines := strings.Split(src, "\n")
	var out []string
	var current []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "## ") {
			if len(current) > 0 {
				out = append(out, strings.TrimSpace(strings.Join(current, "\n")))
			}
			current = []string{strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		out = append(out, strings.TrimSpace(strings.Join(current, "\n")))
	}
	if len(out) == 0 {
		out = []string{src}
	}
	return out
}

func recallLongTermMemories(db *sql.DB, topN, randomM int) ([]LongTermMemory, error) {
	if topN < 0 {
		topN = 0
	}
	if randomM < 0 {
		randomM = 0
	}
	seen := map[string]bool{}
	var memories []LongTermMemory
	readRows := func(rows *sql.Rows) error {
		defer rows.Close()
		for rows.Next() {
			var m LongTermMemory
			var last, lastUsed, sourceMessage, created, updated sql.NullString
			if err := rows.Scan(&m.ID, &m.ModelID, &m.Content, &m.Category, &m.TagsJSON, &m.Rank, &m.Confidence, &m.RecallCount, &m.RecalledCount, &m.UsedCount, &last, &lastUsed, &sourceMessage, &m.Status, &created, &updated); err != nil {
				return err
			}
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			m.LastRecalledAt, _ = time.Parse(time.RFC3339Nano, last.String)
			m.LastUsedAt, _ = time.Parse(time.RFC3339Nano, lastUsed.String)
			m.SourceMessageID = sourceMessage.String
			m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
			m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated.String)
			memories = append(memories, m)
		}
		return rows.Err()
	}
	if topN > 0 {
		rows, err := db.Query(`
			SELECT id, model_id, content, category, tags_json, rank, confidence, recall_count, recalled_count, used_count, last_recalled_at, last_used_at, source_message_id, status, created_at, updated_at
			FROM long_term_memories
			WHERE agent_id = ? AND deleted_at IS NULL AND status = 'active'
			ORDER BY (rank * 1000000 + used_count * 1000 + strftime('%s', updated_at)) DESC
			LIMIT ?
		`, defaultAgentID, topN)
		if err != nil {
			return nil, err
		}
		if err := readRows(rows); err != nil {
			return nil, err
		}
	}
	if randomM > 0 {
		rows, err := db.Query(`
			SELECT id, model_id, content, category, tags_json, rank, confidence, recall_count, recalled_count, used_count, last_recalled_at, last_used_at, source_message_id, status, created_at, updated_at
			FROM long_term_memories
			WHERE agent_id = ? AND deleted_at IS NULL AND status = 'active'
			ORDER BY random()
			LIMIT ?
		`, defaultAgentID, randomM)
		if err != nil {
			return nil, err
		}
		if err := readRows(rows); err != nil {
			return nil, err
		}
	}
	if len(memories) > 0 {
		now := time.Now().Format(time.RFC3339Nano)
		for _, m := range memories {
			_, _ = db.Exec(`UPDATE long_term_memories SET recalled_count = recalled_count + 1, recall_count = recall_count + 1, last_recalled_at = ?, updated_at = ? WHERE id = ?`, now, now, m.ID)
		}
	}
	return memories, nil
}

func loadMemoryIndex(db *sql.DB) (string, error) {
	rows, err := db.Query(`
		SELECT category, tags_json, COUNT(*)
		FROM long_term_memories
		WHERE agent_id = ? AND deleted_at IS NULL AND status = 'active'
		GROUP BY category, tags_json
	`, defaultAgentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	categories := map[string]int{}
	tags := map[string]int{}
	for rows.Next() {
		var category, tagsJSON string
		var count int
		if err := rows.Scan(&category, &tagsJSON, &count); err != nil {
			return "", err
		}
		category = strings.TrimSpace(category)
		if category != "" {
			categories[category] += count
		}
		var list []string
		if json.Unmarshal([]byte(tagsJSON), &list) == nil {
			for _, tag := range list {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tags[tag] += count
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(categories) == 0 && len(tags) == 0 {
		return "No memory categories or tags yet. You may create concise categories and tags through the memory tool.", nil
	}
	return "categories=" + compactCountMap(categories, 24) + "\ntags=" + compactCountMap(tags, 40), nil
}

func compactCountMap(values map[string]int, limit int) string {
	if len(values) == 0 {
		return "(none)"
	}
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(values))
	for k, v := range values {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s:%d", p.key, p.count))
	}
	return strings.Join(parts, ", ")
}

func loadShortTermSummarization(db *sql.DB, threadID string) (ShortTermSummarization, error) {
	now := time.Now().Format(time.RFC3339Nano)
	_, _ = db.Exec(`INSERT OR IGNORE INTO short_term_summarizations(thread_id, updated_at) VALUES(?, ?)`, threadID, now)
	var s ShortTermSummarization
	var updated string
	err := db.QueryRow(`
		SELECT thread_id, content, up_to_seq, source_messages, updated_at
		FROM short_term_summarizations WHERE thread_id = ?
	`, threadID).Scan(&s.ThreadID, &s.Content, &s.UpToSeq, &s.SourceMessages, &updated)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return s, err
}

func loadRecentMessagesFromDB(db *sql.DB, threadID string, limit int) ([]Message, error) {
	rows, err := db.Query(`
		SELECT id, thread_id, COALESCE(seq, 0), role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL
		ORDER BY COALESCE(seq, 0) DESC, created_at DESC
		LIMIT ?
	`, threadID, limit)
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
	if err := loadAttachmentsForMessages(db, reversed); err != nil {
		return nil, err
	}
	return reversed, nil
}

func loadRoleState(db *sql.DB) (RoleState, error) {
	var s RoleState
	var updated string
	err := db.QueryRow(`
		SELECT health, mental, mood, action, short_purpose, short_goal_closeness, short_goal_deviation,
			long_goal_closeness, long_goal_deviation, behavior_effectiveness, control_score, metadata_json, updated_at
		FROM role_states WHERE agent_id = ?
	`, defaultAgentID).Scan(&s.Health, &s.Mental, &s.Mood, &s.Action, &s.ShortPurpose, &s.ShortGoalCloseness, &s.ShortGoalDeviation, &s.LongGoalCloseness, &s.LongGoalDeviation, &s.BehaviorEffectiveness, &s.ControlScore, &s.MetadataJSON, &updated)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return s, err
}

func loadUserProfile(db *sql.DB) (UserProfile, error) {
	var p UserProfile
	var updated string
	err := db.QueryRow(`SELECT user_id, set_json, estimated_json, updated_at FROM user_profiles WHERE user_id = ?`, defaultUserID).Scan(&p.UserID, &p.SetJSON, &p.EstimatedJSON, &updated)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return p, err
}

func loadUserContext(db *sql.DB) (UserContext, error) {
	var c UserContext
	var updated string
	err := db.QueryRow(`
		SELECT user_id, mood, action, environment, next_action_prediction, last_prediction, evaluation_json, updated_at
		FROM user_contexts WHERE user_id = ?
	`, defaultUserID).Scan(&c.UserID, &c.Mood, &c.Action, &c.Environment, &c.NextActionPrediction, &c.LastPrediction, &c.EvaluationJSON, &updated)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, err
}

func loadEnvironmentState(db *sql.DB) (EnvironmentState, error) {
	var e EnvironmentState
	var updated string
	err := db.QueryRow(`SELECT thread_id, scene, surroundings, random_seed, metadata_json, updated_at FROM environment_states WHERE thread_id = ?`, defaultThreadID).Scan(&e.ThreadID, &e.Scene, &e.Surroundings, &e.RandomSeed, &e.MetadataJSON, &updated)
	e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return e, err
}

func loadTurnEvaluationContext(db *sql.DB, limit int) (string, error) {
	if limit <= 0 {
		limit = 6
	}
	rows, err := db.Query(`
		SELECT seq, control_score, behavior_effectiveness, prediction_match_json, short_goal_json, long_goal_json, interaction_strategy_json, next_prediction_json, created_at
		FROM turn_evaluations
		WHERE thread_id = ?
		ORDER BY seq DESC, created_at DESC
		LIMIT ?
	`, defaultThreadID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var reversed []string
	for rows.Next() {
		var seq, control, effectiveness int
		var match, shortGoal, longGoal, strategy, nextPrediction, created string
		if err := rows.Scan(&seq, &control, &effectiveness, &match, &shortGoal, &longGoal, &strategy, &nextPrediction, &created); err != nil {
			return "", err
		}
		reversed = append(reversed, fmt.Sprintf("- seq=%d control=%d effectiveness=%d match=%s short_goal=%s long_goal=%s strategy=%s next_prediction=%s created_at=%s",
			seq, control, effectiveness, compactJSON(match, 360), compactJSON(shortGoal, 280), compactJSON(longGoal, 280), compactJSON(strategy, 320), compactJSON(nextPrediction, 320), created))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(reversed) == 0 {
		return "No turn evaluations yet. Start the predictive empathy loop by calling evaluate_turn after meaningful replies.", nil
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return strings.Join(reversed, "\n"), nil
}

func compactJSON(src string, max int) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return "{}"
	}
	var v any
	if json.Unmarshal([]byte(src), &v) == nil {
		raw, _ := json.Marshal(v)
		src = string(raw)
	}
	return trimRunes(src, max)
}

func savePromptSnapshot(path, messageID string, envelope PromptEnvelope) {
	db, err := openInitializedHistoryDB(path)
	if err != nil {
		return
	}
	defer db.Close()
	sizes, _ := json.Marshal(envelope.SectionSizes)
	now := time.Now().Format(time.RFC3339Nano)
	_, _ = db.Exec(`
		INSERT INTO prompt_snapshots(id, thread_id, message_id, section_sizes_json, system_prompt, created_at)
		VALUES(?, ?, ?, ?, ?, ?)
	`, newID("prompt"), defaultThreadID, messageID, string(sizes), trimRunes(envelope.SystemPrompt, 24000), now)
}
