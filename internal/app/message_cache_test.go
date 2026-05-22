package app

import (
	"strconv"
	"testing"

	"gioui.org/widget"
)

func TestNormalizeAppearanceSettingsDefaultsMessageCacheLimit(t *testing.T) {
	settings := normalizeAppearanceSettings(AppearanceSettings{})
	if settings.MessageCacheLimit != defaultMessageCacheLimit {
		t.Fatalf("expected default cache limit %d, got %d", defaultMessageCacheLimit, settings.MessageCacheLimit)
	}

	settings = normalizeAppearanceSettings(AppearanceSettings{MessageCacheLimit: 1})
	if settings.MessageCacheLimit != defaultWindowSize {
		t.Fatalf("expected minimum cache limit %d, got %d", defaultWindowSize, settings.MessageCacheLimit)
	}
}

func TestTrimMessageCacheKeepsNewestAndCleansUIState(t *testing.T) {
	messages := make([]Message, 0, defaultWindowSize+2)
	messages = append(messages, Message{ID: "m1", Images: []string{"old.png"}})
	for i := 2; i <= defaultWindowSize+1; i++ {
		messages = append(messages, Message{ID: "m" + strconv.Itoa(i)})
	}
	messages = append(messages, Message{ID: "m-new", Images: []string{"new.png"}})

	a := &ChatApp{
		settings: UISettings{Appearance: AppearanceSettings{MessageCacheLimit: defaultWindowSize}},
		messages: messages,
		textEditors: map[string]*widget.Editor{
			"message:m1":    {},
			"time:m1":       {},
			"message:m-new": {},
		},
		imageButtons: map[string]*widget.Clickable{
			"old.png": {},
			"new.png": {},
		},
		removeButtons: map[string]*widget.Clickable{
			"old.png": {},
			"new.png": {},
		},
	}

	a.trimMessageCacheLocked(true)

	if len(a.messages) != defaultWindowSize || a.messages[0].ID != "m3" || a.messages[len(a.messages)-1].ID != "m-new" {
		t.Fatalf("unexpected retained messages: %#v", a.messages)
	}
	if !a.hasOlder {
		t.Fatal("expected hasOlder after trimming oldest messages")
	}
	if _, ok := a.textEditors["message:m1"]; ok {
		t.Fatal("expected old message editor to be removed")
	}
	if _, ok := a.textEditors["message:m-new"]; !ok {
		t.Fatal("expected retained message editor to stay cached")
	}
	if _, ok := a.imageButtons["old.png"]; ok {
		t.Fatal("expected old image button to be removed")
	}
	if _, ok := a.imageButtons["new.png"]; !ok {
		t.Fatal("expected retained image button to stay cached")
	}
}
