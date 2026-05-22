package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
