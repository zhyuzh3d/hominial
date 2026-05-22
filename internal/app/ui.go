package app

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

const (
	uiTextMin     = 6
	uiTextMax     = 24
	uiTextHero    = 16
	uiTextBody    = 12
	uiTextMeta    = 9
	uiTextControl = 11
	uiTextSmall   = 10
	uiTextAvatar  = 14
)

var (
	settingsIcon = mustIcon(icons.ActionSettings)
	imageIcon    = mustIcon(icons.ImageImage)
	sendIcon     = mustIcon(icons.ContentSend)
)

func mustIcon(data []byte) *widget.Icon {
	icon, err := widget.NewIcon(data)
	if err != nil {
		panic(fmt.Sprintf("invalid bundled icon: %v", err))
	}
	return icon
}

func uiSp(size float32) unit.Sp {
	if size < uiTextMin {
		size = uiTextMin
	}
	if size > uiTextMax {
		size = uiTextMax
	}
	return unit.Sp(size)
}

func (a *ChatApp) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, rgb(252, 253, 255))
	a.paintAmbient(gtx)
	dims := layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.messagesView(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.header(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.composerOverlay(gtx)
		}),
	)
	a.previewOverlay(gtx)
	a.settingsOverlay(gtx)
	return dims
}

func (a *ChatApp) composerOverlay(gtx layout.Context) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := a.composer(gtx)
	call := macro.Stop()
	y := gtx.Constraints.Max.Y - dims.Size.Y - gtx.Dp(unit.Dp(14))
	if y < 0 {
		y = 0
	}
	trans := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	trans.Pop()
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (a *ChatApp) header(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	loading := a.loading
	a.mu.Unlock()

	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(6), Left: unit.Dp(26), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 11,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 218},
			Border: color.NRGBA{R: 225, G: 231, B: 246, A: 235},
			Shadow: color.NRGBA{R: 104, G: 119, B: 158, A: 14},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(40))
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(18), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.H6(a.th, a.uiText("app_title"))
						title.TextSize = uiSp(uiTextHero)
						title.Color = rgb(18, 24, 38)
						return layout.Center.Layout(gtx, title.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(16)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := a.uiText("online")
						if loading {
							label = a.uiText("thinking")
						}
						return a.onlineStatus(gtx, label)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.settingsPill(gtx)
					}),
				)
			})
		})
	})
}

func (a *ChatApp) messagesView(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	msgs := append([]Message(nil), a.messages...)
	hasOlder := a.hasOlder
	scrollToEnd := a.scrollToEnd
	a.scrollToEnd = false
	a.mu.Unlock()

	a.scrollList.ScrollToEnd = true
	if scrollToEnd {
		a.scrollList.Position.BeforeEnd = false
	}

	if len(msgs) == 0 {
		return centerText(gtx, a.th, a.uiText("empty_chat"))
	}

	count := len(msgs) + 2
	if hasOlder {
		count++
	}
	list := material.List(a.th, &a.scrollList)
	list.AnchorStrategy = material.Overlay
	list.Track.MajorPadding = unit.Dp(0)
	list.Track.MinorPadding = unit.Dp(0)
	list.Track.Color = color.NRGBA{R: 232, G: 237, B: 248, A: 72}
	list.Indicator.MajorMinLen = unit.Dp(38)
	list.Indicator.MinorWidth = unit.Dp(3)
	list.Indicator.CornerRadius = unit.Dp(3)
	list.Indicator.Color = color.NRGBA{R: 116, G: 126, B: 238, A: 116}
	list.Indicator.HoverColor = color.NRGBA{R: 95, G: 107, B: 236, A: 190}
	return list.Layout(gtx, count, func(gtx layout.Context, i int) layout.Dimensions {
		if i == 0 {
			return layout.Spacer{Height: unit.Dp(104)}.Layout(gtx)
		}
		i--
		if hasOlder {
			if i == 0 {
				return layout.Inset{Bottom: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.actionButton(gtx, &a.loadOlderBtn, a.uiText("load_older"))
					})
				})
			}
			i--
		}
		if i >= len(msgs) {
			return layout.Spacer{Height: unit.Dp(190)}.Layout(gtx)
		}
		return a.messageBubble(gtx, msgs[i])
	})
}

func (a *ChatApp) messageBubble(gtx layout.Context, msg Message) layout.Dimensions {
	isUser := msg.Role != "assistant"
	return layout.Inset{Bottom: unit.Dp(24), Left: unit.Dp(18), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, messageRowChildren(a, msg, isUser)...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if msg.CreatedAt.IsZero() {
					return layout.Dimensions{}
				}
				return a.messageTime(gtx, "time:"+msg.ID, msg.CreatedAt.Format("15:04"), isUser)
			}),
		)
	})
}

func messageRowChildren(a *ChatApp, msg Message, isUser bool) []layout.FlexChild {
	avatar := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return a.avatar(gtx, msg.Role)
	})
	gap := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Width: unit.Dp(14)}.Layout(gtx)
	})
	bubble := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		maxW := int(float32(gtx.Constraints.Max.X) * 0.64)
		if maxW < gtx.Dp(unit.Dp(260)) {
			maxW = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(84))
		}
		if maxW > gtx.Dp(unit.Dp(720)) {
			maxW = gtx.Dp(unit.Dp(720))
		}
		if maxW > 0 && gtx.Constraints.Max.X > maxW {
			gtx.Constraints.Max.X = maxW
		}
		return a.bubbleContent(gtx, msg, isUser)
	})
	if isUser {
		return []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
			}),
			bubble,
			gap,
			avatar,
		}
	}
	return []layout.FlexChild{
		avatar,
		gap,
		bubble,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
		}),
	}
}

func (a *ChatApp) messageTime(gtx layout.Context, key, value string, isUser bool) layout.Dimensions {
	time := func(gtx layout.Context) layout.Dimensions {
		return a.selectableText(gtx, key, value, uiSp(uiTextMeta), rgb(125, 138, 164), true)
	}
	if isUser {
		return layout.Inset{Top: unit.Dp(5), Right: unit.Dp(58)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, time)
		})
	}
	return layout.Inset{Top: unit.Dp(5), Left: unit.Dp(56)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, time)
	})
}

func (a *ChatApp) bubbleContent(gtx layout.Context, msg Message, isUser bool) layout.Dimensions {
	bg := color.NRGBA{R: 255, G: 246, B: 249, A: 225}
	border := color.NRGBA{R: 246, G: 226, B: 235, A: 230}
	if isUser {
		bg = color.NRGBA{R: 239, G: 253, B: 252, A: 232}
		border = color.NRGBA{R: 213, G: 239, B: 238, A: 235}
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(13), Bottom: unit.Dp(13), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				body := msg.Text
				if body == "" {
					if len(msg.ToolCalls) > 0 {
						body = fmt.Sprintf(a.uiText("tool_call_status_fmt"), toolCallNames(msg.ToolCalls))
					} else if len(msg.Attachments)+len(msg.Images) > 0 {
						body = a.uiText("image_only")
					} else {
						body = a.uiText("empty_tool_message")
					}
				}
				return a.selectableTextStyled(gtx, "message:"+msg.ID, compactParagraphSpacing(body), uiSp(uiTextBody), rgb(21, 28, 43), false, font.SemiBold, 1.55)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				paths := append([]string{}, msg.Attachments...)
				paths = append(paths, msg.Images...)
				if len(paths) == 0 {
					return layout.Dimensions{}
				}
				return a.imageStrip(gtx, paths, unit.Dp(72))
			}),
		)
	})
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: 9, SW: 9, NE: 9, NW: 9}
	shadow := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(6)))).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 72, G: 84, B: 118, A: 10}, rr.Op(gtx.Ops))
	shadow.Pop()
	paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, border, clip.Stroke{Path: rr.Path(gtx.Ops), Width: 1}.Op())
	call.Add(gtx.Ops)
	return dims
}

func (a *ChatApp) avatar(gtx layout.Context, role string) layout.Dimensions {
	size := gtx.Dp(unit.Dp(42))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	bg := rgb(106, 100, 246)
	isUser := role != "assistant"
	if role != "assistant" {
		bg = rgb(25, 180, 151)
	}
	paint.FillShape(gtx.Ops, bg, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
	initial := "A"
	if isUser {
		initial = "U"
	}
	txt := material.Body1(a.th, initial)
	txt.Color = rgb(255, 255, 255)
	txt.TextSize = uiSp(uiTextAvatar)
	txt.Alignment = text.Middle
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, txt.Layout)
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
				return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
					return widget.Image{Src: imgOp, Fit: widget.Contain}.Layout(gtx)
				})
			})
		}))
	}
	return children
}

func toolCallNames(calls []ToolCall) string {
	seen := map[string]bool{}
	var names []string
	for _, call := range calls {
		if call.Name == "" || seen[call.Name] {
			continue
		}
		seen[call.Name] = true
		names = append(names, call.Name)
	}
	return strings.Join(names, ", ")
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
						return handClickable(gtx, open, func(gtx layout.Context) layout.Dimensions {
							rr := clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Max}, SE: 4, SW: 4, NE: 4, NW: 4}
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
	return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, color.NRGBA{R: 42, G: 48, B: 60, A: 230})
		lbl := material.Body1(a.th, "×")
		lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		lbl.Alignment = text.Middle
		return layout.Center.Layout(gtx, lbl.Layout)
	})
}

func (a *ChatApp) composer(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	loading := a.loading
	attached := len(a.pendingImgs)
	pending := append([]string(nil), a.pendingImgs...)
	a.mu.Unlock()

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(0), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 11,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 240},
			Border: color.NRGBA{R: 223, G: 229, B: 242, A: 245},
			Shadow: color.NRGBA{R: 79, G: 93, B: 132, A: 26},
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(a.th, &a.input, a.uiText("message_placeholder"))
						ed.TextSize = uiSp(uiTextBody)
						gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(54))
						if max := gtx.Dp(unit.Dp(132)); gtx.Constraints.Max.Y > max {
							gtx.Constraints.Max.Y = max
						}
						return ed.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(pending) == 0 {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.pendingImageStrip(gtx, pending)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(a.th, &a.imagePath, a.uiText("image_paths_placeholder"))
								ed.TextSize = uiSp(uiTextSmall)
								return ed.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx) }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.iconSquareButton(gtx, &a.addImgBtn)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if attached == 0 {
									return layout.Dimensions{}
								}
								return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.subtleTextButton(gtx, &a.clearImgBtn, a.uiText("clear_images"))
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx) }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := a.uiText("send")
								if loading {
									label = a.uiText("sending")
								}
								return a.primarySendButton(gtx, label)
							}),
						)
					}),
				)
			})
		})
	})
}

func (a *ChatApp) paintAmbient(gtx layout.Context) {
	w, h := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	if w <= 0 || h <= 0 {
		return
	}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 246, G: 248, B: 255, A: 150}, clip.Rect(image.Rect(0, 0, w, gtx.Dp(unit.Dp(130)))).Op())
	paint.FillShape(gtx.Ops, color.NRGBA{R: 250, G: 247, B: 255, A: 115}, clip.Rect(image.Rect(0, h-gtx.Dp(unit.Dp(170)), w, h)).Op())
}

type surfaceStyle struct {
	Radius int
	Bg     color.NRGBA
	Border color.NRGBA
	Shadow color.NRGBA
}

func roundedSurface(gtx layout.Context, style surfaceStyle, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: style.Radius, SW: style.Radius, NE: style.Radius, NW: style.Radius}
	if style.Shadow.A > 0 {
		shadow := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(7)))).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, style.Shadow, rr.Op(gtx.Ops))
		shadow.Pop()
	}
	paint.FillShape(gtx.Ops, style.Bg, rr.Op(gtx.Ops))
	if style.Border.A > 0 {
		paint.FillShape(gtx.Ops, style.Border, clip.Stroke{Path: rr.Path(gtx.Ops), Width: 1}.Op())
	}
	call.Add(gtx.Ops)
	return dims
}

func (a *ChatApp) onlineStatus(gtx layout.Context, label string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(22))
	gtx.Constraints.Min.Y = height
	if gtx.Constraints.Max.Y > height {
		gtx.Constraints.Max.Y = height
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(unit.Dp(8))
				gtx.Constraints.Min = image.Pt(size, size)
				gtx.Constraints.Max = image.Pt(size, size)
				paint.FillShape(gtx.Ops, rgb(28, 197, 162), clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
				return layout.Dimensions{Size: image.Pt(size, size)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(7)}.Layout(gtx) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				txt := material.Body2(a.th, label)
				txt.TextSize = uiSp(uiTextSmall)
				txt.Color = rgb(77, 88, 112)
				return layout.Inset{Top: unit.Dp(1)}.Layout(gtx, txt.Layout)
			}),
		)
	})
}

func (a *ChatApp) settingsPill(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(94)), gtx.Dp(unit.Dp(32)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return handClickable(gtx, &a.settingsBtn, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 7,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 238},
			Border: color.NRGBA{R: 222, G: 227, B: 246, A: 255},
			Shadow: color.NRGBA{R: 83, G: 99, B: 144, A: 12},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawIconBox(gtx, settingsIcon, unit.Dp(18), rgb(103, 98, 246))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.buttonLabel(gtx, a.uiText("settings"), uiSp(uiTextControl), rgb(100, 96, 246), font.Medium)
					}),
				)
			})
		})
	})
}

func (a *ChatApp) iconSquareButton(gtx layout.Context, btn *widget.Clickable) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(46)), gtx.Dp(unit.Dp(46)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 6,
			Bg:     rgb(255, 255, 255),
			Border: color.NRGBA{R: 221, G: 226, B: 245, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return drawIconBox(gtx, imageIcon, unit.Dp(21), rgb(104, 100, 246))
			})
		})
	})
}

func (a *ChatApp) subtleTextButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.buttonLabel(gtx, label, uiSp(uiTextSmall), rgb(106, 116, 140), font.Medium)
		})
	})
}

func (a *ChatApp) primarySendButton(gtx layout.Context, label string) layout.Dimensions {
	size := image.Pt(gtx.Dp(unit.Dp(118)), gtx.Dp(unit.Dp(48)))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return handClickable(gtx, &a.sendBtn, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 6,
			Bg:     rgb(105, 96, 245),
			Shadow: color.NRGBA{R: 92, G: 84, B: 221, A: 42},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return drawIconBox(gtx, sendIcon, unit.Dp(21), rgb(255, 255, 255))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.buttonLabel(gtx, label, uiSp(uiTextControl), rgb(255, 255, 255), font.SemiBold)
					}),
				)
			})
		})
	})
}

func (a *ChatApp) actionButton(gtx layout.Context, btn *widget.Clickable, label string) layout.Dimensions {
	return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 6,
			Bg:     rgb(86, 107, 230),
			Border: color.NRGBA{R: 86, G: 107, B: 230, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(34))
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.buttonLabel(gtx, label, uiSp(uiTextControl), rgb(255, 255, 255), font.SemiBold)
				})
			})
		})
	})
}

func (a *ChatApp) buttonLabel(gtx layout.Context, label string, size unit.Sp, c color.NRGBA, weight font.Weight) layout.Dimensions {
	lbl := material.Label(a.th, size, a.buttonText(label))
	lbl.Font.Weight = weight
	lbl.Color = c
	lbl.MaxLines = 1
	lbl.Truncator = "..."
	return visuallyCenteredLabel(gtx, lbl)
}

func visuallyCenteredLabel(gtx layout.Context, lbl material.LabelStyle) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := lbl.Layout(gtx)
	call := macro.Stop()
	trans := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(2)))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	trans.Pop()
	return dims
}

func (a *ChatApp) selectableText(gtx layout.Context, key, value string, size unit.Sp, c color.NRGBA, singleLine bool) layout.Dimensions {
	return a.selectableTextStyled(gtx, key, value, size, c, singleLine, font.Normal, 0)
}

func (a *ChatApp) selectableTextStyled(gtx layout.Context, key, value string, size unit.Sp, c color.NRGBA, singleLine bool, weight font.Weight, lineHeightScale float32) layout.Dimensions {
	ed := a.textEditor(key)
	if ed.Text() != value {
		ed.SetText(value)
	}
	ed.ReadOnly = true
	ed.SingleLine = singleLine
	style := material.Editor(a.th, ed, "")
	style.TextSize = size
	style.Font.Weight = weight
	style.LineHeightScale = lineHeightScale
	style.Color = c
	style.SelectionColor = color.NRGBA{R: 104, G: 100, B: 246, A: 58}
	return style.Layout(gtx)
}

func handClickable(gtx layout.Context, btn *widget.Clickable, w layout.Widget) layout.Dimensions {
	dims := material.Clickable(gtx, btn, w)
	addHandCursor(gtx, dims.Size)
	return dims
}

func addHandCursor(gtx layout.Context, size image.Point) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	defer clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops).Pop()
	pointer.CursorPointer.Add(gtx.Ops)
}

func (a *ChatApp) buttonText(label string) string {
	if a.isChinese() {
		return label
	}
	return strings.ToUpper(label)
}

func compactParagraphSpacing(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	for strings.Contains(s, "\n\n") {
		s = strings.ReplaceAll(s, "\n\n", "\n")
	}
	return s
}

func (a *ChatApp) textEditor(key string) *widget.Editor {
	a.mu.Lock()
	defer a.mu.Unlock()
	ed := a.textEditors[key]
	if ed == nil {
		ed = new(widget.Editor)
		ed.ReadOnly = true
		a.textEditors[key] = ed
	}
	return ed
}

func drawIconBox(gtx layout.Context, icon *widget.Icon, dp unit.Dp, c color.NRGBA) layout.Dimensions {
	size := image.Pt(gtx.Dp(dp), gtx.Dp(dp))
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return icon.Layout(gtx, c)
}

func (a *ChatApp) settingsOverlay(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	open := a.settingsOpen
	tab := a.settingsTab
	note := a.settingsNote
	a.mu.Unlock()
	if !open {
		return layout.Dimensions{}
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Dimensions{Size: gtx.Constraints.Max}
	paint.Fill(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255})
	layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.H6(a.th, a.uiText("settings"))
						title.Color = rgb(26, 33, 46)
						return title.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						txt := material.Body2(a.th, note)
						txt.Color = rgb(76, 92, 122)
						return txt.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.actionButton(gtx, &a.settingsSave, a.uiText("save"))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.actionButton(gtx, &a.settingsDone, a.uiText("done"))
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(18)}.Layout(gtx) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(210))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(210))
						return a.settingsSidebar(gtx, tab)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(18)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.settingsContent(gtx, tab)
					}),
				)
			}),
		)
	})
	if tab == 4 {
		a.memoryEditorDialog(gtx)
	}
	call := macro.Stop()
	call.Add(gtx.Ops)
	return dims
}

func (a *ChatApp) settingsSidebar(gtx layout.Context, selected int) layout.Dimensions {
	labels := []string{
		a.uiText("tab_user_profile"),
		a.uiText("tab_companion_profile"),
		a.uiText("tab_system_access"),
		a.uiText("tab_context"),
		a.uiText("tab_companion_memory"),
		a.uiText("tab_workflows"),
		a.uiText("tab_interface_privacy"),
		a.uiText("tab_about"),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, func() []layout.FlexChild {
		children := make([]layout.FlexChild, 0, len(labels))
		for i, label := range labels {
			idx := i
			text := label
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					bg := color.NRGBA{R: 238, G: 242, B: 248, A: 255}
					fg := rgb(48, 58, 75)
					if idx == selected {
						bg = rgb(86, 107, 230)
						fg = rgb(255, 255, 255)
					}
					return handClickable(gtx, &a.settingsTabs[idx], func(gtx layout.Context) layout.Dimensions {
						rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(44)))}, SE: 4, SW: 4, NE: 4, NW: 4}
						defer rr.Push(gtx.Ops).Pop()
						paint.Fill(gtx.Ops, bg)
						return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body1(a.th, text)
							lbl.Color = fg
							return lbl.Layout(gtx)
						})
					})
				})
			}))
		}
		return children
	}()...)
}

func (a *ChatApp) settingsContent(gtx layout.Context, tab int) layout.Dimensions {
	return material.List(a.th, &a.settingsList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			switch tab {
			case 0:
				return a.userProfileSettings(gtx)
			case 1:
				return a.companionProfileSettings(gtx)
			case 2:
				return a.systemAccessSettings(gtx)
			case 3:
				return a.contextSettings(gtx)
			case 4:
				return a.companionMemorySettings(gtx)
			case 5:
				return a.workflowLogSettings(gtx)
			case 6:
				return a.interfacePrivacySettings(gtx)
			case 7:
				return a.aboutSettings(gtx)
			default:
				return layout.Dimensions{}
			}
		})
	})
}

func (a *ChatApp) userProfileSettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_user_profile"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(a.settingsField(&a.userFullName, a.uiText("full_name"))),
		layout.Rigid(a.settingsField(&a.userNickname, a.uiText("nickname"))),
		layout.Rigid(a.settingsField(&a.userAvatar, a.uiText("avatar_path"))),
		layout.Rigid(a.settingsField(&a.userGender, a.uiText("gender"))),
		layout.Rigid(a.settingsField(&a.userBirthDate, a.uiText("birth_date"))),
		layout.Rigid(a.settingsTextArea(&a.userDescription, a.uiText("personal_description"), 132)),
	)
}

func (a *ChatApp) companionProfileSettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_companion_profile"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(a.settingsField(&a.agentFullName, a.uiText("full_name"))),
		layout.Rigid(a.settingsField(&a.agentNickname, a.uiText("nickname"))),
		layout.Rigid(a.settingsField(&a.agentAvatar, a.uiText("avatar_path"))),
		layout.Rigid(a.settingsField(&a.agentCanonImage, a.uiText("canon_image"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			path := strings.TrimSpace(a.agentCanonImage.Text())
			if path == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.imageStrip(gtx, []string{path}, unit.Dp(96))
			})
		}),
		layout.Rigid(a.settingsField(&a.agentGender, a.uiText("gender"))),
		layout.Rigid(a.settingsField(&a.agentBirthDate, a.uiText("birth_date"))),
		layout.Rigid(a.settingsTextArea(&a.agentStory, a.uiText("character_story"), 112)),
		layout.Rigid(a.settingsTextArea(&a.agentPersonality, a.uiText("personality"), 112)),
		layout.Rigid(a.settingsTextArea(&a.agentHabits, a.uiText("habits"), 112)),
	)
}

func (a *ChatApp) systemAccessSettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_system_access"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(a.settingsField(&a.baseURL, a.uiText("base_url"))),
		layout.Rigid(a.settingsField(&a.model, a.uiText("model"))),
		layout.Rigid(a.settingsField(&a.apiKey, "API Key")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := material.CheckBox(a.th, &a.computerUseEnabled, a.uiText("computer_use_enabled")).Layout(gtx)
				addHandCursor(gtx, dims.Size)
				return dims
			})
		}),
	)
}

func (a *ChatApp) contextSettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_context"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, a.settingsField(&a.contextMessagesK, a.uiText("context_k"))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
				layout.Flexed(1, a.settingsField(&a.memoryTopN, a.uiText("memory_top_n"))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
				layout.Flexed(1, a.settingsField(&a.memoryRandomM, a.uiText("memory_random_m"))),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, a.settingsField(&a.summarizeThreshold, a.uiText("summarize_threshold"))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
				layout.Flexed(1, a.settingsField(&a.dreamTriggerThreshold, a.uiText("dream_threshold"))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx) }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						dims := material.CheckBox(a.th, &a.dailyMeditate, a.uiText("daily_meditate")).Layout(gtx)
						addHandCursor(gtx, dims.Size)
						return dims
					})
				}),
			)
		}),
		layout.Rigid(a.settingsTextArea(&a.summarizePrompt, a.uiText("summarize_prompt"), 110)),
		layout.Rigid(a.settingsTextArea(&a.dreamPrompt, a.uiText("dream_prompt"), 110)),
		layout.Rigid(a.settingsTextArea(&a.meditatePrompt, a.uiText("meditate_prompt"), 110)),
	)
}

func (a *ChatApp) companionMemorySettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_companion_memory"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(a.memoryManager),
	)
}

func (a *ChatApp) memoryManager(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	mode := a.memoryMode
	entries := append([]LongTermMemory(nil), a.memoryEntries...)
	a.mu.Unlock()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(a.modeButton(&a.memoryModeBtns[0], a.uiText("memory_bank"), mode == "memory")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
				layout.Rigid(a.modeButton(&a.memoryModeBtns[1], a.uiText("knowledge_bank"), mode == "knowledge")),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.actionButton(gtx, &a.memoryNewBtn, a.uiText("new"))
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(470))
			return material.List(a.th, &a.memoryList).Layout(gtx, len(entries), func(gtx layout.Context, i int) layout.Dimensions {
				entry := entries[i]
				return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					btn := a.memoryEntryButtonFor(entry.ModelID)
					for btn.Clicked(gtx) {
						a.selectMemoryEntry(i)
					}
					return a.memoryEntryRow(gtx, btn, entry)
				})
			})
		}),
	)
}

func (a *ChatApp) memoryEditorDialog(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	open := a.memoryEditOpen
	a.mu.Unlock()
	if !open {
		return layout.Dimensions{}
	}
	dims := layout.Dimensions{Size: gtx.Constraints.Max}
	paint.Fill(gtx.Ops, color.NRGBA{R: 18, G: 24, B: 38, A: 54})
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := minInt(gtx.Constraints.Max.X-gtx.Dp(unit.Dp(80)), gtx.Dp(unit.Dp(680)))
		if width < gtx.Dp(unit.Dp(420)) {
			width = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32))
		}
		gtx.Constraints.Min.X = width
		gtx.Constraints.Max.X = width
		return roundedSurface(gtx, surfaceStyle{
			Radius: 8,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			Border: color.NRGBA{R: 211, G: 220, B: 240, A: 255},
			Shadow: color.NRGBA{R: 58, G: 70, B: 105, A: 34},
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := material.H6(a.th, a.uiText("memory_editor"))
								title.TextSize = uiSp(uiTextHero)
								title.Font.Weight = font.SemiBold
								title.Color = rgb(28, 35, 48)
								return title.Layout(gtx)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.actionButton(gtx, &a.memoryCancelBtn, a.uiText("cancel"))
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(14)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(.55, a.settingsField(&a.memoryID, "ID")),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Flexed(1, a.settingsField(&a.memoryCategory, a.uiText("category"))),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Flexed(1, a.settingsField(&a.memoryStatus, a.uiText("status"))),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, a.settingsField(&a.memoryTags, a.uiText("tags"))),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Flexed(.7, a.settingsField(&a.memoryRank, a.uiText("rank"))),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
							layout.Flexed(.7, a.settingsField(&a.memoryConfidence, a.uiText("confidence"))),
						)
					}),
					layout.Rigid(a.settingsTextArea(&a.memoryContent, a.uiText("content"), 160)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return a.actionButton(gtx, &a.memoryDeleteBtn, a.uiText("delete"))
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return a.actionButton(gtx, &a.memorySaveBtn, a.uiText("save_memory"))
								}),
							)
						})
					}),
				)
			})
		})
	})
	return dims
}

func (a *ChatApp) memoryEntryRow(gtx layout.Context, btn *widget.Clickable, entry LongTermMemory) layout.Dimensions {
	return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		rowHeight := gtx.Dp(unit.Dp(46))
		gtx.Constraints.Min.Y = rowHeight
		if gtx.Constraints.Max.Y > rowHeight {
			gtx.Constraints.Max.Y = rowHeight
		}
		return roundedSurface(gtx, surfaceStyle{
			Radius: 4,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 250},
			Border: color.NRGBA{R: 224, G: 231, B: 244, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y = rowHeight
			gtx.Constraints.Max.Y = rowHeight
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(42))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(42))
						return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
							id := material.Body2(a.th, fmt.Sprintf("M%d", entry.ModelID))
							id.TextSize = uiSp(uiTextControl)
							id.Font.Weight = font.SemiBold
							id.Color = rgb(89, 94, 232)
							id.MaxLines = 1
							id.Truncator = "..."
							return id.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(150))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(150))
						return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
							meta := material.Body2(a.th, strings.ToUpper(emptyDefault(entry.Status, "active"))+" · "+emptyDefault(entry.Category, "memory"))
							meta.TextSize = uiSp(uiTextSmall)
							meta.Font.Weight = font.Medium
							meta.Color = rgb(107, 119, 145)
							meta.MaxLines = 1
							meta.Truncator = "..."
							return meta.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(14)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
							content := material.Body2(a.th, strings.ReplaceAll(entry.Content, "\n", " "))
							content.TextSize = uiSp(uiTextControl)
							content.Font.Weight = font.Medium
							content.Color = rgb(34, 44, 62)
							content.MaxLines = 1
							content.Truncator = "..."
							return content.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(74))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(74))
						return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
							score := material.Body2(a.th, fmt.Sprintf("R%d / C%d", entry.Rank, entry.Confidence))
							score.TextSize = uiSp(uiTextSmall)
							score.Color = rgb(122, 133, 158)
							score.MaxLines = 1
							score.Truncator = "..."
							return score.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (a *ChatApp) memoryEntryButtonFor(modelID int) *widget.Clickable {
	a.mu.Lock()
	defer a.mu.Unlock()
	btn := a.memoryButtons[modelID]
	if btn == nil {
		btn = new(widget.Clickable)
		a.memoryButtons[modelID] = btn
	}
	return btn
}

func (a *ChatApp) workflowLogSettings(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	logs := append([]WorkflowLog(nil), a.workflowLogs...)
	a.mu.Unlock()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_workflows"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(a.workflowLogHeader),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(6)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(420))
			return material.List(a.th, &a.logList).Layout(gtx, len(logs), func(gtx layout.Context, i int) layout.Dimensions {
				return a.workflowLogRow(gtx, logs[i])
			})
		}),
	)
}

func (a *ChatApp) workflowLogHeader(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			a.logHeaderCell(a.uiText("log_time"), 118),
			a.logHeaderCell(a.uiText("log_workflow"), 150),
			a.logHeaderCell(a.uiText("log_status"), 92),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(a.th, a.uiText("log_result"))
				lbl.TextSize = uiSp(uiTextSmall)
				lbl.Font.Weight = font.SemiBold
				lbl.Color = rgb(89, 101, 126)
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (a *ChatApp) logHeaderCell(label string, width unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(width)
		gtx.Constraints.Max.X = gtx.Dp(width)
		lbl := material.Body2(a.th, label)
		lbl.TextSize = uiSp(uiTextSmall)
		lbl.Font.Weight = font.SemiBold
		lbl.Color = rgb(89, 101, 126)
		return lbl.Layout(gtx)
	})
}

func (a *ChatApp) workflowLogRow(gtx layout.Context, log WorkflowLog) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowHeight := gtx.Dp(unit.Dp(46))
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		gtx.Constraints.Min.Y = rowHeight
		if gtx.Constraints.Max.Y > rowHeight {
			gtx.Constraints.Max.Y = rowHeight
		}
		return roundedSurface(gtx, surfaceStyle{
			Radius: 4,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 250},
			Border: color.NRGBA{R: 224, G: 231, B: 244, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y = rowHeight
			gtx.Constraints.Max.Y = rowHeight
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(a.logTextCell(formatLogTime(log.CreatedAt), 118, rgb(71, 83, 108), font.Medium)),
					layout.Rigid(a.logTextCell(workflowDisplayName(log.Name), 150, rgb(33, 43, 60), font.SemiBold)),
					layout.Rigid(a.logStatusCell(log.Status, 92)),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
							result := material.Body2(a.th, workflowResultSummary(log))
							result.TextSize = uiSp(uiTextControl)
							result.Font.Weight = font.Medium
							result.Color = rgb(77, 88, 110)
							result.MaxLines = 1
							result.Truncator = "..."
							return result.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (a *ChatApp) logTextCell(value string, width unit.Dp, c color.NRGBA, weight font.Weight) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(width)
		gtx.Constraints.Max.X = gtx.Dp(width)
		return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(a.th, value)
			lbl.TextSize = uiSp(uiTextControl)
			lbl.Font.Weight = weight
			lbl.Color = c
			lbl.MaxLines = 1
			lbl.Truncator = "..."
			return lbl.Layout(gtx)
		})
	}
}

func (a *ChatApp) logStatusCell(status string, width unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(width)
		gtx.Constraints.Max.X = gtx.Dp(width)
		return verticalCenterRowCell(gtx, func(gtx layout.Context) layout.Dimensions {
			c := rgb(25, 156, 126)
			if strings.Contains(strings.ToLower(status), "fail") || strings.Contains(strings.ToLower(status), "error") {
				c = rgb(200, 72, 89)
			}
			lbl := material.Body2(a.th, strings.ToUpper(emptyDefault(status, "unknown")))
			lbl.TextSize = uiSp(uiTextSmall)
			lbl.Font.Weight = font.SemiBold
			lbl.Color = c
			lbl.MaxLines = 1
			lbl.Truncator = "..."
			return lbl.Layout(gtx)
		})
	}
}

func verticalCenterRowCell(gtx layout.Context, content layout.Widget) layout.Dimensions {
	min := gtx.Constraints.Min
	macro := op.Record(gtx.Ops)
	gtx.Constraints.Min.Y = 0
	dims := content(gtx)
	call := macro.Stop()
	size := dims.Size
	if size.X < min.X {
		size.X = min.X
	}
	if size.Y < min.Y {
		size.Y = min.Y
	}
	y := (size.Y - dims.Size.Y) / 2
	defer op.Offset(image.Pt(0, y)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return layout.Dimensions{
		Size:     size,
		Baseline: dims.Baseline + size.Y - dims.Size.Y - y,
	}
}

func formatLogTime(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	return t.Format("2006-01-02 15:04")
}

func workflowDisplayName(name string) string {
	name = strings.TrimSuffix(name, ".workflow")
	if name == "" {
		return "workflow"
	}
	return name
}

func workflowResultSummary(log WorkflowLog) string {
	result := strings.TrimSpace(log.Result)
	if result == "" {
		result = strings.TrimSpace(log.Arguments)
	}
	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.Join(strings.Fields(result), " ")
	return emptyDefault(result, "-")
}

func (a *ChatApp) interfacePrivacySettings(gtx layout.Context) layout.Dimensions {
	current := a.uiText("language_english")
	next := a.uiText("switch_to_chinese")
	if a.isChinese() {
		current = a.uiText("language_chinese")
		next = a.uiText("switch_to_english")
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_interface_privacy"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(a.th, a.uiText("language")+": "+current)
			lbl.Color = rgb(48, 58, 75)
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.actionButton(gtx, &a.languageToggle, next)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(16)}.Layout(gtx) }),
		layout.Rigid(a.settingsField(&a.messageCacheLimit, a.uiText("message_cache_limit"))),
	)
}

func (a *ChatApp) aboutSettings(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, a.uiText("tab_about"))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(a.th, "Hominial.Elli version "+appVersion)
			lbl.TextSize = uiSp(uiTextHero)
			lbl.Font.Weight = font.SemiBold
			lbl.Color = rgb(28, 35, 48)
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(a.th, a.uiText("about_description"))
			lbl.TextSize = uiSp(uiTextBody)
			lbl.Font.Weight = font.Medium
			lbl.Color = rgb(70, 82, 105)
			lbl.LineHeightScale = 1.35
			return lbl.Layout(gtx)
		}),
	)
}

func (a *ChatApp) modeButton(btn *widget.Clickable, label string, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		bg := rgb(235, 239, 247)
		fg := rgb(42, 51, 68)
		if selected {
			bg = rgb(86, 107, 230)
			fg = rgb(255, 255, 255)
		}
		return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(gtx.Dp(unit.Dp(112)), gtx.Dp(unit.Dp(36)))}, SE: 4, SW: 4, NE: 4, NW: 4}
			defer rr.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, bg)
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.buttonLabel(gtx, label, uiSp(uiTextSmall), fg, font.Medium)
			})
		})
	}
}

func (a *ChatApp) memoryEntryButton(btn *widget.Clickable, label string, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if selected {
			bg = color.NRGBA{R: 226, G: 232, B: 255, A: 255}
		}
		return handClickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			rr := clip.RRect{Rect: image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(42)))}, SE: 4, SW: 4, NE: 4, NW: 4}
			defer rr.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, bg)
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, material.Body2(a.th, label).Layout)
		})
	}
}

func (a *ChatApp) settingsPlaceholder(gtx layout.Context, title, body string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(a.th, title)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if strings.TrimSpace(body) == "" {
				return layout.Dimensions{}
			}
			txt := material.Body1(a.th, body)
			txt.Color = rgb(77, 88, 106)
			return txt.Layout(gtx)
		}),
	)
}

func sectionTitle(th *material.Theme, title string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.H6(th, title)
		lbl.Color = rgb(28, 35, 48)
		return lbl.Layout(gtx)
	}
}

func (a *ChatApp) settingsField(ed *widget.Editor, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return a.settingsEditor(gtx, ed, label, 38, true)
	}
}

func (a *ChatApp) settingsTextArea(ed *widget.Editor, label string, height unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return a.settingsEditor(gtx, ed, label, height, false)
	}
}

func (a *ChatApp) settingsEditor(gtx layout.Context, ed *widget.Editor, label string, height unit.Dp, singleLine bool) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(a.th, label)
				lbl.TextSize = uiSp(uiTextSmall)
				lbl.Font.Weight = font.Medium
				lbl.Color = rgb(70, 82, 105)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(5)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				border := color.NRGBA{R: 202, G: 212, B: 235, A: 255}
				if gtx.Focused(ed) {
					border = color.NRGBA{R: 103, G: 98, B: 246, A: 255}
				}
				return roundedSurface(gtx, surfaceStyle{
					Radius: 5,
					Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 252},
					Border: border,
				}, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(height)
					if gtx.Constraints.Max.Y > gtx.Dp(height) {
						gtx.Constraints.Max.Y = gtx.Dp(height)
					}
					if singleLine {
						return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								gtx.Constraints.Min.Y = 0
								style := a.settingsEditorStyle(ed)
								style.LineHeightScale = 1.12
								return style.Layout(gtx)
							})
						})
					}
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
						style := a.settingsEditorStyle(ed)
						style.LineHeightScale = 1.35
						return style.Layout(gtx)
					})
				})
			}),
		)
	})
}

func (a *ChatApp) settingsEditorStyle(ed *widget.Editor) material.EditorStyle {
	style := material.Editor(a.th, ed, "")
	style.TextSize = uiSp(uiTextBody)
	style.Font.Weight = font.Medium
	style.Color = rgb(31, 40, 56)
	style.HintColor = rgb(145, 154, 174)
	style.SelectionColor = color.NRGBA{R: 104, G: 100, B: 246, A: 58}
	return style
}

func (a *ChatApp) uiText(key string) string {
	zh := a.isChinese()
	if zh {
		if value, ok := zhUIStrings[key]; ok {
			return value
		}
	}
	if value, ok := enUIStrings[key]; ok {
		return value
	}
	return key
}

var enUIStrings = map[string]string{
	"app_title":               "Hominial.Elli",
	"online":                  "Online",
	"thinking":                "Thinking...",
	"settings":                "Settings",
	"empty_chat":              "Start a conversation, or attach images and send.",
	"load_older":              "Load older",
	"image_only":              "(image only)",
	"tool_call_status_fmt":    "(using tool: %s)",
	"empty_tool_message":      "(tool message)",
	"message_placeholder":     "Message",
	"image_paths_placeholder": "Image paths, comma separated",
	"add_image_fmt":           "Add Image (%d)",
	"clear_images":            "Clear Images",
	"clear":                   "Clear",
	"send":                    "Send",
	"sending":                 "Sending",
	"save":                    "Save",
	"done":                    "Done",
	"tab_user_profile":        "User Profile",
	"tab_companion_profile":   "Hominial Profile",
	"tab_system_access":       "System Access",
	"tab_context":             "Context",
	"tab_companion_memory":    "Hominial Memory",
	"tab_workflows":           "Workflows & Notifications",
	"tab_interface_privacy":   "Interface & Privacy",
	"tab_about":               "About",
	"full_name":               "Full Name",
	"nickname":                "Nickname",
	"avatar_path":             "Avatar Path",
	"gender":                  "Gender",
	"birth_date":              "Birth Date",
	"personal_description":    "Personal Description",
	"canon_image":             "CanonImage",
	"character_story":         "Character Story",
	"personality":             "Personality",
	"habits":                  "Habits",
	"base_url":                "Base URL",
	"model":                   "Model",
	"computer_use_enabled":    "Enable Computer Use",
	"context_k":               "Context Message Window K",
	"memory_top_n":            "Memory Top N",
	"memory_random_m":         "Random Memories M",
	"summarize_threshold":     "Summarize Threshold",
	"dream_threshold":         "Dream Trigger Threshold",
	"daily_meditate":          "Daily Meditate",
	"summarize_prompt":        "Summarize Prompt",
	"dream_prompt":            "Dream Prompt",
	"meditate_prompt":         "Meditate Prompt",
	"memory_bank":             "Memory",
	"knowledge_bank":          "Knowledge",
	"new":                     "New",
	"cancel":                  "Cancel",
	"memory_editor":           "Memory Editor",
	"save_memory":             "Save Memory",
	"delete":                  "Delete",
	"category":                "Category",
	"tags":                    "Tags, comma separated",
	"rank":                    "Rank 0-10",
	"confidence":              "Confidence 0-100",
	"status":                  "Status",
	"content":                 "Content",
	"language":                "Language",
	"language_english":        "English",
	"language_chinese":        "Chinese",
	"message_cache_limit":     "Cached Messages",
	"switch_to_chinese":       "Switch to Chinese",
	"switch_to_english":       "Switch to English",
	"save_failed":             "Save failed",
	"prompt_save_failed":      "Prompt save failed",
	"saved":                   "Saved",
	"settings_saved":          "Settings saved",
	"memory_load_failed":      "Memory load failed",
	"log_load_failed":         "Log load failed",
	"entry_save_failed":       "Entry save failed",
	"saved_memory_fmt":        "Saved M%d",
	"select_memory_first":     "Select a memory first",
	"delete_failed":           "Delete failed",
	"deleted_memory_fmt":      "Deleted M%d",
	"log_time":                "Time",
	"log_workflow":            "Workflow",
	"log_status":              "Status",
	"log_result":              "Result",
	"about_description":       "Empathetic Living Life Intelligence, a hominial runtime for companion memory, context, workflows, and self-evolution.",
}

var zhUIStrings = map[string]string{
	"app_title":               "半人类.艾丽",
	"online":                  "在线",
	"thinking":                "思考中...",
	"settings":                "设置",
	"empty_chat":              "开始对话，或附加图片后发送。",
	"load_older":              "加载更早消息",
	"image_only":              "(仅图片)",
	"tool_call_status_fmt":    "(正在使用工具：%s)",
	"empty_tool_message":      "(工具消息)",
	"message_placeholder":     "输入消息",
	"image_paths_placeholder": "图片路径，可用逗号分隔",
	"add_image_fmt":           "添加图片 (%d)",
	"clear_images":            "清空图片",
	"clear":                   "清空",
	"send":                    "发送",
	"sending":                 "发送中",
	"save":                    "保存",
	"done":                    "完成",
	"tab_user_profile":        "用户画像",
	"tab_companion_profile":   "伴人画像",
	"tab_system_access":       "系统接入",
	"tab_context":             "上下文",
	"tab_companion_memory":    "伴人记忆",
	"tab_workflows":           "工作流与通知",
	"tab_interface_privacy":   "界面与隐私",
	"tab_about":               "关于",
	"full_name":               "全名",
	"nickname":                "昵称",
	"avatar_path":             "头像路径",
	"gender":                  "性别",
	"birth_date":              "出生日期",
	"personal_description":    "个人描述",
	"canon_image":             "定妆照 CanonImage",
	"character_story":         "角色故事",
	"personality":             "个性描述",
	"habits":                  "行为习惯",
	"base_url":                "接入点 Base URL",
	"model":                   "模型",
	"computer_use_enabled":    "启用 Computer Use",
	"context_k":               "上下文消息窗口 K",
	"memory_top_n":            "记忆 Top N",
	"memory_random_m":         "随机记忆 M",
	"summarize_threshold":     "Summarize 阈值",
	"dream_threshold":         "Dream 触发阈值",
	"daily_meditate":          "每日 Meditate",
	"summarize_prompt":        "Summarize 关键提示词",
	"dream_prompt":            "Dream 关键提示词",
	"meditate_prompt":         "Meditate 关键提示词",
	"memory_bank":             "记忆库",
	"knowledge_bank":          "知识库",
	"new":                     "新增",
	"cancel":                  "取消",
	"memory_editor":           "记忆编辑",
	"save_memory":             "保存记忆",
	"delete":                  "删除",
	"category":                "分类",
	"tags":                    "标签，逗号分隔",
	"rank":                    "重要度 0-10",
	"confidence":              "置信度 0-100",
	"status":                  "状态",
	"content":                 "内容",
	"language":                "语言",
	"language_english":        "英文",
	"language_chinese":        "中文",
	"message_cache_limit":     "单次缓存消息条数",
	"switch_to_chinese":       "切换到中文",
	"switch_to_english":       "切换到英文",
	"save_failed":             "保存失败",
	"prompt_save_failed":      "提示词保存失败",
	"saved":                   "已保存",
	"settings_saved":          "设置已保存",
	"memory_load_failed":      "记忆加载失败",
	"log_load_failed":         "日志加载失败",
	"entry_save_failed":       "条目保存失败",
	"saved_memory_fmt":        "已保存 M%d",
	"select_memory_first":     "请先选择一条记忆",
	"delete_failed":           "删除失败",
	"deleted_memory_fmt":      "已删除 M%d",
	"log_time":                "时间",
	"log_workflow":            "工作流",
	"log_status":              "状态",
	"log_result":              "结果",
	"about_description":       "Empathetic Living Life Intelligence，一个用于伴人记忆、上下文、工作流与自我演化的 hominial 运行时。",
}

func centerText(gtx layout.Context, th *material.Theme, s string) layout.Dimensions {
	return layout.Center.Layout(gtx, material.Body1(th, s).Layout)
}

func rgb(r, g, b byte) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}
