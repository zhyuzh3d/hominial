package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-vgo/robotgo"
)

type robotGoComputerBackend struct{}

func newRobotGoComputerBackend() ComputerBackend {
	robotgo.MouseSleep = 40
	robotgo.KeySleep = 20
	robotgo.Scale = true
	return robotGoComputerBackend{}
}

func (robotGoComputerBackend) Name() string {
	return "robotgo"
}

func (robotGoComputerBackend) ScreenInfo() (ComputerScreenInfo, error) {
	logicalWidth, logicalHeight := robotgo.GetScreenSize()
	width, height := robotgo.GetScaleSize()
	return ComputerScreenInfo{
		Width:         width,
		Height:        height,
		LogicalWidth:  logicalWidth,
		LogicalHeight: logicalHeight,
		Scale:         robotgo.ScaleF(),
	}, nil
}

func (robotGoComputerBackend) Screenshot(ctx context.Context, opts ComputerScreenshotOptions) (image.Image, ComputerScreenInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, ComputerScreenInfo{}, err
	}
	info, err := robotGoComputerBackend{}.ScreenInfo()
	if err != nil {
		return nil, ComputerScreenInfo{}, err
	}
	var img image.Image
	if opts.Width > 0 && opts.Height > 0 {
		img, err = robotgo.CaptureImg(opts.X, opts.Y, opts.Width, opts.Height)
	} else {
		img, err = robotgo.CaptureImg(0, 0, info.Width, info.Height)
	}
	if err != nil {
		if runtime.GOOS == "darwin" {
			fallbackImg, fallbackErr := captureWithScreencapture(ctx, opts)
			if fallbackErr == nil {
				return fallbackImg, info, nil
			}
			return nil, ComputerScreenInfo{}, fmt.Errorf("robotgo screenshot failed: %w; macOS screencapture fallback failed: %v", err, fallbackErr)
		}
		return nil, ComputerScreenInfo{}, err
	}
	return img, info, nil
}

func (robotGoComputerBackend) LocateWindow(ctx context.Context, opts ComputerScreenshotOptions) (ComputerWindowInfo, error) {
	if runtime.GOOS != "darwin" {
		return ComputerWindowInfo{}, errors.New("window lookup is currently implemented only on macOS")
	}
	targetApp := strings.TrimSpace(opts.TargetApp)
	titleContains := strings.TrimSpace(opts.WindowTitleContains)
	if targetApp == "" && titleContains == "" {
		return ComputerWindowInfo{}, errors.New("target_app or window_title_contains is required for crop_to_window")
	}

	script := `
set windowLines to ""
tell application "System Events"
	repeat with p in application processes
		set appName to name of p as text
		try
			repeat with w in windows of p
				set winTitle to name of w as text
				set winPos to position of w
				set winSize to size of w
				set windowLines to windowLines & appName & tab & winTitle & tab & (item 1 of winPos as integer) & tab & (item 2 of winPos as integer) & tab & (item 1 of winSize as integer) & tab & (item 2 of winSize as integer) & linefeed
			end repeat
		end try
	end repeat
end tell
return windowLines
`
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return ComputerWindowInfo{}, fmt.Errorf("window lookup failed: %w", err)
	}
	windows := parseMacWindowRows(string(out))
	for _, window := range windows {
		if !windowMatches(window, targetApp, titleContains) {
			continue
		}
		info, err := robotGoComputerBackend{}.ScreenInfo()
		if err != nil {
			return ComputerWindowInfo{}, err
		}
		return scaleAndClampWindow(window, info), nil
	}
	return ComputerWindowInfo{}, fmt.Errorf("window not found for app=%q title_contains=%q", targetApp, titleContains)
}

func (robotGoComputerBackend) Execute(ctx context.Context, actions []ComputerAction) error {
	for _, action := range actions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if action.HasX && action.HasY && action.Type != "move" {
			robotgo.Move(action.X, action.Y)
		}
		switch action.Type {
		case "move":
			robotgo.Move(action.X, action.Y)
		case "click":
			if err := robotgo.Click(action.Button, action.Double); err != nil {
				return err
			}
		case "right_click":
			if err := robotgo.Click("right"); err != nil {
				return err
			}
		case "double_click":
			if err := robotgo.Click(action.Button, true); err != nil {
				return err
			}
		case "key":
			if err := robotgo.KeyTap(action.Key, keyTapArgs(action.Modifiers)...); err != nil {
				return err
			}
		case "hotkey":
			if err := robotgo.KeyTap(action.Key, keyTapArgs(action.Modifiers)...); err != nil {
				return err
			}
		case "key_down":
			if err := robotgo.KeyToggle(action.Key, keyToggleArgs("down", action.Modifiers)...); err != nil {
				return err
			}
		case "key_up":
			if err := robotgo.KeyToggle(action.Key, keyToggleArgs("up", action.Modifiers)...); err != nil {
				return err
			}
		case "type":
			robotgo.Type(action.Text)
		case "wait":
			time.Sleep(time.Duration(action.MS) * time.Millisecond)
		case "scroll":
			robotgo.Scroll(action.DX, action.DY)
		}
	}
	return nil
}

func parseMacWindowRows(raw string) []ComputerWindowInfo {
	var windows []ComputerWindowInfo
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		x, xErr := strconv.Atoi(strings.TrimSpace(parts[2]))
		y, yErr := strconv.Atoi(strings.TrimSpace(parts[3]))
		w, wErr := strconv.Atoi(strings.TrimSpace(parts[4]))
		h, hErr := strconv.Atoi(strings.TrimSpace(parts[5]))
		if xErr != nil || yErr != nil || wErr != nil || hErr != nil || w <= 0 || h <= 0 {
			continue
		}
		windows = append(windows, ComputerWindowInfo{
			App:    strings.TrimSpace(parts[0]),
			Title:  strings.TrimSpace(parts[1]),
			X:      x,
			Y:      y,
			Width:  w,
			Height: h,
		})
	}
	return windows
}

func windowMatches(window ComputerWindowInfo, targetApp, titleContains string) bool {
	if targetApp != "" {
		app := strings.ToLower(window.App)
		target := strings.ToLower(targetApp)
		if app != target && !strings.Contains(app, target) && !strings.Contains(target, app) {
			return false
		}
	}
	if titleContains != "" && !strings.Contains(strings.ToLower(window.Title), strings.ToLower(titleContains)) {
		return false
	}
	return true
}

func scaleAndClampWindow(window ComputerWindowInfo, info ComputerScreenInfo) ComputerWindowInfo {
	scale := info.Scale
	if scale <= 0 {
		scale = 1
	}
	window.X = int(math.Round(float64(window.X) * scale))
	window.Y = int(math.Round(float64(window.Y) * scale))
	window.Width = int(math.Round(float64(window.Width) * scale))
	window.Height = int(math.Round(float64(window.Height) * scale))
	if window.X < 0 {
		window.Width += window.X
		window.X = 0
	}
	if window.Y < 0 {
		window.Height += window.Y
		window.Y = 0
	}
	if info.Width > 0 && window.X+window.Width > info.Width {
		window.Width = info.Width - window.X
	}
	if info.Height > 0 && window.Y+window.Height > info.Height {
		window.Height = info.Height - window.Y
	}
	if window.Width < 1 {
		window.Width = 1
	}
	if window.Height < 1 {
		window.Height = 1
	}
	return window
}

func keyTapArgs(modifiers []string) []interface{} {
	if len(modifiers) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(modifiers))
	for _, modifier := range modifiers {
		out = append(out, modifier)
	}
	return out
}

func keyToggleArgs(direction string, modifiers []string) []interface{} {
	out := []interface{}{direction}
	for _, modifier := range modifiers {
		out = append(out, modifier)
	}
	return out
}

func captureWithScreencapture(ctx context.Context, opts ComputerScreenshotOptions) (image.Image, error) {
	tmpDir, err := os.MkdirTemp("", "cheng-computer-screen-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "screen.png")
	args := []string{"-x"}
	if opts.Width > 0 && opts.Height > 0 {
		args = append(args, "-R", fmt.Sprintf("%d,%d,%d,%d", opts.X, opts.Y, opts.Width, opts.Height))
	}
	args = append(args, outPath)
	cmd := exec.CommandContext(ctx, "screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return loadImage(outPath)
}
