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
	ClientTaskID                 string  `json:"clientTaskId,omitempty" gorm:"size:191;index"`
	RequestFingerprint           string  `json:"-" gorm:"size:64"`
	Phase                        string  `json:"phase,omitempty" gorm:"size:32;index"`
	Revision                     int64   `json:"-"`
	PollFailures                 int     `json:"-"`
	NextPollAt                   string  `json:"-" gorm:"size:64;index"`
	RefundedAt                   string  `json:"-" gorm:"size:64"`
	Hidden                       bool    `json:"-" gorm:"default:false"`
	ID                           string  `json:"id" gorm:"primaryKey"`
	UserID                       string  `json:"userId" gorm:"index;index:idx_video_tasks_user_source_status_created_at,priority:1"`
	UserDisplayName              string  `json:"userDisplayName" gorm:"size:191"`
	Model                        string  `json:"model" gorm:"index"`
	ChannelID                    string  `json:"channelId" gorm:"index"`
	UserChannelID                string  `json:"userChannelId" gorm:"index"`
	ChannelName                  string  `json:"channelName" gorm:"size:191"`
	WorkflowRef                  string  `json:"workflowRef,omitempty" gorm:"type:text"`
	Source                       string  `json:"source" gorm:"index;index:idx_video_tasks_user_source_status_created_at,priority:2"`
	SourceID                     string  `json:"source_id" gorm:"index"`
	UpstreamTaskID               string  `json:"upstreamTaskId" gorm:"index"`
	UpstreamVideoID              string  `json:"upstreamVideoId" gorm:"index"`
	Status                       string  `json:"status" gorm:"index:idx_video_tasks_status_created_at,priority:1;index:idx_video_tasks_status_completed_at,priority:1;index:idx_video_tasks_user_source_status_created_at,priority:3"`
	Progress                     int     `json:"progress"`
	Seconds                      string  `json:"seconds" gorm:"size:32"`
	Size                         string  `json:"size" gorm:"size:64"`
	VideoURL                     string  `json:"videoUrl"`
	Error                        string  `json:"error" gorm:"type:text"`
	ErrorDetail                  string  `json:"errorDetail"`
	RequestBody                  string  `json:"requestBody"`
	ParameterTranslationSnapshot string  `json:"-"`
	ResponseBody                 string  `json:"responseBody"`
	LastResponse                 string  `json:"lastResponse"`
	Credits                      float64 `json:"credits" gorm:"type:decimal(20,2)"`
	CreatedAt                    string  `json:"createdAt" gorm:"index;index:idx_video_tasks_status_created_at,priority:2;index:idx_video_tasks_user_source_status_created_at,priority:4"`
	UpdatedAt                    string  `json:"updatedAt" gorm:"index"`
	StartedAt                    string  `json:"startedAt" gorm:"size:64"`
	CompletedAt                  string  `json:"completedAt" gorm:"size:64;index:idx_video_tasks_status_completed_at,priority:2"`
	LastPolledAt                 string  `json:"lastPolledAt" gorm:"index"`
}
