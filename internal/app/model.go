package app

import (
	"time"

	"gioui.org/f32"
)

type Config struct {
	BaseURL string
	Model   string
	APIKey  string
}

type ProfileSettings struct {
	FullName    string `json:"full_name,omitempty"`
	Nickname    string `json:"nickname,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
	CanonImage  string `json:"canon_image,omitempty"`
	Gender      string `json:"gender,omitempty"`
	BirthDate   string `json:"birth_date,omitempty"`
	Description string `json:"description,omitempty"`
	Story       string `json:"story,omitempty"`
	Personality string `json:"personality,omitempty"`
	Habits      string `json:"habits,omitempty"`
}

type RuntimeSettings struct {
	ContextMessagesK      int  `json:"context_messages_k"`
	MemoryTopN            int  `json:"memory_top_n"`
	MemoryRandomM         int  `json:"memory_random_m"`
	SummarizeThreshold    int  `json:"summarize_threshold"`
	DreamTriggerThreshold int  `json:"dream_trigger_threshold"`
	DailyMeditateEnabled  bool `json:"daily_meditate_enabled"`
	ComputerUseEnabled    bool `json:"computer_use_enabled"`
}

type AppearanceSettings struct {
	Language          string `json:"language,omitempty"`
	MessageCacheLimit int    `json:"message_cache_limit,omitempty"`
}

type UISettings struct {
	User       ProfileSettings    `json:"user_profile"`
	Companion  ProfileSettings    `json:"companion_profile"`
	System     Config             `json:"system"`
	Runtime    RuntimeSettings    `json:"runtime"`
	Appearance AppearanceSettings `json:"appearance"`
}

type Message struct {
	ID          string
	ThreadID    string
	Seq         int
	Role        string
	Text        string
	Images      []string
	CreatedAt   time.Time
	Attachments []string
	ToolCalls   []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
	Status    string
	Result    string
}

type ToolCallback struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type ToolContinuation struct {
	SourceCallID string
	Text         string
	Images       []string
	Payload      map[string]any
}

type OrchestratorResult struct {
	Messages      []Message
	Continuations []ToolContinuation
}

type PEConfig struct {
	LongMemoryTopN        int
	LongMemoryRandomM     int
	RecentMessagesK       int
	MessageWindowSize     int
	SummarizeThreshold    int
	MaxPromptChars        int
	MaxRoleChars          int
	MaxSectionChars       int
	SummarizeEvery        int
	ReferenceImageTimeout time.Duration
}

type LongTermMemory struct {
	ID              string
	ModelID         int
	Content         string
	Category        string
	TagsJSON        string
	Rank            int
	Confidence      int
	RecallCount     int
	RecalledCount   int
	UsedCount       int
	LastRecalledAt  time.Time
	LastUsedAt      time.Time
	SourceMessageID string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ShortTermSummarization struct {
	ThreadID       string
	Content        string
	UpToSeq        int
	SourceMessages int
	UpdatedAt      time.Time
}

type TurnEvaluation struct {
	ID                    string
	ThreadID              string
	AssistantMessageID    string
	Seq                   int
	PreviousPrediction    string
	ActualBehavior        string
	PredictionMatch       string
	ControlScore          int
	BehaviorEffectiveness int
	ShortGoal             string
	LongGoal              string
	InteractionStrategy   string
	NextPrediction        string
	RawJSON               string
	CreatedAt             time.Time
}

type RoleState struct {
	Health                string
	Mental                string
	Mood                  string
	Action                string
	ShortPurpose          string
	ShortGoalCloseness    int
	ShortGoalDeviation    int
	LongGoalCloseness     int
	LongGoalDeviation     int
	BehaviorEffectiveness int
	ControlScore          int
	MetadataJSON          string
	UpdatedAt             time.Time
}

type UserProfile struct {
	UserID        string
	SetJSON       string
	EstimatedJSON string
	UpdatedAt     time.Time
}

type UserContext struct {
	UserID               string
	Mood                 string
	Action               string
	Environment          string
	NextActionPrediction string
	LastPrediction       string
	EvaluationJSON       string
	UpdatedAt            time.Time
}

type EnvironmentState struct {
	ThreadID     string
	Scene        string
	Surroundings string
	RandomSeed   int64
	MetadataJSON string
	UpdatedAt    time.Time
}

type WorkflowLog struct {
	ID        string
	Name      string
	Status    string
	Arguments string
	Result    string
	CreatedAt time.Time
}

type PromptContext struct {
	Config                PEConfig
	RolePrompt            string
	CompanionProfile      ProfileSettings
	Memories              []LongTermMemory
	MemoryIndex           string
	Summarization         ShortTermSummarization
	TurnEvaluationContext string
	Recent                []Message
	RoleState             RoleState
	UserProfile           UserProfile
	UserContext           UserContext
	Environment           EnvironmentState
	Now                   time.Time
}

type PromptEnvelope struct {
	Input        []map[string]any
	Tools        []map[string]any
	WantsImage   bool
	SystemPrompt string
	SectionSizes map[string]int
}

type APIResult struct {
	Message   Message
	ToolCalls []ToolCall
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

type historyStore struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

const (
	defaultUserID            = "local-user"
	defaultAgentID           = "assistant"
	defaultThreadID          = "default-thread"
	defaultWindowSize        = 240
	defaultMessageCacheLimit = 500
)
