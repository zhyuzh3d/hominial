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
const maxComputerObserveRetries = 30
const maxComputerWaitMS = 60000
const maxComputerObserveIntervalMS = 10000

type ComputerBackend interface {
	Name() string
	ScreenInfo() (ComputerScreenInfo, error)
	Screenshot(ctx context.Context, opts ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error)
	Execute(ctx context.Context, actions []ComputerAction) error
}

type ComputerWindowLocator interface {
	LocateWindow(ctx context.Context, opts ComputerScreenshotOptions) (ComputerWindowInfo, error)
}

type ComputerWindowActivator interface {
	ActivateWindow(ctx context.Context, opts ComputerScreenshotOptions, window ComputerWindowInfo) error
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
	ActivateWindow      bool
}

type ComputerObservationOptions struct {
	WaitAfterMS       int
	ObserveRetries    int
	ObserveIntervalMS int
	WaitUntilChanged  bool
	ChangeThreshold   float64
	CheckNextAI       bool
	AIIntervalMS      int
	MaxAIChecks       int
	WaitGoal          string
	SuccessCriteria   string
	BlockedCriteria   string
	LastAction        string
}

type ComputerScreenshotResult struct {
	Path      string
	Info      ComputerScreenInfo
	Window    *ComputerWindowInfo
	Signature []uint8
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

func executeComputerTool(ctx context.Context, db *sql.DB, cfg Config, args map[string]any, toolCallID, messageID string) (map[string]any, *Message, error) {
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
		result, observation, err := observeComputer(ctx, cfg, screenshotOptionsFromArgs(args), observationOptionsFromArgs(args, false), nil, nil)
		if err != nil {
			result := computerFailureResult("observe", err)
			return result, computerFailureMessage(result), nil
		}
		payload := map[string]any{
			"ok":              true,
			"operation":       "observe",
			"backend":         currentComputerBackend.Name(),
			"screen":          result.Info,
			"screenshot_path": result.Path,
			"images":          []string{result.Path},
			"observation":     observation,
		}
		if result.Window != nil {
			payload["window"] = *result.Window
		}
		return payload, computerResultMessage(payload), nil
	case "act":
		stepLogger := startComputerStep(db, toolCallID, messageID, args)
		actions, err := computerActionsFromAny(args["actions"])
		if err != nil {
			stepLogger.Complete("failed", map[string]any{"error": err.Error()})
			return nil, nil, err
		}
		if len(actions) == 0 {
			stepLogger.Complete("failed", map[string]any{"error": "actions are required"})
			return nil, nil, errors.New("actions are required")
		}
		if len(actions) > maxComputerActions {
			stepLogger.Complete("failed", map[string]any{"error": fmt.Sprintf("too many computer actions: max %d", maxComputerActions)})
			return nil, nil, fmt.Errorf("too many computer actions: max %d", maxComputerActions)
		}
		returnScreenshot := boolArg(args, "return_screenshot", true)
		screenshotOpts := screenshotOptionsFromArgs(args)
		observationOpts := observationOptionsFromArgs(args, true)
		var baseline []uint8
		if returnScreenshot && observationOpts.WaitUntilChanged {
			before, _, _, err := captureComputerScreenshotImage(ctx, screenshotOpts)
			if err != nil {
				result := computerFailureResult("act", err)
				result["executed"] = 0
				result["stage"] = "pre_action_observe"
				if stepID := stepLogger.StepID(); stepID != "" {
					result["computer_step_id"] = stepID
				}
				stepLogger.Complete("failed", result)
				return result, computerFailureMessage(result), nil
			}
			baseline = computerImageSignature(before)
		}
		if err := currentComputerBackend.Execute(ctx, actions); err != nil {
			result := computerFailureResult("act", err)
			result["executed"] = 0
			if stepID := stepLogger.StepID(); stepID != "" {
				result["computer_step_id"] = stepID
			}
			stepLogger.Complete("failed", result)
			return result, computerFailureMessage(result), nil
		}
		result := map[string]any{
			"ok":        true,
			"operation": "act",
			"backend":   currentComputerBackend.Name(),
			"executed":  len(actions),
		}
		if stepID := stepLogger.StepID(); stepID != "" {
			result["computer_step_id"] = stepID
		}
		if returnScreenshot {
			screenshot, observation, err := observeComputer(ctx, cfg, screenshotOpts, observationOpts, baseline, stepLogger)
			if err != nil {
				result["ok"] = false
				result["screenshot_error"] = err.Error()
				result["diagnosis"] = computerPermissionDiagnosis()
				stepLogger.Complete("failed", result)
				return result, computerFailureMessage(result), nil
			}
			result["screen"] = screenshot.Info
			result["screenshot_path"] = screenshot.Path
			result["images"] = []string{screenshot.Path}
			result["observation"] = observation
			if screenshot.Window != nil {
				result["window"] = *screenshot.Window
			}
		}
		stepLogger.Complete("complete", result)
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
	if observation, exists := result["observation"]; exists {
		raw, _ := json.Marshal(observation)
		fmt.Fprintf(&b, "- observation: %s\n", string(raw))
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
			{"operation": "observe", "description": "Capture the desktop screenshot. Pass crop_to_window=true with target_app/window_title_contains to activate and crop a specific app window without changing the default fullscreen behavior. Use callback sendmsg target=ai to continue reasoning from the screenshot."},
			{"operation": "act", "description": "Execute up to 10 primitive mouse/keyboard actions. Set return_screenshot=true to observe the result. For slow UI changes, use wait_after_ms plus observe_retries/observe_interval_ms or wait_until_changed."},
		},
		"observe_fields": []map[string]any{
			{"field": "crop_to_window", "type": "boolean", "description": "When true, locate a matching application window and capture only that window region. Defaults to false."},
			{"field": "activate_window", "type": "boolean", "description": "When crop_to_window=true, bring the matching window to the foreground before screenshot. Defaults to true."},
			{"field": "target_app", "type": "string", "description": "Application/process name to match, e.g. Google Chrome or Chrome."},
			{"field": "window_title_contains", "type": "string", "description": "Case-insensitive substring of the window title, e.g. WPS Office for Mac."},
			{"field": "x/y/width/height", "type": "integer", "description": "Optional manual screenshot region. Existing fullscreen observe remains unchanged when omitted."},
		},
		"loop_fields": []map[string]any{
			{"field": "wait_after_ms", "type": "integer", "description": "For observe or act+return_screenshot, wait this many milliseconds before the first screenshot. Defaults to 0 for observe and 800 for act."},
			{"field": "observe_retries", "type": "integer", "description": "Maximum screenshot attempts before returning. Defaults to 1, or 10 when wait_until_changed=true. Max 30."},
			{"field": "observe_interval_ms", "type": "integer", "description": "Milliseconds between screenshot attempts while waiting for change. Defaults to 1000. Max 10000."},
			{"field": "wait_until_changed", "type": "boolean", "description": "When true, compare screenshots and keep waiting until the image changes enough or retries are exhausted. For act, the baseline is captured before actions run."},
			{"field": "change_threshold", "type": "number", "description": "Normalized visual difference needed for wait_until_changed. Defaults to 0.02."},
			{"field": "check_next_ai", "type": "boolean", "description": "When true, periodically call a narrow visual judge outside the main chat context to decide waiting/done/blocked/ready_for_next."},
			{"field": "ai_check_interval_ms", "type": "integer", "description": "Minimum time between checkNext_ai calls. Defaults to 10000 ms."},
			{"field": "max_ai_checks", "type": "integer", "description": "Maximum checkNext_ai calls for this act. Defaults to 3."},
			{"field": "wait_goal/success_criteria/blocked_criteria", "type": "string", "description": "Short local context sent to checkNext_ai without the main chat history."},
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
				"activate_window":       true,
				"target_app":            "Google Chrome",
				"window_title_contains": "WPS Office for Mac",
				"callback":              map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
			},
			"act_then_continue": map[string]any{
				"operation":         "act",
				"return_screenshot": true,
				"wait_after_ms":     800,
				"actions":           []map[string]any{{"type": "click", "x": 200, "y": 300}, {"type": "type", "text": "hello"}, {"type": "key", "key": "enter"}},
				"callback":          map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
			},
			"act_wait_for_change_then_continue": map[string]any{
				"operation":            "act",
				"return_screenshot":    true,
				"wait_after_ms":        1000,
				"observe_retries":      12,
				"observe_interval_ms":  1000,
				"wait_until_changed":   true,
				"check_next_ai":        true,
				"ai_check_interval_ms": 10000,
				"max_ai_checks":        3,
				"actions":              []map[string]any{{"type": "click", "x": 200, "y": 300}},
				"callback":             map[string]any{"tool": "sendmsg", "args": map[string]any{"target": "ai", "kind": "tool_result"}},
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
	result, err := captureComputerScreenshotResult(ctx, opts)
	if err != nil {
		return "", ComputerScreenInfo{}, nil, err
	}
	return result.Path, result.Info, result.Window, nil
}

func captureComputerScreenshotResult(ctx context.Context, opts ComputerScreenshotOptions) (ComputerScreenshotResult, error) {
	img, info, window, err := captureComputerScreenshotImage(ctx, opts)
	if err != nil {
		return ComputerScreenshotResult{}, err
	}
	return writeComputerScreenshotResult(img, info, window)
}

func writeComputerScreenshotResult(img image.Image, info ComputerScreenInfo, window *ComputerWindowInfo) (ComputerScreenshotResult, error) {
	if img == nil {
		return ComputerScreenshotResult{}, errors.New("computer backend returned nil screenshot")
	}
	outPath, err := appOutputPath("screenshots", "screen_"+time.Now().Format("20060102_150405_000000000")+".png")
	if err != nil {
		return ComputerScreenshotResult{}, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return ComputerScreenshotResult{}, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return ComputerScreenshotResult{}, err
	}
	return ComputerScreenshotResult{
		Path:      outPath,
		Info:      info,
		Window:    window,
		Signature: computerImageSignature(img),
	}, nil
}

func captureComputerScreenshotImage(ctx context.Context, opts ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, *ComputerWindowInfo, error) {
	var window *ComputerWindowInfo
	if opts.CropToWindow {
		locator, ok := currentComputerBackend.(ComputerWindowLocator)
		if !ok {
			return nil, ComputerScreenInfo{}, nil, errors.New("computer backend does not support window lookup")
		}
		located, err := locator.LocateWindow(ctx, opts)
		if err != nil {
			return nil, ComputerScreenInfo{}, nil, err
		}
		window = &located
		if opts.ActivateWindow {
			activator, ok := currentComputerBackend.(ComputerWindowActivator)
			if !ok {
				return nil, ComputerScreenInfo{}, nil, errors.New("computer backend does not support window activation")
			}
			if err := activator.ActivateWindow(ctx, opts, located); err != nil {
				return nil, ComputerScreenInfo{}, nil, err
			}
			time.Sleep(350 * time.Millisecond)
			relocated, err := locator.LocateWindow(ctx, opts)
			if err != nil {
				return nil, ComputerScreenInfo{}, nil, err
			}
			window = &relocated
			located = relocated
		}
		opts.X = located.X
		opts.Y = located.Y
		opts.Width = located.Width
		opts.Height = located.Height
	}

	img, info, err := currentComputerBackend.Screenshot(ctx, opts)
	if err != nil {
		return nil, ComputerScreenInfo{}, nil, err
	}
	if img == nil {
		return nil, ComputerScreenInfo{}, nil, errors.New("computer backend returned nil screenshot")
	}
	return img, info, window, nil
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
		ActivateWindow:      boolArg(args, "activate_window", true),
	}
}

func observationOptionsFromArgs(args map[string]any, afterAction bool) ComputerObservationOptions {
	waitDefault := 0
	if afterAction {
		waitDefault = 800
	}
	waitUntilChanged := boolArg(args, "wait_until_changed", false)
	retriesDefault := 1
	if waitUntilChanged {
		retriesDefault = 10
	}
	return ComputerObservationOptions{
		WaitAfterMS:       clampedIntArg(args, "wait_after_ms", waitDefault, 0, maxComputerWaitMS),
		ObserveRetries:    clampedIntArg(args, "observe_retries", retriesDefault, 1, maxComputerObserveRetries),
		ObserveIntervalMS: clampedIntArg(args, "observe_interval_ms", 1000, 0, maxComputerObserveIntervalMS),
		WaitUntilChanged:  waitUntilChanged,
		ChangeThreshold:   clampFloat(floatFromAny(args["change_threshold"], 0.02), 0.001, 1, 0.02),
		CheckNextAI:       boolArg(args, "check_next_ai", false),
		AIIntervalMS:      clampedIntArg(args, "ai_check_interval_ms", 10000, 1000, maxComputerWaitMS),
		MaxAIChecks:       clampedIntArg(args, "max_ai_checks", 3, 0, 10),
		WaitGoal:          strings.TrimSpace(stringArg(args, "wait_goal")),
		SuccessCriteria:   strings.TrimSpace(stringArg(args, "success_criteria")),
		BlockedCriteria:   strings.TrimSpace(stringArg(args, "blocked_criteria")),
		LastAction:        strings.TrimSpace(stringArg(args, "last_action")),
	}
}

func clampedIntArg(args map[string]any, key string, def, min, max int) int {
	v := def
	if hasArg(args, key) {
		v = intFromAny(args[key], def)
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func observeComputer(ctx context.Context, cfg Config, screenshotOpts ComputerScreenshotOptions, observationOpts ComputerObservationOptions, baseline []uint8, stepLogger *computerStepLogger) (ComputerScreenshotResult, map[string]any, error) {
	if observationOpts.WaitAfterMS > 0 {
		if err := sleepContext(ctx, time.Duration(observationOpts.WaitAfterMS)*time.Millisecond); err != nil {
			return ComputerScreenshotResult{}, nil, err
		}
	}
	var lastImg image.Image
	var lastInfo ComputerScreenInfo
	var lastWindow *ComputerWindowInfo
	var lastSignature []uint8
	var lastScore float64
	changed := false
	lastAIAt := time.Time{}
	aiChecks := 0
	aiState := ""
	for attempt := 1; attempt <= observationOpts.ObserveRetries; attempt++ {
		img, info, window, err := captureComputerScreenshotImage(ctx, screenshotOpts)
		if err != nil {
			return ComputerScreenshotResult{}, nil, err
		}
		signature := computerImageSignature(img)
		lastImg = img
		lastInfo = info
		lastWindow = window
		lastSignature = signature
		if observationOpts.WaitUntilChanged && len(baseline) > 0 {
			lastScore = computerSignatureDifference(baseline, signature)
			changed = lastScore >= observationOpts.ChangeThreshold
			stepLogger.LogLocalCheck(attempt, "", lastScore, changed, false, map[string]any{
				"threshold": observationOpts.ChangeThreshold,
				"phase":     "post_action_observe",
			})
			if changed || attempt == observationOpts.ObserveRetries {
				result, err := writeComputerScreenshotResult(img, info, window)
				if err != nil {
					return ComputerScreenshotResult{}, nil, err
				}
				stepLogger.LogLocalCheck(attempt, result.Path, lastScore, changed, true, map[string]any{
					"threshold": observationOpts.ChangeThreshold,
					"phase":     "final_observe",
				})
				if shouldRunCheckNextAI(observationOpts, aiChecks, lastAIAt) {
					aiChecks++
					lastAIAt = time.Now()
					judge := runCheckNextAI(ctx, cfg, observationOpts, result.Path, attempt, stepLogger)
					aiState = judge.State
					if judge.State == "waiting" && attempt < observationOpts.ObserveRetries {
						if observationOpts.ObserveIntervalMS > 0 {
							if err := sleepContext(ctx, time.Duration(observationOpts.ObserveIntervalMS)*time.Millisecond); err != nil {
								return ComputerScreenshotResult{}, nil, err
							}
						}
						continue
					} else if judge.State == "done" || judge.State == "blocked" || judge.State == "ready_for_next" || judge.State == "uncertain" {
						observation := computerObservationResult(observationOpts, attempt, changed, lastScore)
						observation["check_next_ai_state"] = judge.State
						observation["check_next_ai_reason"] = judge.Reason
						observation["check_next_ai_confidence"] = judge.Confidence
						return result, observation, nil
					}
				}
				return result, computerObservationResult(observationOpts, attempt, changed, lastScore), nil
			}
		} else {
			result, err := writeComputerScreenshotResult(img, info, window)
			if err != nil {
				return ComputerScreenshotResult{}, nil, err
			}
			return result, computerObservationResult(observationOpts, attempt, false, 0), nil
		}
		if observationOpts.ObserveIntervalMS > 0 {
			if err := sleepContext(ctx, time.Duration(observationOpts.ObserveIntervalMS)*time.Millisecond); err != nil {
				return ComputerScreenshotResult{}, nil, err
			}
		}
	}
	if lastImg == nil {
		return ComputerScreenshotResult{}, nil, errors.New("computer observe produced no screenshot")
	}
	result, err := writeComputerScreenshotResult(lastImg, lastInfo, lastWindow)
	if err != nil {
		return ComputerScreenshotResult{}, nil, err
	}
	result.Signature = lastSignature
	observation := computerObservationResult(observationOpts, observationOpts.ObserveRetries, changed, lastScore)
	if aiState != "" {
		observation["check_next_ai_state"] = aiState
	}
	return result, observation, nil
}

func computerObservationResult(opts ComputerObservationOptions, attempts int, changed bool, changeScore float64) map[string]any {
	return map[string]any{
		"attempts":            attempts,
		"wait_after_ms":       opts.WaitAfterMS,
		"observe_retries":     opts.ObserveRetries,
		"observe_interval_ms": opts.ObserveIntervalMS,
		"wait_until_changed":  opts.WaitUntilChanged,
		"changed":             changed,
		"change_score":        changeScore,
		"change_threshold":    opts.ChangeThreshold,
		"check_next_ai":       opts.CheckNextAI,
	}
}

func shouldRunCheckNextAI(opts ComputerObservationOptions, checks int, last time.Time) bool {
	if !opts.CheckNextAI || opts.MaxAIChecks <= 0 || checks >= opts.MaxAIChecks {
		return false
	}
	return last.IsZero() || time.Since(last) >= time.Duration(opts.AIIntervalMS)*time.Millisecond
}

func runCheckNextAI(ctx context.Context, cfg Config, opts ComputerObservationOptions, screenshotPath string, attempt int, logger *computerStepLogger) CheckNextAIResult {
	result, raw, err := callCheckNextAI(ctx, cfg, opts.WaitGoal, opts.LastAction, opts.SuccessCriteria, opts.BlockedCriteria, screenshotPath)
	extra := map[string]any{"attempt": attempt}
	if raw != "" {
		extra["raw"] = raw
	}
	if err != nil {
		result = CheckNextAIResult{State: "uncertain", Reason: err.Error(), Confidence: 0}
		extra["error"] = err.Error()
	}
	logger.LogAICheck(attempt, screenshotPath, result.State, result.Reason, result.Confidence, extra)
	return result
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func computerImageSignature(img image.Image) []uint8 {
	const cols = 32
	const rows = 18
	bounds := img.Bounds()
	if bounds.Empty() {
		return nil
	}
	out := make([]uint8, 0, cols*rows)
	for row := 0; row < rows; row++ {
		y := bounds.Min.Y + (row*bounds.Dy()+bounds.Dy()/2)/rows
		for col := 0; col < cols; col++ {
			x := bounds.Min.X + (col*bounds.Dx()+bounds.Dx()/2)/cols
			r, g, b, _ := img.At(x, y).RGBA()
			gray := (299*uint32(r>>8) + 587*uint32(g>>8) + 114*uint32(b>>8)) / 1000
			out = append(out, uint8(gray))
		}
	}
	return out
}

func computerSignatureDifference(a, b []uint8) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var total int
	for i := 0; i < n; i++ {
		diff := int(a[i]) - int(b[i])
		if diff < 0 {
			diff = -diff
		}
		total += diff
	}
	return float64(total) / float64(n*255)
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
