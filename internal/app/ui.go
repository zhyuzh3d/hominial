package app

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

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
	hasOlder := a.hasOlder
	a.mu.Unlock()

	if len(msgs) == 0 {
		return centerText(gtx, a.th, "Single conversation. Add text, optionally attach image paths, then send.")
	}

	count := len(msgs)
	if hasOlder {
		count++
	}
	return material.List(a.th, &a.scrollList).Layout(gtx, count, func(gtx layout.Context, i int) layout.Dimensions {
		if hasOlder {
			if i == 0 {
				return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, material.Button(a.th, &a.loadOlderBtn, "Load older").Layout)
				})
			}
			i--
		}
		return a.messageBubble(gtx, msgs[i])
	})
}

func (a *ChatApp) messageBubble(gtx layout.Context, msg Message) layout.Dimensions {
	isUser := msg.Role != "assistant"
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, messageRowChildren(a, msg, isUser)...)
	})
}

func messageRowChildren(a *ChatApp, msg Message, isUser bool) []layout.FlexChild {
	avatar := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return a.avatar(gtx, msg.Role)
	})
	gap := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
	})
	bubble := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		maxW := int(float32(gtx.Constraints.Max.X) * 0.68)
		if maxW < gtx.Dp(unit.Dp(260)) {
			maxW = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(56))
		}
		if maxW > gtx.Dp(unit.Dp(680)) {
			maxW = gtx.Dp(unit.Dp(680))
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

func (a *ChatApp) bubbleContent(gtx layout.Context, msg Message, isUser bool) layout.Dimensions {
	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if isUser {
		bg = color.NRGBA{R: 218, G: 246, B: 214, A: 255}
	}
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				meta := strings.ToUpper(msg.Role)
				if !msg.CreatedAt.IsZero() {
					meta += "  " + msg.CreatedAt.Format("15:04")
				}
				title := material.Body2(a.th, meta)
				title.Color = rgb(92, 100, 108)
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
				return a.imageStrip(gtx, paths, unit.Dp(72))
			}),
		)
	})
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: 10, SW: 10, NE: 10, NW: 10}
	defer rr.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, bg)
	call.Add(gtx.Ops)
	return dims
}

func (a *ChatApp) avatar(gtx layout.Context, role string) layout.Dimensions {
	size := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	bg := rgb(102, 121, 245)
	label := "A"
	if role != "assistant" {
		bg = rgb(34, 150, 82)
		label = "U"
	}
	defer clip.Ellipse{Max: image.Pt(size, size)}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, bg)
	txt := material.Body1(a.th, label)
	txt.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	txt.Alignment = text.Middle
	return layout.Center.Layout(gtx, txt.Layout)
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

func centerText(gtx layout.Context, th *material.Theme, s string) layout.Dimensions {
	return layout.Center.Layout(gtx, material.Body1(th, s).Layout)
}

func rgb(r, g, b byte) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 255}
}
