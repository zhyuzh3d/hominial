package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const appVersion = "0.3.3"

type ChatApp struct {
	win *gioapp.Window
	th  *material.Theme
	ops op.Ops

	cfg Config

	input           widget.Editor
	imagePath       widget.Editor
	baseURL         widget.Editor
	model           widget.Editor
	apiKey          widget.Editor
	sendBtn         widget.Clickable
	addImgBtn       widget.Clickable
	clearBtn        widget.Clickable
	loadOlderBtn    widget.Clickable
	clearImgBtn     widget.Clickable
	settingsBtn     widget.Clickable
	settingsSave    widget.Clickable
	settingsDone    widget.Clickable
	languageToggle  widget.Clickable
	settingsTabs    [8]widget.Clickable
	settingsList    widget.List
	memoryList      widget.List
	logList         widget.List
	memoryModeBtns  [2]widget.Clickable
	memoryEntryBtns [80]widget.Clickable
	memoryNewBtn    widget.Clickable
	memorySaveBtn   widget.Clickable
	memoryDeleteBtn widget.Clickable
	memoryCancelBtn widget.Clickable
	closeBtn        widget.Clickable
	zoomInBtn       widget.Clickable
	zoomOutBtn      widget.Clickable
	actualBtn       widget.Clickable
	fitBtn          widget.Clickable
	scrollList      widget.List

	mu             sync.Mutex
	messages       []Message
	pendingImgs    []string
	status         string
	settings       UISettings
	settingsOpen   bool
	settingsTab    int
	settingsNote   string
	scrollToEnd    bool
	memoryMode     string
	memoryEntries  []LongTermMemory
	selectedMemory int
	memoryEditOpen bool
	workflowLogs   []WorkflowLog
	loading        bool
	enlarged       string
	historyPath    string
	peConfig       PEConfig
	hasOlder       bool
	preview        previewState
	imgCache       map[string]image.Image
	imgOps         map[string]paint.ImageOp
	imageButtons   map[string]*widget.Clickable
	removeButtons  map[string]*widget.Clickable
	memoryButtons  map[int]*widget.Clickable
	textEditors    map[string]*widget.Editor

	userFullName    widget.Editor
	userNickname    widget.Editor
	userAvatar      widget.Editor
	userGender      widget.Editor
	userBirthDate   widget.Editor
	userDescription widget.Editor

	agentFullName    widget.Editor
	agentNickname    widget.Editor
	agentAvatar      widget.Editor
	agentCanonImage  widget.Editor
	agentGender      widget.Editor
	agentBirthDate   widget.Editor
	agentStory       widget.Editor
	agentPersonality widget.Editor
	agentHabits      widget.Editor

	contextMessagesK      widget.Editor
	memoryTopN            widget.Editor
	memoryRandomM         widget.Editor
	summarizeThreshold    widget.Editor
	dreamTriggerThreshold widget.Editor
	dailyMeditate         widget.Bool
	computerUseEnabled    widget.Bool
	summarizePrompt       widget.Editor
	dreamPrompt           widget.Editor
	meditatePrompt        widget.Editor
	memoryID              widget.Editor
	memoryContent         widget.Editor
	memoryCategory        widget.Editor
	memoryTags            widget.Editor
	memoryRank            widget.Editor
	memoryConfidence      widget.Editor
	memoryStatus          widget.Editor
	messageCacheLimit     widget.Editor
}

func Run() {
	go func() {
		w := new(gioapp.Window)
		w.Option(gioapp.Title("Hominial.Elli"), gioapp.Size(unit.Dp(980), unit.Dp(760)))
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
		win:            w,
		th:             newAppTheme(),
		cfg:            cfg,
		historyPath:    historyPath(),
		peConfig:       defaultPEConfig(),
		imgCache:       make(map[string]image.Image),
		imgOps:         make(map[string]paint.ImageOp),
		imageButtons:   make(map[string]*widget.Clickable),
		removeButtons:  make(map[string]*widget.Clickable),
		memoryButtons:  make(map[int]*widget.Clickable),
		textEditors:    make(map[string]*widget.Editor),
		status:         "Ready",
		scrollToEnd:    true,
		memoryMode:     "memory",
		selectedMemory: -1,
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
	if settings, err := loadUISettings(a.historyPath, cfg); err == nil {
		a.settings = settings
		a.cfg = settings.System
		a.peConfig = peConfigFromRuntime(settings.Runtime)
		a.trimMessageCacheLocked(true)
	} else {
		a.settings = UISettings{System: cfg, Runtime: defaultRuntimeSettings()}
		a.status = "Settings warning: " + err.Error()
	}
	a.input.SingleLine = false
	a.input.Submit = true
	a.imagePath.SingleLine = true
	a.baseURL.SingleLine = true
	a.model.SingleLine = true
	a.apiKey.SingleLine = true
	a.baseURL.SetText(a.cfg.BaseURL)
	a.model.SetText(a.cfg.Model)
	a.apiKey.SetText(a.cfg.APIKey)
	a.applySettingsToEditors(a.settings)
	a.loadSettingsPanelData()
	a.scrollList.Axis = layout.Vertical
	a.scrollList.ScrollToEnd = true
	a.settingsList.Axis = layout.Vertical
	a.memoryList.Axis = layout.Vertical
	a.logList.Axis = layout.Vertical
	go a.runSchedulerLoop()
	return a
}

func newAppTheme() *material.Theme {
	th := material.NewTheme()
	faces := gofont.Collection()
	for _, path := range []string{
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/System/Library/Fonts/Supplemental/NotoSansCJK-Regular.ttc",
		"/System/Library/Fonts/Supplemental/NotoSansCJKsc-Regular.otf",
		"/Library/Fonts/Microsoft YaHei.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/Supplemental/Songti.ttc",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := opentype.ParseCollection(raw)
		if err != nil {
			continue
		}
		faces = append(faces, parsed...)
	}
	th.Shaper = text.NewShaper(text.WithCollection(faces))
	th.Face = font.Typeface("PingFang SC, STHeiti, Hiragino Sans GB, Noto Sans CJK SC, Source Han Sans SC, Microsoft YaHei, Arial Unicode MS, sans-serif")
	th.TextSize = uiSp(uiTextBody)
	return th
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
	for a.settingsBtn.Clicked(gtx) {
		a.mu.Lock()
		a.settingsOpen = true
		a.settingsNote = ""
		a.mu.Unlock()
		a.win.Invalidate()
	}
	a.mu.Lock()
	settingsOpen := a.settingsOpen
	a.mu.Unlock()
	if settingsOpen {
		for a.settingsDone.Clicked(gtx) {
			a.mu.Lock()
			a.settingsOpen = false
			a.mu.Unlock()
			a.win.Invalidate()
		}
		for a.settingsSave.Clicked(gtx) {
			a.saveSettingsFromEditors()
		}
		for a.languageToggle.Clicked(gtx) {
			a.toggleLanguage()
		}
		for a.memoryModeBtns[0].Clicked(gtx) {
			a.setMemoryMode("memory")
		}
		for a.memoryModeBtns[1].Clicked(gtx) {
			a.setMemoryMode("knowledge")
		}
		for a.memoryNewBtn.Clicked(gtx) {
			a.newMemoryEditor()
		}
		for a.memorySaveBtn.Clicked(gtx) {
			a.saveMemoryEditor()
		}
		for a.memoryDeleteBtn.Clicked(gtx) {
			a.deleteMemoryEditor()
		}
		for a.memoryCancelBtn.Clicked(gtx) {
			a.mu.Lock()
			a.memoryEditOpen = false
			a.mu.Unlock()
			a.win.Invalidate()
		}
		for i := range a.settingsTabs {
			for a.settingsTabs[i].Clicked(gtx) {
				a.mu.Lock()
				a.settingsTab = i
				a.mu.Unlock()
				a.win.Invalidate()
				if i == 4 || i == 5 {
					a.loadSettingsPanelData()
				}
			}
		}
		return
	}
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
		if a.loading {
			a.status = "Wait for the current reply before clearing"
			a.mu.Unlock()
			a.win.Invalidate()
			continue
		}
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
		a.status = "Still sending previous message..."
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	imgs := append([]string(nil), a.pendingImgs...)
	imgs = append(imgs, typedImgs...)
	imgs = dedupeStrings(imgs)
	if text == "" && len(imgs) == 0 {
		a.status = "Type a message or attach an image"
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	a.cfg.BaseURL = strings.TrimSpace(a.baseURL.Text())
	a.cfg.Model = strings.TrimSpace(a.model.Text())
	a.cfg.APIKey = strings.TrimSpace(a.apiKey.Text())
	a.settings.System = a.cfg
	userMsg := Message{Role: "user", Text: text, Attachments: imgs, CreatedAt: time.Now()}
	if err := saveMessageDB(a.historyPath, &userMsg); err != nil {
		a.status = "History save failed: " + err.Error()
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	a.input.SetText("")
	a.imagePath.SetText("")
	a.pendingImgs = nil
	a.messages = append(a.messages, userMsg)
	a.trimMessageCacheLocked(true)
	a.scrollToEnd = true
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
			a.trimMessageCacheLocked(true)
			a.scrollToEnd = true
			a.status = "Ready"
			a.mu.Unlock()
			orch, orchErr := runOrchestrator(context.Background(), historyPath, cfg, reply.ID, result.ToolCalls)
			a.mu.Lock()
			for i := range orch.Messages {
				if err := saveMessageDB(historyPath, &orch.Messages[i]); err == nil {
					a.messages = append(a.messages, orch.Messages[i])
					a.trimMessageCacheLocked(true)
					a.scrollToEnd = true
				}
			}
			if orchErr != nil {
				a.status = "Tool warning: " + orchErr.Error()
			}
			a.mu.Unlock()
			a.runContinuations(context.Background(), cfg, historyPath, peCfg, orch.Continuations)
			if err := maybeRefreshShortSummarization(context.Background(), cfg, historyPath, peCfg); err != nil {
				a.setStatus("Summarize warning: " + err.Error())
			}
		}
		a.win.Invalidate()
	}()
}

func (a *ChatApp) runContinuations(ctx context.Context, cfg Config, historyPath string, peCfg PEConfig, continuations []ToolContinuation) {
	const maxContinuationActionDepth = 30
	const maxContinuationTotalDepth = 60
	const maxContinuationWallClock = 10 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, maxContinuationWallClock)
	defer cancel()
	actionDepth := 0
	computerHelpSeen := false
	for totalDepth := 0; totalDepth < maxContinuationTotalDepth && actionDepth < maxContinuationActionDepth && len(continuations) > 0 && ctx.Err() == nil; totalDepth++ {
		if continuationsContainComputerHelp(continuations) {
			computerHelpSeen = true
		}
		if !continuationsAreInformational(continuations) {
			actionDepth++
		}
		if computerHelpSeen {
			continuations = addComputerHelpReminder(continuations)
		}
		a.setStatus("Continuing with tool results...")
		result, err := callResponsesWithContinuations(ctx, cfg, historyPath, peCfg, continuations)
		if err != nil {
			a.setStatus("Continuation warning: " + err.Error())
			return
		}
		reply := result.Message
		if strings.TrimSpace(reply.Text) == "" && len(reply.Images) == 0 && len(reply.ToolCalls) == 0 {
			return
		}
		if err := saveMessageDB(historyPath, &reply); err != nil {
			a.setStatus("History save failed: " + err.Error())
			return
		}
		a.mu.Lock()
		a.messages = append(a.messages, reply)
		a.trimMessageCacheLocked(true)
		a.scrollToEnd = true
		a.mu.Unlock()
		a.win.Invalidate()

		orch, orchErr := runOrchestrator(ctx, historyPath, cfg, reply.ID, result.ToolCalls)
		a.mu.Lock()
		for i := range orch.Messages {
			if err := saveMessageDB(historyPath, &orch.Messages[i]); err == nil {
				a.messages = append(a.messages, orch.Messages[i])
				a.trimMessageCacheLocked(true)
				a.scrollToEnd = true
			}
		}
		if orchErr != nil {
			a.status = "Tool warning: " + orchErr.Error()
		} else {
			a.status = "Ready"
		}
		a.mu.Unlock()
		a.win.Invalidate()
		continuations = orch.Continuations
	}
	if ctx.Err() != nil {
		a.setStatus("Continuation stopped after time limit")
		return
	}
	if len(continuations) > 0 {
		a.setStatus("Continuation stopped after safety limit")
	}
}

func continuationsAreInformational(continuations []ToolContinuation) bool {
	if len(continuations) == 0 {
		return false
	}
	for _, continuation := range continuations {
		if !continuation.Informational {
			return false
		}
	}
	return true
}

func continuationsContainComputerHelp(continuations []ToolContinuation) bool {
	for _, continuation := range continuations {
		if isComputerHelpPayload(continuation.Payload) {
			return true
		}
	}
	return false
}

func addComputerHelpReminder(continuations []ToolContinuation) []ToolContinuation {
	if len(continuations) == 0 {
		return continuations
	}
	out := make([]ToolContinuation, len(continuations))
	copy(out, continuations)
	const reminder = "Computer API guide has already been fetched in this turn. Do not call computer help again just to use observe or act; continue with observe/act based on the current task and latest screenshot.\n"
	for i := range out {
		if !strings.Contains(out[i].Text, "Computer API guide has already been fetched") {
			out[i].Text = reminder + out[i].Text
		}
	}
	return out
}

func (a *ChatApp) runSchedulerLoop() {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		<-timer.C
		a.runDueScheduledTools()
		timer.Reset(time.Minute)
	}
}

func (a *ChatApp) runDueScheduledTools() {
	a.mu.Lock()
	cfg := a.cfg
	historyPath := a.historyPath
	a.mu.Unlock()
	messages, err := executeDueScheduledTools(context.Background(), historyPath, cfg, 10)
	if err != nil {
		a.setStatus("Schedule warning: " + err.Error())
		return
	}
	if len(messages) == 0 {
		return
	}
	a.mu.Lock()
	for i := range messages {
		if err := saveMessageDB(historyPath, &messages[i]); err == nil {
			a.messages = append(a.messages, messages[i])
			a.trimMessageCacheLocked(true)
			a.scrollToEnd = true
		}
	}
	a.status = "Scheduled task completed"
	a.mu.Unlock()
	a.win.Invalidate()
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
		trimmedOlder := len(a.messages) > normalizeAppearanceSettings(a.settings.Appearance).MessageCacheLimit
		a.trimMessageCacheLocked(true)
		a.hasOlder = hasOlder || trimmedOlder
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

func (a *ChatApp) applySettingsToEditors(settings UISettings) {
	a.userFullName.SetText(settings.User.FullName)
	a.userNickname.SetText(settings.User.Nickname)
	a.userAvatar.SetText(settings.User.Avatar)
	a.userGender.SetText(settings.User.Gender)
	a.userBirthDate.SetText(settings.User.BirthDate)
	a.userDescription.SetText(settings.User.Description)
	a.agentFullName.SetText(settings.Companion.FullName)
	a.agentNickname.SetText(settings.Companion.Nickname)
	a.agentAvatar.SetText(settings.Companion.Avatar)
	a.agentCanonImage.SetText(settings.Companion.CanonImage)
	a.agentGender.SetText(settings.Companion.Gender)
	a.agentBirthDate.SetText(settings.Companion.BirthDate)
	a.agentStory.SetText(settings.Companion.Story)
	a.agentPersonality.SetText(settings.Companion.Personality)
	a.agentHabits.SetText(settings.Companion.Habits)
	runtime := normalizeRuntimeSettings(settings.Runtime)
	a.contextMessagesK.SetText(strconv.Itoa(runtime.ContextMessagesK))
	a.memoryTopN.SetText(strconv.Itoa(runtime.MemoryTopN))
	a.memoryRandomM.SetText(strconv.Itoa(runtime.MemoryRandomM))
	a.summarizeThreshold.SetText(strconv.Itoa(runtime.SummarizeThreshold))
	a.dreamTriggerThreshold.SetText(strconv.Itoa(runtime.DreamTriggerThreshold))
	a.dailyMeditate.Value = runtime.DailyMeditateEnabled
	a.computerUseEnabled.Value = runtime.ComputerUseEnabled
	prompts := loadPromptEditorTexts()
	a.summarizePrompt.SetText(prompts["summarize"])
	a.dreamPrompt.SetText(prompts["dream"])
	a.meditatePrompt.SetText(prompts["meditate"])
	appearance := normalizeAppearanceSettings(settings.Appearance)
	a.messageCacheLimit.SetText(strconv.Itoa(appearance.MessageCacheLimit))
	a.userDescription.SingleLine = false
	a.agentStory.SingleLine = false
	a.agentPersonality.SingleLine = false
	a.agentHabits.SingleLine = false
	a.summarizePrompt.SingleLine = false
	a.dreamPrompt.SingleLine = false
	a.meditatePrompt.SingleLine = false
	a.memoryContent.SingleLine = false
	for _, ed := range []*widget.Editor{
		&a.userFullName, &a.userNickname, &a.userAvatar, &a.userGender, &a.userBirthDate,
		&a.agentFullName, &a.agentNickname, &a.agentAvatar, &a.agentCanonImage, &a.agentGender, &a.agentBirthDate,
		&a.contextMessagesK, &a.memoryTopN, &a.memoryRandomM, &a.summarizeThreshold, &a.dreamTriggerThreshold,
		&a.memoryID, &a.memoryCategory, &a.memoryTags, &a.memoryRank, &a.memoryConfidence, &a.memoryStatus,
		&a.messageCacheLimit,
	} {
		ed.SingleLine = true
	}
}

func (a *ChatApp) collectSettingsFromEditors() UISettings {
	a.mu.Lock()
	appearance := normalizeAppearanceSettings(a.settings.Appearance)
	a.mu.Unlock()
	appearance.MessageCacheLimit = intEditorValue(&a.messageCacheLimit, defaultMessageCacheLimit)
	appearance = normalizeAppearanceSettings(appearance)
	return UISettings{
		User: ProfileSettings{
			FullName:    strings.TrimSpace(a.userFullName.Text()),
			Nickname:    strings.TrimSpace(a.userNickname.Text()),
			Avatar:      strings.TrimSpace(a.userAvatar.Text()),
			Gender:      strings.TrimSpace(a.userGender.Text()),
			BirthDate:   strings.TrimSpace(a.userBirthDate.Text()),
			Description: strings.TrimSpace(a.userDescription.Text()),
		},
		Companion: ProfileSettings{
			FullName:    strings.TrimSpace(a.agentFullName.Text()),
			Nickname:    strings.TrimSpace(a.agentNickname.Text()),
			Avatar:      strings.TrimSpace(a.agentAvatar.Text()),
			CanonImage:  strings.TrimSpace(a.agentCanonImage.Text()),
			Gender:      strings.TrimSpace(a.agentGender.Text()),
			BirthDate:   strings.TrimSpace(a.agentBirthDate.Text()),
			Story:       strings.TrimSpace(a.agentStory.Text()),
			Personality: strings.TrimSpace(a.agentPersonality.Text()),
			Habits:      strings.TrimSpace(a.agentHabits.Text()),
		},
		System: Config{
			BaseURL: strings.TrimSpace(a.baseURL.Text()),
			Model:   strings.TrimSpace(a.model.Text()),
			APIKey:  strings.TrimSpace(a.apiKey.Text()),
		},
		Runtime: RuntimeSettings{
			ContextMessagesK:      intEditorValue(&a.contextMessagesK, defaultRuntimeSettings().ContextMessagesK),
			MemoryTopN:            intEditorValue(&a.memoryTopN, defaultRuntimeSettings().MemoryTopN),
			MemoryRandomM:         intEditorValue(&a.memoryRandomM, defaultRuntimeSettings().MemoryRandomM),
			SummarizeThreshold:    intEditorValue(&a.summarizeThreshold, defaultRuntimeSettings().SummarizeThreshold),
			DreamTriggerThreshold: intEditorValue(&a.dreamTriggerThreshold, defaultRuntimeSettings().DreamTriggerThreshold),
			DailyMeditateEnabled:  a.dailyMeditate.Value,
			ComputerUseEnabled:    a.computerUseEnabled.Value,
		},
		Appearance: appearance,
	}
}

func (a *ChatApp) saveSettingsFromEditors() {
	settings := a.collectSettingsFromEditors()
	if err := saveUISettings(a.historyPath, settings); err != nil {
		note := a.uiText("save_failed") + ": " + err.Error()
		a.mu.Lock()
		a.settingsNote = note
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	if err := savePromptEditorTexts(map[string]string{
		"summarize": a.summarizePrompt.Text(),
		"dream":     a.dreamPrompt.Text(),
		"meditate":  a.meditatePrompt.Text(),
	}); err != nil {
		note := a.uiText("prompt_save_failed") + ": " + err.Error()
		a.mu.Lock()
		a.settingsNote = note
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	saved := a.uiText("saved")
	settingsSaved := a.uiText("settings_saved")
	a.mu.Lock()
	a.settings = settings
	a.cfg = settings.System
	a.peConfig = peConfigFromRuntime(settings.Runtime)
	a.trimMessageCacheLocked(true)
	a.settingsNote = saved
	a.status = settingsSaved
	a.mu.Unlock()
	a.win.Invalidate()
}

func normalizeAppearanceSettings(settings AppearanceSettings) AppearanceSettings {
	if settings.Language != "zh" {
		settings.Language = "en"
	}
	if settings.MessageCacheLimit <= 0 {
		settings.MessageCacheLimit = defaultMessageCacheLimit
	}
	if settings.MessageCacheLimit < defaultWindowSize {
		settings.MessageCacheLimit = defaultWindowSize
	}
	return settings
}

func (a *ChatApp) trimMessageCacheLocked(keepNewest bool) {
	limit := normalizeAppearanceSettings(a.settings.Appearance).MessageCacheLimit
	if limit <= 0 || len(a.messages) <= limit {
		return
	}
	var removed []Message
	if keepNewest {
		cut := len(a.messages) - limit
		removed = append([]Message(nil), a.messages[:cut]...)
		a.messages = append([]Message(nil), a.messages[cut:]...)
		a.hasOlder = true
	} else {
		removed = append([]Message(nil), a.messages[limit:]...)
		a.messages = append([]Message(nil), a.messages[:limit]...)
	}
	a.cleanupMessageUIStateLocked(removed)
}

func (a *ChatApp) cleanupMessageUIStateLocked(removed []Message) {
	if len(removed) == 0 {
		return
	}
	for _, msg := range removed {
		delete(a.textEditors, "message:"+msg.ID)
		delete(a.textEditors, "time:"+msg.ID)
	}
	retainedPaths := make(map[string]bool)
	for _, msg := range a.messages {
		for _, p := range msg.Attachments {
			retainedPaths[p] = true
		}
		for _, p := range msg.Images {
			retainedPaths[p] = true
		}
	}
	for _, p := range a.pendingImgs {
		retainedPaths[p] = true
	}
	for _, msg := range removed {
		for _, p := range msg.Attachments {
			if !retainedPaths[p] {
				delete(a.imageButtons, p)
				delete(a.removeButtons, p)
			}
		}
		for _, p := range msg.Images {
			if !retainedPaths[p] {
				delete(a.imageButtons, p)
				delete(a.removeButtons, p)
			}
		}
	}
}

func (a *ChatApp) language() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return normalizeAppearanceSettings(a.settings.Appearance).Language
}

func (a *ChatApp) isChinese() bool {
	return a.language() == "zh"
}

func (a *ChatApp) toggleLanguage() {
	a.mu.Lock()
	settings := normalizeAppearanceSettings(a.settings.Appearance)
	if settings.Language == "zh" {
		settings.Language = "en"
		a.settingsNote = "Language set to English"
		a.status = "Language set to English"
	} else {
		settings.Language = "zh"
		a.settingsNote = "语言已切换为中文"
		a.status = "语言已切换为中文"
	}
	a.settings.Appearance = settings
	a.mu.Unlock()
	a.win.Invalidate()
}

func intEditorValue(ed *widget.Editor, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(ed.Text()))
	if err != nil {
		return def
	}
	return v
}

func (a *ChatApp) loadSettingsPanelData() {
	entries, err := loadMemoryEntries(a.historyPath, a.memoryMode, 0)
	logs, logErr := loadWorkflowLogs(a.historyPath, 60)
	memoryLoadFailed := ""
	if err != nil {
		memoryLoadFailed = a.uiText("memory_load_failed") + ": " + err.Error()
	}
	logLoadFailed := ""
	if logErr != nil {
		logLoadFailed = a.uiText("log_load_failed") + ": " + logErr.Error()
	}
	a.mu.Lock()
	if err == nil {
		a.memoryEntries = entries
		if a.selectedMemory >= len(entries) {
			a.selectedMemory = -1
		}
	} else {
		a.settingsNote = memoryLoadFailed
	}
	if logErr == nil {
		a.workflowLogs = logs
	} else if a.settingsNote == "" {
		a.settingsNote = logLoadFailed
	}
	a.mu.Unlock()
	a.win.Invalidate()
}

func (a *ChatApp) setMemoryMode(mode string) {
	a.mu.Lock()
	a.memoryMode = mode
	a.selectedMemory = -1
	a.mu.Unlock()
	a.newMemoryEditor()
	a.mu.Lock()
	a.memoryEditOpen = false
	a.mu.Unlock()
	a.loadSettingsPanelData()
}

func (a *ChatApp) newMemoryEditor() {
	a.mu.Lock()
	mode := a.memoryMode
	a.selectedMemory = -1
	a.memoryEditOpen = true
	a.mu.Unlock()
	a.memoryID.SetText("")
	a.memoryContent.SetText("")
	if mode == "knowledge" {
		a.memoryCategory.SetText("knowledge")
	} else {
		a.memoryCategory.SetText("")
	}
	a.memoryTags.SetText("")
	a.memoryRank.SetText("3")
	a.memoryConfidence.SetText("70")
	a.memoryStatus.SetText("active")
	a.win.Invalidate()
}

func (a *ChatApp) selectMemoryEntry(index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.memoryEntries) {
		a.mu.Unlock()
		return
	}
	entry := a.memoryEntries[index]
	a.selectedMemory = index
	a.memoryEditOpen = true
	a.mu.Unlock()
	a.memoryID.SetText(strconv.Itoa(entry.ModelID))
	a.memoryContent.SetText(entry.Content)
	a.memoryCategory.SetText(entry.Category)
	a.memoryTags.SetText(tagsEditorText(entry.TagsJSON))
	a.memoryRank.SetText(strconv.Itoa(entry.Rank))
	a.memoryConfidence.SetText(strconv.Itoa(entry.Confidence))
	a.memoryStatus.SetText(entry.Status)
	a.win.Invalidate()
}

func (a *ChatApp) saveMemoryEditor() {
	a.mu.Lock()
	mode := a.memoryMode
	a.mu.Unlock()
	entry := LongTermMemory{
		ModelID:    intEditorValue(&a.memoryID, 0),
		Content:    strings.TrimSpace(a.memoryContent.Text()),
		Category:   strings.TrimSpace(a.memoryCategory.Text()),
		TagsJSON:   tagsJSONFromEditor(a.memoryTags.Text()),
		Rank:       intEditorValue(&a.memoryRank, 3),
		Confidence: intEditorValue(&a.memoryConfidence, 70),
		Status:     strings.TrimSpace(a.memoryStatus.Text()),
	}
	modelID, err := saveMemoryEntry(a.historyPath, mode, entry)
	entrySaveFailed := ""
	if err != nil {
		entrySaveFailed = a.uiText("entry_save_failed") + ": " + err.Error()
	}
	savedMemory := fmt.Sprintf(a.uiText("saved_memory_fmt"), modelID)
	a.mu.Lock()
	if err != nil {
		a.settingsNote = entrySaveFailed
	} else {
		a.settingsNote = savedMemory
		a.memoryID.SetText(strconv.Itoa(modelID))
		a.memoryEditOpen = false
	}
	a.mu.Unlock()
	a.loadSettingsPanelData()
}

func (a *ChatApp) deleteMemoryEditor() {
	modelID := intEditorValue(&a.memoryID, 0)
	if modelID <= 0 {
		note := a.uiText("select_memory_first")
		a.mu.Lock()
		a.settingsNote = note
		a.mu.Unlock()
		a.win.Invalidate()
		return
	}
	err := archiveMemoryEntry(a.historyPath, modelID)
	deleteFailed := ""
	if err != nil {
		deleteFailed = a.uiText("delete_failed") + ": " + err.Error()
	}
	deleted := fmt.Sprintf(a.uiText("deleted_memory_fmt"), modelID)
	a.mu.Lock()
	if err != nil {
		a.settingsNote = deleteFailed
	} else {
		a.settingsNote = deleted
		a.memoryEditOpen = false
	}
	a.mu.Unlock()
	a.newMemoryEditor()
	a.loadSettingsPanelData()
}

func tagsJSONFromEditor(text string) string {
	var tags []string
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	raw, _ := json.Marshal(tags)
	return string(raw)
}

func tagsEditorText(tagsJSON string) string {
	var tags []string
	if json.Unmarshal([]byte(emptyDefault(tagsJSON, "[]")), &tags) != nil {
		return ""
	}
	return strings.Join(tags, ", ")
}
