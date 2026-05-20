package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type ChatApp struct {
	win *gioapp.Window
	th  *material.Theme
	ops op.Ops

	cfg Config

	input        widget.Editor
	imagePath    widget.Editor
	baseURL      widget.Editor
	model        widget.Editor
	sendBtn      widget.Clickable
	addImgBtn    widget.Clickable
	clearBtn     widget.Clickable
	loadOlderBtn widget.Clickable
	clearImgBtn  widget.Clickable
	closeBtn     widget.Clickable
	zoomInBtn    widget.Clickable
	zoomOutBtn   widget.Clickable
	actualBtn    widget.Clickable
	fitBtn       widget.Clickable
	scrollList   widget.List

	mu            sync.Mutex
	messages      []Message
	pendingImgs   []string
	status        string
	loading       bool
	enlarged      string
	historyPath   string
	peConfig      PEConfig
	hasOlder      bool
	preview       previewState
	imgCache      map[string]image.Image
	imgOps        map[string]paint.ImageOp
	imageButtons  map[string]*widget.Clickable
	removeButtons map[string]*widget.Clickable
}

func Run() {
	go func() {
		w := new(gioapp.Window)
		w.Option(gioapp.Title("Chengcheng Chat"), gioapp.Size(unit.Dp(980), unit.Dp(760)))
		if err := NewChatApp(w).Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}()
	gioapp.Main()
}

func NewChatApp(w *gioapp.Window) *ChatApp {
	cfg := loadConfig()
	a := &ChatApp{
		win:           w,
		th:            material.NewTheme(),
		cfg:           cfg,
		historyPath:   historyPath(),
		peConfig:      defaultPEConfig(),
		imgCache:      make(map[string]image.Image),
		imgOps:        make(map[string]paint.ImageOp),
		imageButtons:  make(map[string]*widget.Clickable),
		removeButtons: make(map[string]*widget.Clickable),
		status:        "Ready",
	}
	a.th.Palette.Bg = rgb(247, 249, 252)
	a.th.Palette.Fg = rgb(28, 34, 45)
	a.th.Palette.ContrastBg = rgb(86, 107, 230)
	a.th.Palette.ContrastFg = rgb(255, 255, 255)
	if err := initHistoryDB(a.historyPath); err != nil {
		a.status = "History DB error: " + err.Error()
	} else if err := migrateJSONHistory(a.historyPath); err != nil {
		a.status = "History migration warning: " + err.Error()
	} else if msgs, hasOlder, err := loadRecentMessages(a.historyPath, defaultThreadID, a.peConfig.MessageWindowSize); err == nil && len(msgs) > 0 {
		a.messages = msgs
		a.hasOlder = hasOlder
		a.status = fmt.Sprintf("Restored %d message(s)", len(msgs))
	}
	a.input.SingleLine = false
	a.input.Submit = true
	a.imagePath.SingleLine = true
	a.baseURL.SingleLine = true
	a.model.SingleLine = true
	a.baseURL.SetText(cfg.BaseURL)
	a.model.SetText(cfg.Model)
	a.scrollList.Axis = layout.Vertical
	return a
}

func (a *ChatApp) Run() error {
	for {
		ev := a.win.Event()
		switch ev := ev.(type) {
		case gioapp.DestroyEvent:
			return ev.Err
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&a.ops, ev)
			a.handleEvents(gtx)
			a.layout(gtx)
			ev.Frame(gtx.Ops)
		}
	}
}

func (a *ChatApp) handleEvents(gtx layout.Context) {
	for a.addImgBtn.Clicked(gtx) {
		path, err := pickImageFile()
		if err != nil {
			a.setStatus("Add image canceled")
			continue
		}
		if err := validateImage(path); err != nil {
			a.setStatus("Image error: " + err.Error())
			continue
		}
		path, err = prepareImageAttachment(path)
		if err != nil {
			a.setStatus("Image error: " + err.Error())
			continue
		}
		a.mu.Lock()
		a.pendingImgs = append(a.pendingImgs, path)
		a.status = fmt.Sprintf("Attached %d image(s)", len(a.pendingImgs))
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.closeBtn.Clicked(gtx) {
		a.mu.Lock()
		a.enlarged = ""
		a.preview = previewState{}
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.clearBtn.Clicked(gtx) {
		a.mu.Lock()
		a.messages = nil
		a.pendingImgs = nil
		a.enlarged = ""
		a.preview = previewState{}
		a.hasOlder = false
		a.status = "Conversation cleared"
		a.mu.Unlock()
		a.saveHistoryAllowEmpty()
		a.win.Invalidate()
	}
	for a.loadOlderBtn.Clicked(gtx) {
		a.loadOlderMessages()
	}
	for a.clearImgBtn.Clicked(gtx) {
		a.mu.Lock()
		a.pendingImgs = nil
		a.status = "Attachments cleared"
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.sendBtn.Clicked(gtx) {
		a.send()
	}
	for {
		ev, ok := a.input.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := ev.(widget.SubmitEvent); ok {
			if submit.Text != "" {
				a.send()
			}
		}
	}
}

func (a *ChatApp) send() {
	text := strings.TrimSpace(a.input.Text())
	typedImgs, err := parseImagePaths(a.imagePath.Text())
	if err != nil {
		a.setStatus("Image error: " + err.Error())
		return
	}
	a.mu.Lock()
	if a.loading {
		a.mu.Unlock()
		return
	}
	imgs := append([]string(nil), a.pendingImgs...)
	imgs = append(imgs, typedImgs...)
	imgs = dedupeStrings(imgs)
	if text == "" && len(imgs) == 0 {
		a.mu.Unlock()
		return
	}
	a.cfg.BaseURL = strings.TrimSpace(a.baseURL.Text())
	a.cfg.Model = strings.TrimSpace(a.model.Text())
	a.input.SetText("")
	a.imagePath.SetText("")
	a.pendingImgs = nil
	userMsg := Message{Role: "user", Text: text, Attachments: imgs, CreatedAt: time.Now()}
	if err := saveMessageDB(a.historyPath, &userMsg); err != nil {
		a.status = "History save failed: " + err.Error()
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	a.messages = append(a.messages, userMsg)
	a.loading = true
	a.status = "Sending..."
	cfg := a.cfg
	historyPath := a.historyPath
	peCfg := a.peConfig
	a.mu.Unlock()
	a.win.Invalidate()

	go func() {
		result, err := callResponses(context.Background(), cfg, historyPath, peCfg)
		a.mu.Lock()
		a.loading = false
		if err != nil {
			a.status = "Error: " + err.Error()
			a.mu.Unlock()
		} else {
			reply := result.Message
			if err := saveMessageDB(historyPath, &reply); err != nil {
				a.status = "History save failed: " + err.Error()
				a.mu.Unlock()
				a.win.Invalidate()
				return
			}
			a.messages = append(a.messages, reply)
			a.status = "Ready"
			a.mu.Unlock()
			generated, orchErr := runOrchestrator(context.Background(), historyPath, cfg, reply.ID, result.ToolCalls)
			a.mu.Lock()
			for i := range generated {
				if err := saveMessageDB(historyPath, &generated[i]); err == nil {
					a.messages = append(a.messages, generated[i])
				}
			}
			if orchErr != nil {
				a.status = "Tool warning: " + orchErr.Error()
			}
			a.mu.Unlock()
			if err := maybeRefreshShortSummary(context.Background(), cfg, historyPath, peCfg); err != nil {
				a.setStatus("Summary warning: " + err.Error())
			}
		}
		a.win.Invalidate()
	}()
}

func (a *ChatApp) loadOlderMessages() {
	a.mu.Lock()
	if len(a.messages) == 0 || !a.hasOlder {
		a.mu.Unlock()
		return
	}
	beforeSeq := a.messages[0].Seq
	a.mu.Unlock()
	msgs, hasOlder, err := loadOlderMessages(a.historyPath, defaultThreadID, beforeSeq, a.peConfig.MessageWindowSize)
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.status = "Load older failed: " + err.Error()
	} else if len(msgs) == 0 {
		a.hasOlder = false
		a.status = "No older messages"
	} else {
		a.messages = append(msgs, a.messages...)
		a.hasOlder = hasOlder
		a.status = fmt.Sprintf("Loaded %d older message(s)", len(msgs))
	}
	a.win.Invalidate()
}

func (a *ChatApp) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.win.Invalidate()
}
