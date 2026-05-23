package app

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *ChatApp) checkNextOverlay(gtx layout.Context) {
	a.mu.Lock()
	open := a.checkNext.Open
	checks := append([]ComputerStepCheck(nil), a.checkNext.Checks...)
	status := a.checkNext.StatusText
	stepID := a.checkNext.StepID
	a.mu.Unlock()
	if !open {
		return
	}
	for a.checkNext.CloseBtn.Clicked(gtx) {
		a.mu.Lock()
		a.checkNext.Open = false
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	viewport := gtx.Constraints.Max
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 148})
	returnDims := layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := minInt(gtx.Constraints.Max.X-gtx.Dp(unit.Dp(40)), gtx.Dp(unit.Dp(760)))
		h := minInt(gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(64)), gtx.Dp(unit.Dp(620)))
		if w < gtx.Dp(unit.Dp(320)) {
			w = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(20))
		}
		if h < gtx.Dp(unit.Dp(260)) {
			h = gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(32))
		}
		gtx.Constraints.Min = image.Pt(w, h)
		gtx.Constraints.Max = image.Pt(w, h)
		return roundedSurface(gtx, surfaceStyle{
			Radius: 6,
			Bg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
			Border: color.NRGBA{R: 204, G: 214, B: 235, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								title := material.H6(a.th, a.uiText("checknext_title"))
								title.Color = rgb(28, 35, 48)
								return title.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.actionButton(gtx, &a.checkNext.CloseBtn, a.uiText("close"))
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						meta := stepID
						if meta == "" {
							meta = status
						}
						lbl := material.Body2(a.th, meta)
						lbl.Color = rgb(94, 106, 128)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx) }),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if strings.TrimSpace(status) != "" && len(checks) == 0 {
							lbl := material.Body1(a.th, status)
							lbl.Color = rgb(72, 82, 102)
							return lbl.Layout(gtx)
						}
						list := material.List(a.th, &a.checkNext.List)
						return list.Layout(gtx, len(checks), func(gtx layout.Context, i int) layout.Dimensions {
							return a.checkNextRow(gtx, checks[i])
						})
					}),
				)
			})
		})
	})
	defer clip.Rect{Max: viewport}.Push(gtx.Ops).Pop()
	_ = returnDims
}

func (a *ChatApp) checkNextRow(gtx layout.Context, check ComputerStepCheck) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return roundedSurface(gtx, surfaceStyle{
			Radius: 4,
			Bg:     color.NRGBA{R: 247, G: 249, B: 252, A: 255},
			Border: color.NRGBA{R: 225, G: 231, B: 242, A: 255},
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lines := []string{
					fmt.Sprintf("%s #%d  %s  +%dms", check.Kind, check.Attempt, check.CreatedAt.Format("15:04:05.000"), check.ElapsedMS),
				}
				if check.Kind == "checkNext_local" {
					lines = append(lines, fmt.Sprintf("diff=%.4f changed=%v stable=%v", check.DiffScore, check.Changed, check.Stable))
				} else {
					lines = append(lines, fmt.Sprintf("state=%s confidence=%.2f", emptyDefault(check.State, "unknown"), check.Confidence))
					if check.Reason != "" {
						lines = append(lines, check.Reason)
					}
				}
				if check.ScreenshotPath != "" {
					lines = append(lines, check.ScreenshotPath)
				}
				lbl := material.Body2(a.th, strings.Join(lines, "\n"))
				lbl.Color = rgb(33, 43, 60)
				return lbl.Layout(gtx)
			})
		})
	})
}
