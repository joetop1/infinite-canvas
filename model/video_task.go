package model

// Generation phases describe delivery separately from provider execution.
const (
	GenerationSubmitting = "submitting"
	GenerationUncertain  = "submission_unknown"
	GenerationQueued     = "queued"
	GenerationRunning    = "running"
	GenerationRetrieving = "retrieving"
	GenerationCompleted  = "completed"
	GenerationFailed     = "provider_failed"
	GenerationRejected   = "rejected"
	GenerationRecovery   = "recovery_needed"
)

type VideoTask struct {
	ClientTaskID       string  `json:"clientTaskId,omitempty" gorm:"index"`
	RequestFingerprint string  `json:"-"`
	Phase              string  `json:"phase,omitempty" gorm:"index"`
	Revision           int64   `json:"-"`
	PollFailures       int     `json:"-"`
	NextPollAt         string  `json:"-" gorm:"index"`
	RefundedAt         string  `json:"-"`
	Hidden             bool    `json:"-" gorm:"default:false"`
	ID                 string  `json:"id" gorm:"primaryKey"`
	UserID             string  `json:"userId" gorm:"index"`
	UserDisplayName    string  `json:"userDisplayName"`
	Model              string  `json:"model" gorm:"index"`
	ChannelID          string  `json:"channelId" gorm:"index"`
	UserChannelID      string  `json:"userChannelId" gorm:"index"`
	ChannelName        string  `json:"channelName"`
	WorkflowRef        string  `json:"workflowRef,omitempty" gorm:"type:text"`
	Source             string  `json:"source" gorm:"index"`
	SourceID           string  `json:"source_id" gorm:"index"`
	UpstreamTaskID     string  `json:"upstreamTaskId" gorm:"index"`
	UpstreamVideoID    string  `json:"upstreamVideoId" gorm:"index"`
	Status             string  `json:"status" gorm:"index:idx_video_tasks_status_created_at,priority:1"`
	Progress           int     `json:"progress"`
	Seconds            string  `json:"seconds"`
	Size               string  `json:"size"`
	VideoURL           string  `json:"videoUrl" gorm:"type:text"`
	Error              string  `json:"error" gorm:"type:text"`
	ErrorDetail        string  `json:"errorDetail" gorm:"type:text"`
	RequestBody        string  `json:"requestBody" gorm:"type:text"`
	ResponseBody       string  `json:"responseBody" gorm:"type:text"`
	LastResponse       string  `json:"lastResponse" gorm:"type:text"`
	Credits            float64 `json:"credits" gorm:"type:decimal(20,2)"`
	CreatedAt          string  `json:"createdAt" gorm:"index;index:idx_video_tasks_status_created_at,priority:2"`
	UpdatedAt          string  `json:"updatedAt" gorm:"index"`
	StartedAt          string  `json:"startedAt"`
	CompletedAt        string  `json:"completedAt"`
	LastPolledAt       string  `json:"lastPolledAt" gorm:"index"`
}
