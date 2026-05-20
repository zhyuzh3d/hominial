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
	sections["memory"] = formatMemories(ctx.Memories, cfg.MaxSectionChars)
	sections["summary"] = trimRunes(ctx.Summary.Content, cfg.MaxSectionChars)
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
		{"memory", "Long-Term Memory Recall"},
		{"summary", "Short-Term Memory Summary"},
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
		fmt.Fprintf(&b, "- rank=%d recall_count=%d id=%s: %s\n", m.Rank, m.RecallCount, m.ID, strings.TrimSpace(m.Content))
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
	return `Available functions can update long-term memory, role state, user profile, user context, environment, and request reference-image generation. Use them as private state actions. When updating scores, use 0-100 integers. Compare your previous prediction against the user's actual reply before updating control_score and behavior_effectiveness.`
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
