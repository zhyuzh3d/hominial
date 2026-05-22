package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"
)

const maxComputerActions = 10

type ComputerBackend interface {
	Name() string
	ScreenInfo() (ComputerScreenInfo, error)
	Screenshot(ctx context.Context, opts ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error)
	Execute(ctx context.Context, actions []ComputerAction) error
}

type ComputerWindowLocator interface {
	LocateWindow(ctx context.Context, opts ComputerScreenshotOptions) (ComputerWindowInfo, error)
}

type ComputerScreenInfo struct {
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	LogicalWidth  int     `json:"logical_width,omitempty"`
	LogicalHeight int     `json:"logical_height,omitempty"`
	Scale         float64 `json:"scale"`
}

type ComputerScreenshotOptions struct {
	X      int
	Y      int
	Width  int
	Height int

	TargetApp           string
	WindowTitleContains string
	CropToWindow        bool
}

type ComputerWindowInfo struct {
	App    string `json:"app"`
	Title  string `json:"title"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type ComputerAction struct {
	Type      string
	X         int
	Y         int
	HasX      bool
	HasY      bool
	Button    string
	Double    bool
	Key       string
	Modifiers []string
	Text      string
	MS        int
	DX        int
	DY        int
}

var currentComputerBackend ComputerBackend = newRobotGoComputerBackend()

func executeComputerTool(ctx context.Context, db *sql.DB, args map[string]any) (map[string]any, *Message, error) {
	operation := emptyDefault(stringArg(args, "operation"), "help")
	if operation == "help" || operation == "apis" || operation == "get_apis" {
		return computerHelpResult(db), nil, nil
	}
	if currentComputerBackend == nil {
		return nil, nil, errors.New("computer backend is not available")
	}
	if !computerUseEnabled(db) {
		return nil, nil, errors.New("computer use is disabled in settings")
	}

	switch operation {
	case "observe", "screenshot":
		path, info, window, err := captureComputerScreenshot(ctx, screenshotOptionsFromArgs(args))
		if err != nil {
			result := computerFailureResult("observe", err)
			return result, computerFailureMessage(result), nil
		}
		result := map[string]any{
			"ok":              true,
			"operation":       "observe",
			"backend":         currentComputerBackend.Name(),
			"screen":          info,
			"screenshot_path": path,
			"images":          []string{path},
		}
		if window != nil {
			result["window"] = *window
		}
		return result, computerResultMessage(result), nil
	case "act":
		actions, err := computerActionsFromAny(args["actions"])
		if err != nil {
			return nil, nil, err
		}
		if len(actions) == 0 {
			return nil, nil, errors.New("actions are required")
		}
		if len(actions) > maxComputerActions {
			return nil, nil, fmt.Errorf("too many computer actions: max %d", maxComputerActions)
		}
		if err := currentComputerBackend.Execute(ctx, actions); err != nil {
			result := computerFailureResult("act", err)
			result["executed"] = 0
			return result, computerFailureMessage(result), nil
		}
		result := map[string]any{
			"ok":        true,
			"operation": "act",
			"backend":   currentComputerBackend.Name(),
			"executed":  len(actions),
		}
		if boolArg(args, "return_screenshot", true) {
			path, info, window, err := captureComputerScreenshot(ctx, screenshotOptionsFromArgs(args))
			if err != nil {
				result["ok"] = false
				result["screenshot_error"] = err.Error()
				result["diagnosis"] = computerPermissionDiagnosis()
				return result, computerFailureMessage(result), nil
			}
			result["screen"] = info
			result["screenshot_path"] = path
			result["images"] = []string{path}
			if window != nil {
				result["window"] = *window
			}
		}
		return result, computerResultMessage(result), nil
	default:
		return nil, nil, fmt.Errorf("unsupported computer operation %q", operation)
	}
}

func computerResultMessage(result map[string]any) *Message {
	ok, _ := result["ok"].(bool)
	if !ok {
		return computerFailureMessage(result)
	}
	op, _ := result["operation"].(string)
	var b strings.Builder
	fmt.Fprintf(&b, "computer %s result:\n", emptyDefault(op, "operation"))
	b.WriteString("- ok: true\n")
	if executed, exists := result["executed"]; exists {
		fmt.Fprintf(&b, "- executed: %v\n", executed)
	}
	if screen, exists := result["screen"]; exists {
		raw, _ := json.Marshal(screen)
		fmt.Fprintf(&b, "- screen: %s\n", string(raw))
	}
	if window, exists := result["window"]; exists {
		raw, _ := json.Marshal(window)
		fmt.Fprintf(&b, "- window: %s\n", string(raw))
	}
	if path, _ := result["screenshot_path"].(string); path != "" {
		fmt.Fprintf(&b, "- screenshot_path: %s\n", path)
	}
	return &Message{
		Role:      "assistant",
		Text:      strings.TrimSpace(b.String()),
		Images:    stringSlice(result["images"]),
		CreatedAt: time.Now(),
	}
}

func computerFailureMessage(result map[string]any) *Message {
	op, _ := result["operation"].(string)
	errText, _ := result["error"].(string)
	if errText == "" {
		errText, _ = result["screenshot_error"].(string)
	}
	if errText == "" {
		return nil
	}
	text := fmt.Sprintf("computer %s failed: %s", emptyDefault(op, "operation"), errText)
	if diagnosis, ok := result["diagnosis"].(map[string]any); ok {
		if path, _ := diagnosis["macos_path"].(string); path != "" {
			text += "\n\nCheck: " + path
		}
		if grantTo := stringSlice(diagnosis["grant_to"]); len(grantTo) > 0 {
			text += "\nGrant permission to: " + strings.Join(grantTo, " / ")
		}
	}
	return &Message{Role: "assistant", Text: text, CreatedAt: time.Now()}
}

func computerFailureResult(operation string, err error) map[string]any {
	return map[string]any{
		"ok":        false,
		"operation": operation,
		"backend":   currentComputerBackend.Name(),
		"error":     err.Error(),
		"diagnosis": computerPermissionDiagnosis(),
	}
}

func computerPermissionDiagnosis() map[string]any {
	return map[string]any{
		"likely_cause": "macOS privacy permission is missing or attached to a different launcher.",
		"required_permissions": []string{
			"Screen Recording for screenshots",
			"Accessibility for mouse and keyboard control",
		},
		"grant_to": []string{
			"Hominial-Elli.app if you launch the macOS app bundle",
			"Hominial-Elli if you launch the bare binary directly",
			"Terminal or iTerm if you launch ./Hominial-Elli from a shell",
		},
		"macos_path": "System Settings -> Privacy & Security -> Screen Recording / Accessibility",
		"notes": []string{
			"After changing permissions, fully quit and restart Hominial-Elli.",
			"Prefer launching Hominial-Elli.app; it gives macOS a stable app identity for privacy permissions.",
			"If the app was rebuilt, remove the old privacy entry and add the current Hominial-Elli.app again.",
			"Do not attempt mouse actions until observe returns ok=true with a screenshot_path.",
		},
	}
}

func computerHelpResult(db *sql.DB) map[string]any {
	backend := "unavailable"
	if currentComputerBackend != nil {
		backend = currentComputerBackend.Name()
	}
	return map[string]any{
		"tool":                    "computer",
		"enabled":                 computerUseEnabled(db),
		"backend":                 backend,
		"permission_requirements": computerPermissionDiagnosis(),
		"operations": []map[string]any{
			{"operation": "help", "description": "Return this API guide. This is safe and available even when computer use is disabled."},
			{"operation": "observe", "description": "Capture the desktop screenshot. Pass crop_to_window=true with target_app/window_title_contains to crop a specific app window without changing the default fullscreen behavior. Use callback sendmsg target=ai to continue reasoning from the screenshot."},
			{"operation": "act", "description": "Execute up to 10 primitive mouse/keyboard actions. Set return_screenshot=true to observe the result."},
		},
		"observe_fields": []map[string]any{
			{"field": "crop_to_window", "type": "boolean", "description": "When true, locate a matching application window and capture only that window region. Defaults to false."},
			{"field": "target_app", "type": "string", "description": "Application/process name to match, e.g. Google Chrome or Chrome."},
			{"field": "window_title_contains", "type": "string", "description": "Case-insensitive substring of the window title, e.g. WPS Office for Mac."},
			{"field": "x/y/width/height", "type": "integer", "description": "Optional manual screenshot region. Existing fullscreen observe remains unchanged when omitted."},
		},
		"actions": []map[string]any{
			{"type": "move", "fields": "x, y", "description": "Move mouse pointer to screenshot pixel coordinates."},
			{"type": "click", "fields": "optional x, y, button=left|right|middle, double", "description": "Click at current pointer or optional coordinates."},
			{"type": "right_click", "fields": "optional x, y", "description": "Right click at current pointer or optional coordinates."},
			{"type": "double_click", "fields": "optional x, y, button=left|right|middle", "description": "Double click at current pointer or optional coordinates."},
			{"type": "key", "fields": "key, optional modifiers[]", "description": "Tap a keyboard key. Examples: enter, escape, tab, space, backspace, a."},
			{"type": "hotkey", "fields": "key, modifiers[]", "description": "Tap a modified key. Examples: key=c modifiers=[command], key=v modifiers=[command]."},
			{"type": "key_down", "fields": "key, optional modifiers[]", "description": "Hold a key down."},
			{"type": "key_up", "fields": "key, optional modifiers[]", "description": "Release a key."},
			{"type": "type", "fields": "text", "description": "Type a UTF-8 string into the focused control."},
			{"type": "wait", "fields": "ms", "description": "Pause before the next action. Max 5000 ms per action."},
			{"type": "scroll", "fields": "dx, dy", "description": "Scroll by horizontal/vertical amounts."},
		},
		"usage": map[string]any{
			"observe_then_continue": map[string]any{
				"operation": "observe",
				"callback":  map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
			},
			"observe_window_then_continue": map[string]any{
				"operation":             "observe",
				"crop_to_window":        true,
				"target_app":            "Google Chrome",
				"window_title_contains": "WPS Office for Mac",
				"callback":              map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
			},
			"act_then_continue": map[string]any{
				"operation":         "act",
				"return_screenshot": true,
				"actions":           []map[string]any{{"type": "click", "x": 200, "y": 300}, {"type": "type", "text": "hello"}, {"type": "key", "key": "enter"}},
				"callback":          map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
			},
		},
		"safety": []string{
			"Use observe before acting when coordinates are uncertain.",
			"Use the smallest action sequence that can make progress.",
			"Do not use computer from scheduled tool calls.",
			"Stop and ask the user before sensitive account, payment, deletion, or irreversible actions.",
		},
	}
}

func computerUseEnabled(db *sql.DB) bool {
	if db == nil {
		return defaultRuntimeSettings().ComputerUseEnabled
	}
	var settings RuntimeSettings
	if err := loadAppSetting(db, "runtime_settings", &settings); err != nil {
		return defaultRuntimeSettings().ComputerUseEnabled
	}
	return normalizeRuntimeSettings(settings).ComputerUseEnabled
}

func captureComputerScreenshot(ctx context.Context, opts ComputerScreenshotOptions) (string, ComputerScreenInfo, *ComputerWindowInfo, error) {
	var window *ComputerWindowInfo
	if opts.CropToWindow {
		locator, ok := currentComputerBackend.(ComputerWindowLocator)
		if !ok {
			return "", ComputerScreenInfo{}, nil, errors.New("computer backend does not support window lookup")
		}
		located, err := locator.LocateWindow(ctx, opts)
		if err != nil {
			return "", ComputerScreenInfo{}, nil, err
		}
		window = &located
		opts.X = located.X
		opts.Y = located.Y
		opts.Width = located.Width
		opts.Height = located.Height
	}

	img, info, err := currentComputerBackend.Screenshot(ctx, opts)
	if err != nil {
		return "", ComputerScreenInfo{}, nil, err
	}
	if img == nil {
		return "", ComputerScreenInfo{}, nil, errors.New("computer backend returned nil screenshot")
	}
	outPath, err := appOutputPath("screenshots", "screen_"+time.Now().Format("20060102_150405_000000000")+".png")
	if err != nil {
		return "", ComputerScreenInfo{}, nil, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", ComputerScreenInfo{}, nil, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", ComputerScreenInfo{}, nil, err
	}
	return outPath, info, window, nil
}

func screenshotOptionsFromArgs(args map[string]any) ComputerScreenshotOptions {
	return ComputerScreenshotOptions{
		X:                   intFromAny(args["x"], 0),
		Y:                   intFromAny(args["y"], 0),
		Width:               intFromAny(args["width"], 0),
		Height:              intFromAny(args["height"], 0),
		TargetApp:           strings.TrimSpace(stringArg(args, "target_app")),
		WindowTitleContains: strings.TrimSpace(stringArg(args, "window_title_contains")),
		CropToWindow:        boolArg(args, "crop_to_window", false),
	}
}

func computerActionsFromAny(v any) ([]ComputerAction, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, errors.New("actions must be an array")
	}
	actions := make([]ComputerAction, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("action %d must be an object", i)
		}
		action, err := computerActionFromMap(m)
		if err != nil {
			return nil, fmt.Errorf("action %d: %w", i, err)
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func computerActionFromMap(m map[string]any) (ComputerAction, error) {
	action := ComputerAction{
		Type:      strings.TrimSpace(stringArg(m, "type")),
		Button:    emptyDefault(strings.ToLower(strings.TrimSpace(stringArg(m, "button"))), "left"),
		Key:       strings.ToLower(strings.TrimSpace(stringArg(m, "key"))),
		Modifiers: normalizeKeyNames(stringSlice(m["modifiers"])),
		Text:      stringArg(m, "text"),
		MS:        intFromAny(m["ms"], 0),
		DX:        intFromAny(m["dx"], 0),
		DY:        intFromAny(m["dy"], 0),
	}
	if x, ok := optionalInt(m["x"]); ok {
		action.X = x
		action.HasX = true
	}
	if y, ok := optionalInt(m["y"]); ok {
		action.Y = y
		action.HasY = true
	}
	action.Double = boolArg(m, "double", false)
	if action.Type == "" {
		return ComputerAction{}, errors.New("type is required")
	}
	switch action.Type {
	case "move":
		if !action.HasX || !action.HasY {
			return ComputerAction{}, errors.New("move requires x and y")
		}
	case "click", "right_click", "double_click":
		if action.Button != "left" && action.Button != "right" && action.Button != "middle" {
			return ComputerAction{}, errors.New("button must be left, right, or middle")
		}
	case "key", "hotkey", "key_down", "key_up":
		if action.Key == "" {
			return ComputerAction{}, errors.New("key is required")
		}
	case "type":
		if action.Text == "" {
			return ComputerAction{}, errors.New("text is required")
		}
		if len([]rune(action.Text)) > 4000 {
			return ComputerAction{}, errors.New("text is too long")
		}
	case "wait":
		if action.MS < 0 || action.MS > 5000 {
			return ComputerAction{}, errors.New("wait ms must be between 0 and 5000")
		}
	case "scroll":
	default:
		return ComputerAction{}, fmt.Errorf("unsupported action type %q", action.Type)
	}
	return action, nil
}

func normalizeKeyNames(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "cmd":
			key = "command"
		case "ctrl":
			key = "control"
		case "option":
			key = "alt"
		}
		if key != "" {
			out = append(out, key)
		}
	}
	return out
}

func optionalInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	}
	return 0, false
}

func boolArg(args map[string]any, key string, def bool) bool {
	v, ok := args[key]
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}
