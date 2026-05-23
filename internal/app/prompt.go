package app

import (
	"fmt"
	"strings"
	"time"
)

func buildPrompt(ctx PromptContext) (PromptEnvelope, error) {
	cfg := ctx.Config
	if cfg.MaxPromptChars <= 0 {
		cfg = defaultPEConfig()
	}
	sections := map[string]string{}
	sections["role"] = trimRunes(strings.TrimSpace(ctx.RolePrompt), cfg.MaxRoleChars)
	sections["companion_profile"] = formatCompanionProfile(ctx.CompanionProfile)
	sections["memory"] = formatMemories(ctx.Memories, cfg.MaxSectionChars)
	sections["memory_index"] = trimRunes(ctx.MemoryIndex, cfg.MaxSectionChars/2)
	sections["summarization"] = trimRunes(ctx.Summarization.Content, cfg.MaxSectionChars)
	sections["turn_evaluations"] = trimRunes(ctx.TurnEvaluationContext, cfg.MaxSectionChars)
	sections["role_context"] = formatRoleContext(ctx.RoleState)
	sections["user_profile"] = formatUserProfile(ctx.UserProfile)
	sections["user_context"] = formatUserContext(ctx.UserContext)
	sections["environment"] = formatEnvironment(ctx.Now, ctx.Environment)
	sections["function_policy"] = functionPolicyText()

	system := composeSystemPrompt(sections, cfg.MaxPromptChars)
	input := []map[string]any{{
		"role": "system",
		"content": []map[string]any{{
			"type": "input_text",
			"text": system,
		}},
	}}
	for _, m := range ctx.Recent {
		msg, err := messageToAPIInput(m)
		if err != nil {
			return PromptEnvelope{}, err
		}
		if msg != nil {
			input = append(input, msg)
		}
	}
	sizes := map[string]int{"system_total": len([]rune(system))}
	for name, section := range sections {
		sizes[name] = len([]rune(section))
	}
	return PromptEnvelope{
		Input:        input,
		Tools:        apiTools(),
		WantsImage:   wantsImage(ctx.Recent),
		SystemPrompt: system,
		SectionSizes: sizes,
	}, nil
}

func messageToAPIInput(m Message) (map[string]any, error) {
	role := m.Role
	if role != "assistant" {
		role = "user"
	}
	parts := []map[string]any{}
	if strings.TrimSpace(m.Text) != "" {
		typ := "input_text"
		if role == "assistant" {
			typ = "output_text"
		}
		parts = append(parts, map[string]any{"type": typ, "text": m.Text})
	}
	if role == "user" {
		for _, img := range m.Attachments {
			dataURL, err := fileDataURL(img)
			if err != nil {
				return nil, err
			}
			parts = append(parts, map[string]any{"type": "input_image", "image_url": dataURL})
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return map[string]any{"role": role, "content": parts}, nil
}

func composeSystemPrompt(sections map[string]string, maxChars int) string {
	order := []struct {
		key   string
		title string
	}{
		{"role", "Role Setting"},
		{"companion_profile", "Companion Profile"},
		{"memory_index", "Memory Categories And Tags"},
		{"memory", "Long-Term Memory Recall"},
		{"summarization", "Short-Term Memory Summarization"},
		{"turn_evaluations", "Predictive Empathy Loop"},
		{"role_context", "Role Context"},
		{"user_profile", "User Profile"},
		{"user_context", "User Context"},
		{"environment", "Environment Context"},
		{"function_policy", "Function Calling Policy"},
	}
	var b strings.Builder
	for _, item := range order {
		value := strings.TrimSpace(sections[item.key])
		if value == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", item.title, value)
	}
	b.WriteString("## Response Strategy\n")
	b.WriteString("Respond naturally as the character. Maintain a clear sense of time, environment, relationship continuity, and your own goals. After each meaningful reply, use available functions when state, memory, prediction, or environment should be updated. Keep user-visible text concise unless the user asks for depth.\n")
	return trimRunes(b.String(), maxChars)
}

func formatMemories(memories []LongTermMemory, maxChars int) string {
	if len(memories) == 0 {
		return "No recalled long-term memories yet."
	}
	var b strings.Builder
	for _, m := range memories {
		id := m.ID
		if m.ModelID > 0 {
			id = fmt.Sprintf("M%d", m.ModelID)
		}
		category := emptyDefault(m.Category, "uncategorized")
		tags := strings.TrimSpace(m.TagsJSON)
		if tags == "" || tags == "null" {
			tags = "[]"
		}
		fmt.Fprintf(&b, "- id=%s category=%s tags=%s rank=%d confidence=%d recalled=%d used=%d: %s\n", id, category, tags, m.Rank, m.Confidence, m.RecalledCount, m.UsedCount, strings.TrimSpace(m.Content))
	}
	return trimRunes(b.String(), maxChars)
}

func formatRoleContext(s RoleState) string {
	return fmt.Sprintf(`health=%s
mental=%s
mood=%s
current_action=%s
short_purpose=%s
short_goal_closeness=%d
short_goal_deviation=%d
long_goal_closeness=%d
long_goal_deviation=%d
behavior_effectiveness=%d
control_score=%d
metadata=%s`, emptyDefault(s.Health, "stable"), emptyDefault(s.Mental, "clear"), emptyDefault(s.Mood, "calm"), emptyDefault(s.Action, "chatting"), s.ShortPurpose, s.ShortGoalCloseness, s.ShortGoalDeviation, s.LongGoalCloseness, s.LongGoalDeviation, s.BehaviorEffectiveness, s.ControlScore, emptyDefault(s.MetadataJSON, "{}"))
}

func formatUserProfile(p UserProfile) string {
	return fmt.Sprintf("user_set_profile=%s\ncharacter_estimated_profile=%s", emptyDefault(p.SetJSON, "{}"), emptyDefault(p.EstimatedJSON, "{}"))
}

func formatCompanionProfile(p ProfileSettings) string {
	lines := []string{}
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			lines = append(lines, fmt.Sprintf("%s=%s", key, strings.TrimSpace(value)))
		}
	}
	add("full_name", p.FullName)
	add("nickname", p.Nickname)
	add("avatar", p.Avatar)
	add("canon_image", p.CanonImage)
	add("gender", p.Gender)
	add("birth_date", p.BirthDate)
	add("description", p.Description)
	add("story", p.Story)
	add("personality", p.Personality)
	add("habits", p.Habits)
	return strings.Join(lines, "\n")
}

func formatUserContext(c UserContext) string {
	return fmt.Sprintf(`estimated_user_mood=%s
estimated_user_action=%s
estimated_user_environment=%s
next_action_prediction=%s
last_prediction=%s
prediction_evaluation=%s`, c.Mood, c.Action, c.Environment, c.NextActionPrediction, c.LastPrediction, emptyDefault(c.EvaluationJSON, "{}"))
}

func formatEnvironment(now time.Time, e EnvironmentState) string {
	return fmt.Sprintf(`current_time=%s
weekday=%s
lunar_calendar=not_calculated
virtual_scene=%s
surroundings=%s
random_seed=%d
metadata=%s`, now.Format("2006-01-02 15:04:05 MST"), now.Weekday().String(), emptyDefault(e.Scene, "quiet room"), emptyDefault(e.Surroundings, "desk, soft light, active chat window"), e.RandomSeed, emptyDefault(e.MetadataJSON, "{}"))
}

func functionPolicyText() string {
	return `Use only the model-visible tools when you need actions. Prefer compact unified tools: db, memory, query, evaluate_turn, sendmsg, selfie, computer, notify, schedule, summarize, dream, and meditate. computer is high-permission desktop observation/control; call computer operation=help once before using it for a task unless the same turn already provided the computer API guide, use observe before uncertain coordinates, and ask the user before sensitive, irreversible, account, payment, or deletion actions. For slow UI changes after computer act, prefer wait_after_ms, observe_retries, observe_interval_ms, and wait_until_changed so waiting happens inside the tool instead of repeated empty reasoning. If a computer result has ok=false, stop acting and explain the error/diagnosis to the user instead of trying more mouse or keyboard actions. A callback is another tool call; use callback.tool="sendmsg" to send a tool result to the user, to yourself for continuation, or to the internal event stream. user_set_profile is read-only; write only your own estimated user profile. Memory IDs are shown as M<number>. Mark a memory as used only when you actually relied on it in this reply; recalled memories are not automatically used.

Predictive empathy is the core self-evaluation loop. control_score means grasp of topic flow, interaction rhythm, emotional field, and conversational dynamics; it does not mean control over the user. After each meaningful assistant reply, call evaluate_turn to compare the previous prediction with the user's actual behavior, update control_score, update short/long goal distance and angle, patch the real-time interaction_strategy, and write the next_prediction. Predict response latency as part of next_prediction. Use 0-100 integers for scores.`
}

func trimRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "\n...[truncated]"
}

func emptyDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
