package windowkeeper

import "time"

const (
	KindFiveHour = "five_hour"
	KindWeekly   = "weekly"
	KindMonthly  = "monthly"
	KindCustom   = "custom"

	SourceExplicit      = "explicit"
	SourceDerived       = "derived"
	SourceProbeRelative = "probe_relative"

	GroupFree        = "free"
	GroupPlusTeam    = "plus_team"
	GroupProAndAbove = "pro_and_above"
	GroupObserveOnly = "observe_only"

	PhaseBaseline     = "baseline"
	PhaseBlocked      = "blocked"
	PhaseDue          = "due"
	PhaseAnchored     = "anchored"
	PhaseFixed        = "fixed"
	PhaseInconclusive = "inconclusive"
	PhaseStopped      = "stopped"
	PhaseClear        = "clear"
	PhaseAbsent       = "absent"

	ActionIdle     = "idle"
	ActionWait     = "wait"
	ActionBaseline = "baseline"
	ActionSend     = "send"
	ActionFixed    = "fixed"

	PolicyActionProbe   = "probe"
	PolicyActionBackoff = "backoff"
	PolicyActionPause   = "pause_account"
	PolicyActionStop    = "stop_window"

	ErrKindQuota  = "quota"
	ErrKindAuth   = "auth"
	ErrKindConfig = "config"
	ErrKindRetry  = "retry"
)

type Window struct {
	LimitID       string    `json:"limit_id"`
	Slot          string    `json:"slot"`
	Kind          string    `json:"kind"`
	PeriodSeconds int64     `json:"period_seconds"`
	UsedPercent   float64   `json:"used_percent"`
	LimitReached  bool      `json:"limit_reached"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	TimeSource    string    `json:"time_source"`
	Gating        bool      `json:"gating"`
	OtherModel    bool      `json:"other_model,omitempty"`
	RawName       string    `json:"raw_name,omitempty"`
}

type Snapshot struct {
	PlanType string   `json:"plan_type"`
	Windows  []Window `json:"windows"`
}

type Options struct {
	Kinds             []string
	IncludeCodeReview bool
	IncludeAdditional string
	Model             string
}

type State struct {
	LimitID        string    `json:"limit_id"`
	Slot           string    `json:"slot"`
	Kind           string    `json:"kind"`
	PeriodSeconds  int64     `json:"period_seconds"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	LastBlockedEnd time.Time `json:"last_blocked_end"`
	TimeSource     string    `json:"time_source"`
	UsedPercent    float64   `json:"used_percent"`
	Gating         bool      `json:"gating"`
	Phase          string    `json:"phase"`
	SeenBlocked    bool      `json:"seen_blocked"`
	Hypothesis     string    `json:"hypothesis"`
	Absent         bool      `json:"absent"`
}

type Result struct {
	Action     string
	NotBefore  time.Time
	Generation string
	States     []State
}

type Decision struct {
	Action  string
	Consume bool
}

type AccountRef struct {
	AuthID, AuthIndex, AccountID, Email, Name, Plan string
	Disabled, Unavailable                           bool
	NextRetryAfter                                  time.Time
}

type SendResult struct {
	OK          bool
	Status      int
	Kind        string
	Excerpt     string
	ResponseID  string
	ReqHeaders  string
	ReqBody     string
	RespHeaders string
	RespBody    string
}

// AttemptFinish carries terminal attempt fields written by FinishAttempt.
type AttemptFinish struct {
	Status      string
	HTTPStatus  int
	Kind        string
	Excerpt     string
	ResponseID  string
	ReqHeaders  string
	ReqBody     string
	RespHeaders string
	RespBody    string
}

type Attempt struct {
	ID            int64     `json:"id"`
	AccountID     string    `json:"account_id"`
	GenerationKey string    `json:"generation_key"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	Status        string    `json:"status"`
	HTTPStatus    int       `json:"http_status,omitempty"`
	ErrorKind     string    `json:"error_kind,omitempty"`
	AttemptNo     int       `json:"attempt_no"`
	NotBefore     time.Time `json:"not_before,omitempty"`
	Excerpt       string    `json:"output_excerpt,omitempty"`
	ResponseID    string    `json:"response_id,omitempty"`
	ReqHeaders    string    `json:"req_headers,omitempty"`
	ReqBody       string    `json:"req_body,omitempty"`
	RespHeaders   string    `json:"resp_headers,omitempty"`
	RespBody      string    `json:"resp_body,omitempty"`
}

type Account struct {
	AuthID        string    `json:"auth_id"`
	AuthIndex     string    `json:"auth_index"`
	AccountID     string    `json:"account_id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	PlanType      string    `json:"plan_type"`
	PlanGroup     string    `json:"plan_group"`
	ShapeMismatch string    `json:"shape_mismatch"`
	Disabled      bool      `json:"disabled"`
	Unavailable   bool      `json:"unavailable"`
	OverrideJSON  string    `json:"override_json"`
	PauseReason   string    `json:"pause_reason"`
	NotBefore     time.Time `json:"not_before"`
	Windows       []State   `json:"windows,omitempty"`
}
