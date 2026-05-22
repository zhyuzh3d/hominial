package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func wantsImage(history []Message) bool {
	if len(history) == 0 {
		return false
	}
	text := strings.ToLower(history[len(history)-1].Text)
	triggers := []string{
		"画", "绘制", "生成图片", "做一张图", "出图", "画图", "图片吧",
		"draw", "generate an image", "create an image", "make an image", "illustration",
	}
	for _, trigger := range triggers {
		if strings.Contains(text, trigger) {
			return true
		}
	}
	return false
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func appOutputPath(parts ...string) (string, error) {
	local := filepath.Join(append([]string{"app_outputs"}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(local), 0755); err == nil {
		return local, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("output directory unavailable: %w", err)
		}
	}
	path := filepath.Join(append([]string{base, "Hominial-Elli", "app_outputs"}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, nil
}

func trimForStatus(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 700 {
		return s[:700] + "..."
	}
	return s
}
