package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	_ "golang.org/x/image/webp"
	_ "modernc.org/sqlite"
)

type Config struct {
	BaseURL string
	Model   string
	APIKey  string
}

type Message struct {
	ID          string
	ThreadID    string
	Role        string
	Text        string
	Images      []string
	CreatedAt   time.Time
	Attachments []string
}

type ChatApp struct {
	win *app.Window
	th  *material.Theme
	ops op.Ops

	cfg Config

	input       widget.Editor
	imagePath   widget.Editor
	baseURL     widget.Editor
	model       widget.Editor
	sendBtn     widget.Clickable
	addImgBtn   widget.Clickable
	clearBtn    widget.Clickable
	clearImgBtn widget.Clickable
	closeBtn    widget.Clickable
	zoomInBtn   widget.Clickable
	zoomOutBtn  widget.Clickable
	actualBtn   widget.Clickable
	fitBtn      widget.Clickable
	scrollList  widget.List

	mu            sync.Mutex
	messages      []Message
	pendingImgs   []string
	status        string
	loading       bool
	enlarged      string
	historyPath   string
	preview       previewState
	imgCache      map[string]image.Image
	imgOps        map[string]paint.ImageOp
	imageButtons  map[string]*widget.Clickable
	removeButtons map[string]*widget.Clickable
}

type previewState struct {
	tag      struct{}
	path     string
	zoom     float32
	mode     string
	offset   f32.Point
	dragging bool
	lastPos  f32.Point
}

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("Chengcheng Chat"), app.Size(unit.Dp(980), unit.Dp(760)))
		if err := NewChatApp(w).Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func NewChatApp(w *app.Window) *ChatApp {
	cfg := loadConfig()
	a := &ChatApp{
		win:           w,
		th:            material.NewTheme(),
		cfg:           cfg,
		historyPath:   historyPath(),
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
	} else if msgs, err := loadHistory(a.historyPath); err == nil && len(msgs) > 0 {
		a.messages = msgs
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
		case app.DestroyEvent:
			return ev.Err
		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, ev)
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
		a.status = "Conversation cleared"
		a.mu.Unlock()
		a.saveHistoryAllowEmpty()
		a.win.Invalidate()
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
	a.messages = append(a.messages, Message{Role: "user", Text: text, Attachments: imgs, CreatedAt: time.Now()})
	a.loading = true
	a.status = "Sending..."
	snapshot := append([]Message(nil), a.messages...)
	cfg := a.cfg
	a.mu.Unlock()
	a.saveHistory()
	a.win.Invalidate()

	go func() {
		reply, err := callResponses(context.Background(), cfg, snapshot)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.loading = false
		if err != nil {
			a.status = "Error: " + err.Error()
		} else {
			a.messages = append(a.messages, reply)
			a.status = "Ready"
		}
		a.saveHistoryLocked(false)
		a.win.Invalidate()
	}()
}

func (a *ChatApp) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, rgb(250, 251, 253))
	inset := layout.UniformInset(unit.Dp(16))
	dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.header),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx)
			}),
			layout.Flexed(1, a.messagesView),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx)
			}),
			layout.Rigid(a.composer),
		)
	})
	a.previewOverlay(gtx)
	return dims
}

func (a *ChatApp) header(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	status := a.status
	loading := a.loading
	a.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, material.Editor(a.th, &a.baseURL, "Base URL").Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
				layout.Flexed(.45, material.Editor(a.th, &a.model, "Model").Layout),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := status
			if loading {
				label = "Thinking..."
			}
			txt := material.Body2(a.th, label)
			txt.Color = rgb(88, 96, 105)
			return txt.Layout(gtx)
		}),
	)
}

func (a *ChatApp) messagesView(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	msgs := append([]Message(nil), a.messages...)
	a.mu.Unlock()

	if len(msgs) == 0 {
		return centerText(gtx, a.th, "Single conversation. Add text, optionally attach image paths, then send.")
	}

	return material.List(a.th, &a.scrollList).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		return a.messageBubble(gtx, msgs[i])
	})
}

func (a *ChatApp) messageBubble(gtx layout.Context, msg Message) layout.Dimensions {
	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 238}
	if msg.Role == "assistant" {
		bg = color.NRGBA{R: 236, G: 243, B: 255, A: 238}
	}
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rr := clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Max}, SE: 8, SW: 8, NE: 8, NW: 8}
		defer rr.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, bg)
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					title := material.Body2(a.th, strings.ToUpper(msg.Role))
					title.Color = rgb(84, 92, 100)
					return title.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					body := msg.Text
					if body == "" {
						body = "(image only)"
					}
					lbl := material.Body1(a.th, body)
					lbl.Alignment = text.Start
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					paths := append([]string{}, msg.Attachments...)
					paths = append(paths, msg.Images...)
					if len(paths) == 0 {
						return layout.Dimensions{}
					}
					return a.imageStrip(gtx, paths, unit.Dp(88))
				}),
			)
		})
	})
}

func (a *ChatApp) imageStrip(gtx layout.Context, paths []string, sizeDp unit.Dp) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, imageChildren(a, paths, sizeDp)...)
}

func imageChildren(a *ChatApp, paths []string, sizeDp unit.Dp) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(paths))
	for _, path := range paths {
		p := path
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				img, imgOp, err := a.cachedImageOp(p)
				if err != nil || img == nil {
					txt := material.Body2(a.th, filepath.Base(p))
					return txt.Layout(gtx)
				}
				size := gtx.Dp(sizeDp)
				gtx.Constraints.Min = image.Pt(size, size)
				gtx.Constraints.Max = image.Pt(size, size)
				btn := a.imageButton(p)
				for btn.Clicked(gtx) {
					a.mu.Lock()
					a.enlarged = p
					if a.preview.path != p {
						a.preview = previewState{path: p, zoom: 1, mode: "fit"}
					}
					a.mu.Unlock()
					a.win.Invalidate()
				}
				return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
					return widget.Image{Src: imgOp, Fit: widget.Contain}.Layout(gtx)
				})
			})
		}))
	}
	return children
}

func (a *ChatApp) cachedImageOp(path string) (image.Image, paint.ImageOp, error) {
	a.mu.Lock()
	img := a.imgCache[path]
	imgOp, ok := a.imgOps[path]
	a.mu.Unlock()
	if img != nil && ok {
		return img, imgOp, nil
	}

	loaded, err := loadImage(path)
	if err != nil {
		return nil, paint.ImageOp{}, err
	}
	op := paint.NewImageOp(loaded)

	a.mu.Lock()
	a.imgCache[path] = loaded
	a.imgOps[path] = op
	a.mu.Unlock()
	return loaded, op, nil
}

func (a *ChatApp) imageButton(path string) *widget.Clickable {
	a.mu.Lock()
	defer a.mu.Unlock()
	btn := a.imageButtons[path]
	if btn == nil {
		btn = new(widget.Clickable)
		a.imageButtons[path] = btn
	}
	return btn
}

func (a *ChatApp) removeButton(path string) *widget.Clickable {
	a.mu.Lock()
	defer a.mu.Unlock()
	btn := a.removeButtons[path]
	if btn == nil {
		btn = new(widget.Clickable)
		a.removeButtons[path] = btn
	}
	return btn
}

func (a *ChatApp) removePendingImage(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := a.pendingImgs[:0]
	for _, img := range a.pendingImgs {
		if img != path {
			next = append(next, img)
		}
	}
	a.pendingImgs = next
	a.status = fmt.Sprintf("Attached %d image(s)", len(a.pendingImgs))
}

func (a *ChatApp) pendingImageStrip(gtx layout.Context, paths []string) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(paths))
	for _, path := range paths {
		p := path
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				img, imgOp, err := a.cachedImageOp(p)
				if err != nil || img == nil {
					return material.Body2(a.th, filepath.Base(p)).Layout(gtx)
				}
				remove := a.removeButton(p)
				for remove.Clicked(gtx) {
					a.removePendingImage(p)
					a.win.Invalidate()
				}
				open := a.imageButton("pending:" + p)
				for open.Clicked(gtx) {
					a.mu.Lock()
					a.enlarged = p
					if a.preview.path != p {
						a.preview = previewState{path: p, zoom: 1, mode: "fit"}
					}
					a.mu.Unlock()
					a.win.Invalidate()
				}
				size := gtx.Dp(unit.Dp(78))
				gtx.Constraints.Min = image.Pt(size, size)
				gtx.Constraints.Max = image.Pt(size, size)
				return layout.Stack{Alignment: layout.NE}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, open, func(gtx layout.Context) layout.Dimensions {
							rr := clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Max}, SE: 8, SW: 8, NE: 8, NW: 8}
							defer rr.Push(gtx.Ops).Pop()
							paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 245})
							return layout.UniformInset(unit.Dp(3)).Layout(gtx, widget.Image{Src: imgOp, Fit: widget.Contain}.Layout)
						})
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(-6), Right: unit.Dp(-6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.roundCloseButton(gtx, remove)
						})
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (a *ChatApp) roundCloseButton(gtx layout.Context, btn *widget.Clickable) layout.Dimensions {
	size := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, color.NRGBA{R: 42, G: 48, B: 60, A: 230})
		lbl := material.Body1(a.th, "×")
		lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		lbl.Alignment = text.Middle
		return layout.Center.Layout(gtx, lbl.Layout)
	})
}

func (a *ChatApp) previewOverlay(gtx layout.Context) {
	for a.closeBtn.Clicked(gtx) {
		a.mu.Lock()
		a.enlarged = ""
		a.preview = previewState{}
		a.mu.Unlock()
		a.win.Invalidate()
	}
	a.mu.Lock()
	path := a.enlarged
	if path != "" && a.preview.path != path {
		a.preview = previewState{path: path, zoom: 1, mode: "fit"}
	}
	a.mu.Unlock()
	if path == "" {
		return
	}
	img, imgOp, err := a.cachedImageOp(path)
	if err != nil {
		a.setStatus("Preview error: " + err.Error())
		return
	}

	a.handlePreviewControls(gtx)
	viewport := gtx.Constraints.Max
	defer clip.Rect{Max: viewport}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 176})
	area := clip.Rect{Max: viewport}.Push(gtx.Ops)
	event.Op(gtx.Ops, &a.preview.tag)
	area.Pop()
	pointer.CursorGrab.Add(gtx.Ops)

	a.layoutZoomableImage(gtx, img, imgOp)
	a.previewToolbar(gtx, filepath.Base(path))
}

func (a *ChatApp) previewToolbar(gtx layout.Context, filename string) layout.Dimensions {
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(34)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(material.Button(a.th, &a.zoomOutBtn, "Zoom Out").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Rigid(material.Button(a.th, &a.zoomInBtn, "Zoom In").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Rigid(material.Button(a.th, &a.actualBtn, "1:1").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Rigid(material.Button(a.th, &a.fitBtn, "Fit").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Rigid(material.Button(a.th, &a.closeBtn, "Close").Layout),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(420)))
							txt := material.Body2(a.th, filename)
							txt.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 245}
							return txt.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (a *ChatApp) handlePreviewControls(gtx layout.Context) {
	for a.zoomInBtn.Clicked(gtx) {
		a.mu.Lock()
		a.preview.zoom = clampFloat32(a.preview.zoom*1.25, 0.2, 8)
		a.preview.mode = "custom"
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.zoomOutBtn.Clicked(gtx) {
		a.mu.Lock()
		a.preview.zoom = clampFloat32(a.preview.zoom/1.25, 0.2, 8)
		a.preview.mode = "custom"
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.actualBtn.Clicked(gtx) {
		a.mu.Lock()
		a.preview.zoom = 1
		a.preview.mode = "actual"
		a.preview.offset = f32.Point{}
		a.mu.Unlock()
		a.win.Invalidate()
	}
	for a.fitBtn.Clicked(gtx) {
		a.mu.Lock()
		a.preview.zoom = 1
		a.preview.mode = "fit"
		a.preview.offset = f32.Point{}
		a.mu.Unlock()
		a.win.Invalidate()
	}
	a.updatePreviewDrag(gtx)
}

func (a *ChatApp) updatePreviewDrag(gtx layout.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	if a.preview.zoom <= 0 {
		a.preview.zoom = 1
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &a.preview.tag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if e.Buttons == pointer.ButtonPrimary || e.Source == pointer.Touch {
				a.preview.dragging = true
				a.preview.lastPos = e.Position
				gtx.Execute(pointer.GrabCmd{Tag: &a.preview.tag, ID: e.PointerID})
			}
		case pointer.Drag:
			if a.preview.dragging {
				d := e.Position.Sub(a.preview.lastPos)
				a.preview.offset = a.preview.offset.Add(d)
				a.preview.lastPos = e.Position
				changed = true
			}
		case pointer.Release, pointer.Cancel:
			a.preview.dragging = false
		}
	}
	if changed {
		a.win.Invalidate()
	}
}

func (a *ChatApp) layoutZoomableImage(gtx layout.Context, img image.Image, imgOp paint.ImageOp) layout.Dimensions {
	a.mu.Lock()
	zoom := a.preview.zoom
	offset := a.preview.offset
	mode := a.preview.mode
	a.mu.Unlock()
	if zoom <= 0 {
		zoom = 1
	}
	viewport := gtx.Constraints.Max
	src := img.Bounds().Size()
	if src.X <= 0 || src.Y <= 0 || viewport.X <= 0 || viewport.Y <= 0 {
		return layout.Dimensions{Size: viewport}
	}
	base := minFloat32(float32(viewport.X)/float32(src.X), float32(viewport.Y)/float32(src.Y))
	if base > 1 {
		base = 1
	}
	scale := base * zoom
	if mode == "actual" {
		scale = zoom
	}
	drawSize := image.Pt(maxInt(1, int(float32(src.X)*scale)), maxInt(1, int(float32(src.Y)*scale)))
	center := f32.Pt(float32(viewport.X-drawSize.X)/2, float32(viewport.Y-drawSize.Y)/2)
	pos := image.Pt(int(center.X+offset.X), int(center.Y+offset.Y))

	defer clip.Rect{Max: viewport}.Push(gtx.Ops).Pop()
	defer op.Offset(pos).Push(gtx.Ops).Pop()
	gtx.Constraints.Min = drawSize
	gtx.Constraints.Max = drawSize
	widget.Image{
		Src:   imgOp,
		Fit:   widget.Fill,
		Scale: 1 / gtx.Metric.PxPerDp / scale,
	}.Layout(gtx)
	return layout.Dimensions{Size: viewport}
}

func (a *ChatApp) composer(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	loading := a.loading
	attached := len(a.pendingImgs)
	pending := append([]string(nil), a.pendingImgs...)
	a.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(a.th, &a.input, "Message")
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(92))
			if max := gtx.Dp(unit.Dp(170)); gtx.Constraints.Max.Y > max {
				gtx.Constraints.Max.Y = max
			}
			return ed.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(pending) == 0 {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.pendingImageStrip(gtx, pending)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, material.Editor(a.th, &a.imagePath, "Optional image paths, comma separated").Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
				layout.Rigid(material.Button(a.th, &a.addImgBtn, fmt.Sprintf("Add Image (%d)", attached)).Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if attached == 0 {
						return layout.Dimensions{}
					}
					return material.Button(a.th, &a.clearImgBtn, "Clear Images").Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if attached == 0 {
						return layout.Dimensions{}
					}
					return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
				}),
				layout.Rigid(material.Button(a.th, &a.clearBtn, "Clear").Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Send"
					if loading {
						label = "Sending"
					}
					return material.Button(a.th, &a.sendBtn, label).Layout(gtx)
				}),
			)
		}),
	)
}

func callResponses(ctx context.Context, cfg Config, history []Message) (Message, error) {
	if cfg.APIKey == "" {
		return Message{}, errors.New("missing OPENAI_API_KEY in ~/.codex/auth.json or environment")
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return Message{}, errors.New("base URL and model are required")
	}

	input, err := buildInput(history)
	if err != nil {
		return Message{}, err
	}
	body := map[string]any{
		"model":  cfg.Model,
		"input":  input,
		"stream": true,
		"tools": []map[string]any{
			{"type": "image_generation"},
		},
	}
	if wantsImage(history) {
		body["tool_choice"] = map[string]any{"type": "image_generation"}
	}
	data, _ := json.Marshal(body)

	url := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasSuffix(url, "/v1") {
		url += "/v1"
	}
	url += "/responses"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("api %s: %s", resp.Status, trimForStatus(raw))
	}

	text, images, err := parseResponseStream(resp.Body)
	if err != nil {
		return Message{}, err
	}
	return Message{Role: "assistant", Text: text, Images: images, CreatedAt: time.Now()}, nil
}

func buildInput(history []Message) ([]map[string]any, error) {
	mem, _ := os.ReadFile("memories.md")
	char, _ := os.ReadFile("character.md")
	systemText := strings.TrimSpace("# character.md\n" + string(char) + "\n\n# memories.md\n" + string(mem))
	input := []map[string]any{{
		"role": "system",
		"content": []map[string]any{{
			"type": "input_text",
			"text": systemText,
		}},
	}}
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}
	for _, m := range history[start:] {
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
		for _, img := range m.Attachments {
			dataURL, err := fileDataURL(img)
			if err != nil {
				return nil, err
			}
			parts = append(parts, map[string]any{"type": "input_image", "image_url": dataURL})
		}
		if len(parts) == 0 {
			continue
		}
		input = append(input, map[string]any{"role": role, "content": parts})
	}
	return input, nil
}

func parseResponse(raw []byte) (string, []string, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", nil, err
	}
	var texts []string
	var images []string
	walkJSON(root, func(m map[string]any) {
		if t, _ := m["type"].(string); t == "output_text" || t == "text" {
			if s, _ := m["text"].(string); s != "" {
				texts = append(texts, s)
			}
		}
		for _, key := range []string{"result", "b64_json", "image_base64"} {
			if s, _ := m[key].(string); looksBase64Image(s) {
				if path, err := saveBase64Image(s); err == nil {
					images = append(images, path)
				}
			}
		}
		for _, key := range []string{"image_url", "url"} {
			if s, _ := m[key].(string); strings.HasPrefix(s, "data:image/") {
				if path, err := saveDataURL(s); err == nil {
					images = append(images, path)
				}
			}
		}
	})
	if len(texts) == 0 {
		if s := extractOutputText(root); s != "" {
			texts = append(texts, s)
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n\n")), dedupeStrings(images), nil
}

func parseResponseStream(r io.Reader) (string, []string, error) {
	br := bufio.NewReaderSize(r, 1024*1024)
	var eventName string
	var data strings.Builder
	var lastText string
	var lastImages []string
	var sawCompleted bool

	flush := func() error {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" || payload == "[DONE]" {
			eventName = ""
			return nil
		}

		var evt map[string]any
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			eventName = ""
			return nil
		}
		typ, _ := evt["type"].(string)
		if typ == "" {
			typ = eventName
		}
		if typ == "response.failed" || typ == "response.incomplete" {
			if errObj, ok := evt["error"]; ok {
				return fmt.Errorf("api stream failed: %v", errObj)
			}
			return fmt.Errorf("api stream failed: %s", typ)
		}

		if typ == "response.output_item.done" || typ == "response.completed" {
			text, images, err := parseResponse([]byte(payload))
			if err == nil {
				if text != "" {
					lastText = text
				}
				if len(images) > 0 {
					lastImages = append(lastImages, images...)
				}
			}
		}
		if typ == "response.completed" {
			sawCompleted = true
			if response, ok := evt["response"]; ok {
				raw, _ := json.Marshal(response)
				text, images, err := parseResponse(raw)
				if err == nil {
					if text != "" {
						lastText = text
					}
					if len(images) > 0 {
						lastImages = append(lastImages, images...)
					}
				}
			}
		}
		eventName = ""
		return nil
	}

	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			switch {
			case line == "":
				if err := flush(); err != nil {
					return "", nil, err
				}
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if data.Len() > 0 {
					if err := flush(); err != nil {
						return "", nil, err
					}
				}
				break
			}
			return "", nil, err
		}
	}

	if !sawCompleted && lastText == "" && len(lastImages) == 0 {
		return "", nil, errors.New("stream ended without a completed response")
	}
	return lastText, dedupeStrings(lastImages), nil
}

func walkJSON(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, child := range x {
			walkJSON(child, fn)
		}
	case []any:
		for _, child := range x {
			walkJSON(child, fn)
		}
	}
}

func extractOutputText(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if s, _ := m["output_text"].(string); s != "" {
		return s
	}
	return ""
}

func saveBase64Image(s string) (string, error) {
	ext := ".png"
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	if len(data) > 3 && data[0] == 0xff && data[1] == 0xd8 {
		ext = ".jpg"
	} else if len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		ext = ".webp"
	}
	sum := sha1.Sum(data)
	path := filepath.Join("app_outputs", "images", "response_"+hex.EncodeToString(sum[:8])+ext)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0644)
}

func saveDataURL(s string) (string, error) {
	idx := strings.Index(s, ",")
	if idx < 0 {
		return "", errors.New("bad data URL")
	}
	meta := s[:idx]
	ext := ".png"
	if strings.Contains(meta, "jpeg") || strings.Contains(meta, "jpg") {
		ext = ".jpg"
	} else if strings.Contains(meta, "webp") {
		ext = ".webp"
	}
	data, err := base64.StdEncoding.DecodeString(s[idx+1:])
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data)
	path := filepath.Join("app_outputs", "images", "response_"+hex.EncodeToString(sum[:8])+ext)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0644)
}

func loadConfig() Config {
	home, _ := os.UserHomeDir()
	cfg := Config{
		BaseURL: "https://ai.martin98.top/v1",
		Model:   "gpt-5.5",
		APIKey:  os.Getenv("OPENAI_API_KEY"),
	}
	if b, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml")); err == nil {
		if s := tomlString(string(b), "base_url"); s != "" {
			cfg.BaseURL = s
		}
		if s := tomlString(string(b), "model"); s != "" {
			cfg.Model = s
		}
	}
	if cfg.APIKey == "" {
		if b, err := os.ReadFile(filepath.Join(home, ".codex", "auth.json")); err == nil {
			var auth map[string]string
			if json.Unmarshal(b, &auth) == nil {
				cfg.APIKey = auth["OPENAI_API_KEY"]
			}
		}
	}
	return cfg
}

type historyStore struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

const (
	defaultUserID   = "local-user"
	defaultAgentID  = "assistant"
	defaultThreadID = "default-thread"
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
		SELECT id, thread_id, role, content, created_at
		FROM messages
		WHERE thread_id = ? AND deleted_at IS NULL
		ORDER BY created_at, seq
	`, defaultThreadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Text, &created); err != nil {
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
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	now := time.Now().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT OR IGNORE INTO users(id, display_name, created_at) VALUES(?, ?, ?)`, defaultUserID, "Local User", now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO agents(id, display_name, provider, created_at) VALUES(?, ?, ?, ?)`, defaultAgentID, "Assistant", "OpenAI-compatible", now); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO threads(id, title, user_id, default_agent_id, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`, defaultThreadID, "Default conversation", defaultUserID, defaultAgentID, now, now)
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
		`, m.ID, m.ThreadID, i+1, m.Role, actorType, actorID, m.Text, created, now); err != nil {
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

func tomlString(src, key string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]+)"`)
	m := re.FindStringSubmatch(src)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func fileDataURL(path string) (string, error) {
	path, err := prepareImageAttachment(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if typ == "" {
		typ = http.DetectContentType(data)
	}
	if !strings.HasPrefix(typ, "image/") {
		return "", fmt.Errorf("%s is not an image", path)
	}
	return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func looksBase64Image(s string) bool {
	if len(s) < 64 || strings.Contains(s, " ") || strings.Contains(s, "\n") {
		return false
	}
	return strings.HasPrefix(s, "iVBOR") || strings.HasPrefix(s, "/9j/") || strings.HasPrefix(s, "UklGR")
}

func parseImagePaths(src string) ([]string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(src, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	var paths []string
	for _, field := range fields {
		path := strings.Trim(strings.TrimSpace(field), `"'`)
		if path == "" {
			continue
		}
		path = expandPath(path)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("%s not found", path)
		}
		prepared, err := prepareImageAttachment(path)
		if err != nil {
			return nil, err
		}
		paths = append(paths, prepared)
	}
	return paths, nil
}

func validateImage(path string) error {
	_, err := prepareImageAttachment(path)
	return err
}

func prepareImageAttachment(path string) (string, error) {
	path = expandPath(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%s not found", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".svg" || ext == ".svgz" {
		converted, err := convertSVGToPNG(path)
		if err != nil {
			return "", err
		}
		return flattenImageToWhitePNG(converted)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	typ := mime.TypeByExtension(ext)
	if typ == "" {
		typ = http.DetectContentType(data)
	}
	if !strings.HasPrefix(typ, "image/") {
		return "", fmt.Errorf("%s is not an image", path)
	}
	if typ == "image/svg+xml" {
		converted, err := convertSVGToPNG(path)
		if err != nil {
			return "", err
		}
		return flattenImageToWhitePNG(converted)
	}
	return flattenImageToWhitePNG(path)
}

func convertSVGToPNG(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(append([]byte(path), data...))
	outDir := filepath.Join("app_outputs", "converted")
	outPath := filepath.Join(outDir, "svg_"+hex.EncodeToString(sum[:8])+".png")
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("SVG attachments must be converted to PNG first on this platform")
	}
	tmp, err := os.MkdirTemp("", "cheng-svg-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command("qlmanage", "-t", "-s", "1600", "-o", tmp, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to convert SVG with qlmanage: %s", strings.TrimSpace(string(out)))
	}
	produced := filepath.Join(tmp, filepath.Base(path)+".png")
	if _, err := os.Stat(produced); err != nil {
		return "", fmt.Errorf("failed to convert SVG: PNG thumbnail was not produced")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	data, err = os.ReadFile(produced)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "", err
	}
	return outPath, nil
}

func flattenImageToWhitePNG(path string) (string, error) {
	path = expandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(append([]byte(path), data...))
	outDir := filepath.Join("app_outputs", "prepared")
	outPath := filepath.Join(outDir, "image_"+hex.EncodeToString(sum[:8])+".png")
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}
	src, err := loadImage(path)
	if err != nil {
		return "", err
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, dst); err != nil {
		return "", err
	}
	return outPath, nil
}

func pickImageFile() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e", `POSIX path of (choose file of type {"public.image"} with prompt "Choose an image")`).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Filter = 'Images|*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp|All files|*.*'; if ($d.ShowDialog() -eq 'OK') { $d.FileName }`
		out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		out, err := exec.Command("zenity", "--file-selection", "--title=Choose an image", "--file-filter=Images | *.png *.jpg *.jpeg *.gif *.webp *.bmp").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
}

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

func clampFloat32(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func trimForStatus(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 700 {
		return s[:700] + "..."
	}
	return s
}

func centerText(gtx layout.Context, th *material.Theme, s string) layout.Dimensions {
	return layout.Center.Layout(gtx, material.Body1(th, s).Layout)
}

func (a *ChatApp) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.win.Invalidate()
}

func rgb(r, g, b byte) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}
