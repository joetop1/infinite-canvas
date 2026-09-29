package service

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

const videoTaskPollInterval = 5 * time.Second
const videoTaskFinishedRetention = 10 * time.Minute
const videoTaskCleanupInterval = 10 * time.Minute
const runningHubWorkflowPageSize = 200

var (
	videoTaskPollerOnce  sync.Once
	videoTaskPollWake    = make(chan struct{}, 1)
	videoTaskPollerMu    sync.RWMutex
	videoTaskPoller      VideoTaskPollFunc
	videoTaskRunningMu   sync.Mutex
	videoTaskRunning     bool
	videoTaskWakePending bool
)

type VideoTaskCreateInput struct {
	UserID          string
	UserDisplayName string
	Model           string
	ChannelID       string
	UserChannelID   string
	ChannelName     string
	WorkflowRef     string
	Source          string
	SourceID        string
	ClientTaskID    string
	UpstreamTaskID  string
	UpstreamVideoID string
	Status          string
	Progress        int
	Seconds         string
	Size            string
	VideoURL        string
	Error           string
	ErrorDetail     string
	RequestBody     string
	ResponseBody    string
	Credits         float64
	BillingName     string
	BillingPath     string
}

type VideoTaskPollUpdate struct {
	Status       string
	Progress     int
	Seconds      string
	Size         string
	VideoURL     string
	Error        string
	ErrorDetail  string
	ResponseBody string
	Phase        string
	Retryable    bool
}

type VideoTaskPollFunc func(model.VideoTask) (VideoTaskPollUpdate, error)

func CreateVideoTask(input VideoTaskCreateInput) (model.VideoTask, error) {
	current := now()
	status := NormalizeVideoTaskStatus(input.Status)
	if status == "" {
		status = "queued"
	}
	task := model.VideoTask{
		ID:              firstVideoTaskValue(input.ClientTaskID, input.UpstreamTaskID, input.UpstreamVideoID, "video-task-"+uuid.NewString()),
		UserID:          strings.TrimSpace(input.UserID),
		UserDisplayName: strings.TrimSpace(input.UserDisplayName),
		Model:           strings.TrimSpace(input.Model),
		ChannelID:       strings.TrimSpace(input.ChannelID),
		UserChannelID:   strings.TrimSpace(input.UserChannelID),
		ChannelName:     strings.TrimSpace(input.ChannelName),
		WorkflowRef:     input.WorkflowRef,
		Source:          normalizeVideoTaskSource(input.Source),
		SourceID:        strings.TrimSpace(input.SourceID),
		UpstreamTaskID:  strings.TrimSpace(input.UpstreamTaskID),
		UpstreamVideoID: strings.TrimSpace(input.UpstreamVideoID),
		Status:          status,
		Progress:        clampProgress(input.Progress),
		Seconds:         strings.TrimSpace(input.Seconds),
		Size:            strings.TrimSpace(input.Size),
		VideoURL:        strings.TrimSpace(input.VideoURL),
		Error:           strings.TrimSpace(input.Error),
		ErrorDetail:     strings.TrimSpace(input.ErrorDetail),
		RequestBody:     input.RequestBody,
		ResponseBody:    input.ResponseBody,
		LastResponse:    input.ResponseBody,
		Credits:         normalizeCredits(input.Credits),
		CreatedAt:       current,
		UpdatedAt:       current,
	}
	if IsCompletedVideoTaskStatus(task.Status) || task.VideoURL != "" {
		task.Status = "completed"
		task.Progress = 100
		task.CompletedAt = current
	} else if IsFailedVideoTaskStatus(task.Status) || task.Error != "" {
		task.Status = "failed"
		task.CompletedAt = current
	}
	var saved model.VideoTask
	var err error
	if input.WorkflowRef != "" {
		saved = task
		err = ConsumeUserCredits(saved.UserID, input.BillingName, saved.Credits, input.BillingPath, &saved)
	} else {
		saved, err = repository.SaveVideoTask(task)
	}
	if err == nil && input.WorkflowRef == "" && !IsCompletedVideoTaskStatus(saved.Status) && !IsFailedVideoTaskStatus(saved.Status) {
		WakeVideoTaskPoller()
	}
	return saved, err
}

func GetUserVideoTask(userID string, id string) (model.VideoTask, bool, error) {
	return repository.GetUserVideoTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func ListUserVideoTasks(userID string, source string, limit int) ([]map[string]any, error) {
	tasks, err := repository.ListUserVideoTasks(strings.TrimSpace(userID), normalizeVideoTaskSource(source), limit)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, VideoTaskResponse(task))
	}
	return result, nil
}

func DeleteUserVideoTask(userID string, id string) error {
	return repository.DeleteUserVideoTask(strings.TrimSpace(userID), strings.TrimSpace(id))
}

func VideoTaskResponse(task model.VideoTask) map[string]any {
	result := map[string]any{
		"id":            task.ID,
		"object":        "video",
		"model":         task.Model,
		"channelId":     task.ChannelID,
		"userChannelId": task.UserChannelID,
		"channelName":   task.ChannelName,
		"source":        task.Source,
		"source_id":     task.SourceID,
		"status":        task.Status,
		"phase":         task.Phase,
		"progress":      task.Progress,
		"task_id":       firstVideoTaskValue(task.UpstreamTaskID, task.ID),
		"video_id":      task.UpstreamVideoID,
		"seconds":       task.Seconds,
		"size":          task.Size,
		"created_at":    task.CreatedAt,
		"updated_at":    task.UpdatedAt,
		"started_at":    task.StartedAt,
		"completed_at":  task.CompletedAt,
		"createdAt":     task.CreatedAt,
		"updatedAt":     task.UpdatedAt,
		"request_body":  task.RequestBody,
	}
	if task.VideoURL != "" {
		result["url"] = task.VideoURL
		result["video_url"] = task.VideoURL
		result["data"] = []map[string]any{{"url": task.VideoURL}}
	}
	if task.WorkflowRef != "" {
		result["workflowRef"] = task.WorkflowRef
	}
	if IsFailedVideoTaskStatus(task.Status) && (task.Error != "" || task.ErrorDetail != "") {
		result["error"] = map[string]any{"message": firstVideoTaskValue(task.Error, task.ErrorDetail)}
		result["error_detail"] = task.ErrorDetail
	}
	return result
}

func StartVideoTaskPoller(poll VideoTaskPollFunc) {
	if poll == nil {
		return
	}
	videoTaskPollerMu.Lock()
	videoTaskPoller = poll
	videoTaskPollerMu.Unlock()
	videoTaskPollerOnce.Do(func() {
		go runVideoTaskPoller()
	})
	WakeVideoTaskPoller()
}

func WakeVideoTaskPoller() {
	videoTaskRunningMu.Lock()
	if videoTaskRunning {
		videoTaskWakePending = true
		videoTaskRunningMu.Unlock()
		return
	}
	videoTaskRunning = true
	videoTaskWakePending = false
	videoTaskRunningMu.Unlock()
	select {
	case videoTaskPollWake <- struct{}{}:
	default:
		videoTaskRunningMu.Lock()
		videoTaskRunning = false
		videoTaskRunningMu.Unlock()
	}
}

func runVideoTaskPoller() {
	inFlight := sync.Map{}
	pollSlots := make(chan struct{}, 16)
	lastCleanupAt := time.Time{}
	lastImageCleanupAt := time.Time{}
	workflowCursorCreatedAt, workflowCursorID := "", ""
	for range videoTaskPollWake {
		for {
			current := time.Now()
			if err := repository.MarkStaleVideoSubmissions(videoTaskTime(current.Add(-3*time.Minute)), videoTaskTime(current)); err != nil {
				log.Printf("mark stale video submissions failed err=%v", err)
			}
			hasComfyTasks, comfyErr := expireComfyBridgeRequests("")
			if comfyErr != nil {
				log.Printf("expire Comfy Bridge requests failed err=%v", comfyErr)
			}
			hasComfyCleanup, cleanupErr := repository.CleanupComfyBridgeRequests(
				current.Add(-videoTaskFinishedRetention),
				current.Add(-comfyBridgeInspectTimeout),
			)
			if cleanupErr != nil {
				log.Printf("cleanup Comfy Bridge requests failed err=%v", cleanupErr)
			}
			tasks, err := repository.ListDueVideoTasks(200)
			if err != nil {
				log.Printf("list due video tasks failed err=%v", err)
				waitForNextVideoTaskPoll()
				continue
			}
			pendingVideoSubmissions, pendingSubmissionErr := repository.HasPendingVideoSubmissions()
			if pendingSubmissionErr != nil {
				log.Printf("check pending video submissions failed err=%v", pendingSubmissionErr)
			}
			workflowTasks, workflowErr := repository.ListDueRunningHubWorkflowTasks(workflowCursorCreatedAt, workflowCursorID, runningHubWorkflowPageSize)
			if workflowErr == nil && len(workflowTasks) == 0 && workflowCursorCreatedAt != "" {
				workflowCursorCreatedAt, workflowCursorID = "", ""
				workflowTasks, workflowErr = repository.ListDueRunningHubWorkflowTasks("", "", runningHubWorkflowPageSize)
			}
			if workflowErr != nil {
				log.Printf("list due RunningHub workflow tasks failed err=%v", workflowErr)
				workflowTasks = nil
			} else if len(workflowTasks) == runningHubWorkflowPageSize {
				last := workflowTasks[len(workflowTasks)-1]
				workflowCursorCreatedAt, workflowCursorID = last.CreatedAt, last.ID
			} else {
				workflowCursorCreatedAt, workflowCursorID = "", ""
			}
			hasImageTasks, imageErr := repository.HasActiveCanvasImageTasks()
			if imageErr != nil {
				log.Printf("check active canvas image tasks failed err=%v", imageErr)
			}
			if len(tasks) == 0 && len(workflowTasks) == 0 && !pendingVideoSubmissions && !hasComfyTasks && !hasComfyCleanup && !hasImageTasks {
				if workflowErr != nil || pendingSubmissionErr != nil || comfyErr != nil || cleanupErr != nil || imageErr != nil {
					waitForNextVideoTaskPoll()
					continue
				}
				videoTaskRunningMu.Lock()
				if videoTaskWakePending {
					videoTaskWakePending = false
					videoTaskRunningMu.Unlock()
					continue
				}
				videoTaskRunning = false
				videoTaskRunningMu.Unlock()
				break
			}
			if (len(tasks) > 0 || len(workflowTasks) > 0) && (lastCleanupAt.IsZero() || current.Sub(lastCleanupAt) >= videoTaskCleanupInterval) {
				if err := repository.DeleteFinishedVideoTasksBefore(videoTaskTime(current.Add(-videoTaskFinishedRetention))); err != nil {
					log.Printf("cleanup finished video tasks failed err=%v", err)
				}
				lastCleanupAt = current
			}
			if hasImageTasks && (lastImageCleanupAt.IsZero() || current.Sub(lastImageCleanupAt) >= videoTaskCleanupInterval) {
				if err := repository.DeleteFinishedCanvasImageTasksBefore(videoTaskTime(current.Add(-videoTaskFinishedRetention))); err != nil {
					log.Printf("cleanup finished canvas image tasks failed err=%v", err)
				}
				lastImageCleanupAt = current
			}
			for _, task := range tasks {
				if _, loaded := inFlight.LoadOrStore(task.ID, true); loaded {
					continue
				}
				go func(task model.VideoTask) {
					defer inFlight.Delete(task.ID)
					pollSlots <- struct{}{}
					defer func() { <-pollSlots }()
					poll := currentVideoTaskPoller()
					if poll == nil {
						return
					}
					update, err := poll(task)
					if err != nil {
						update = VideoTaskPollUpdate{Status: task.Status, ErrorDetail: err.Error(), Retryable: true}
					}
					if err := UpdateVideoTaskFromPoll(task, update); err != nil {
						log.Printf("update video task failed id=%s err=%v", task.ID, err)
					}
				}(task)
			}
			for _, task := range workflowTasks {
				key := "workflow:" + task.ID
				if _, loaded := inFlight.LoadOrStore(key, true); loaded {
					continue
				}
				go func(task repository.RunningHubWorkflowTask) {
					defer inFlight.Delete("workflow:" + task.ID)
					if err := pollRunningHubWorkflowTask(task); err != nil {
						log.Printf("poll RunningHub workflow task failed id=%s err=%v", task.ID, err)
					}
				}(task)
			}
			waitForNextVideoTaskPoll()
		}
	}
}

func currentVideoTaskPoller() VideoTaskPollFunc {
	videoTaskPollerMu.RLock()
	defer videoTaskPollerMu.RUnlock()
	return videoTaskPoller
}

func waitForNextVideoTaskPoll() {
	time.Sleep(videoTaskPollInterval)
}

func UpdateVideoTaskFromPoll(task model.VideoTask, update VideoTaskPollUpdate) error {
	current := time.Now().UTC()
	timestamp := videoTaskTime(current)
	task.UpdatedAt, task.LastPolledAt = timestamp, timestamp
	if update.ResponseBody != "" {
		task.LastResponse = update.ResponseBody
	}
	if update.Retryable {
		task.PollFailures++
		task.ErrorDetail = strings.TrimSpace(update.ErrorDetail)
		if task.PollFailures >= 12 {
			task.Phase, task.Status = model.GenerationRecovery, "failed"
			task.Error = "多次查询上游任务失败；未退款。请核对服务商任务后再决定是否重新生成。"
			task.CompletedAt = timestamp
			task.NextPollAt = ""
		} else {
			task.NextPollAt = videoTaskTime(current.Add(time.Duration(1<<min(task.PollFailures-1, 5)) * 5 * time.Second))
		}
	} else {
		task.PollFailures = 0
		task.NextPollAt = ""
		task.ErrorDetail = strings.TrimSpace(update.ErrorDetail)
		if update.Error != "" {
			task.Error = strings.TrimSpace(update.Error)
		}
		if update.Phase == model.GenerationRetrieving {
			task.PollFailures++
			task.Status = "processing"
			task.Error = ""
			if task.PollFailures >= 12 {
				task.Phase, task.Status = model.GenerationRecovery, "failed"
				task.Error = "视频已生成，但多次读取结果失败；未退款。请核对服务商任务后再决定是否重新生成。"
				task.CompletedAt, task.NextPollAt = timestamp, ""
			} else {
				task.Phase = model.GenerationRetrieving
				task.NextPollAt = videoTaskTime(current.Add(time.Duration(1<<min(task.PollFailures-1, 5)) * 5 * time.Second))
			}
		} else {
			task.Status = NormalizeVideoTaskStatus(firstVideoTaskValue(update.Status, task.Status))
			if task.Status == "" {
				task.Status = "processing"
			}
			if update.Progress > 0 || task.Progress == 0 {
				task.Progress = clampProgress(update.Progress)
			}
			task.Seconds = firstVideoTaskValue(update.Seconds, task.Seconds)
			task.Size = firstVideoTaskValue(update.Size, task.Size)
			task.VideoURL = firstVideoTaskValue(update.VideoURL, task.VideoURL)
			switch {
			case task.VideoURL != "" || IsCompletedVideoTaskStatus(task.Status):
				task.Phase, task.Status, task.Progress, task.CompletedAt = model.GenerationCompleted, "completed", 100, timestamp
				task.Error, task.ErrorDetail = "", ""
			case IsFailedVideoTaskStatus(task.Status):
				task.Phase, task.CompletedAt = model.GenerationFailed, timestamp
				task.Error = firstVideoTaskValue(task.Error, task.ErrorDetail, "视频任务生成失败")
			default:
				task.Phase = model.GenerationRunning
			}
		}
	}
	if task.Revision > 0 {
		expected := task.Revision
		task.Revision++
		_, err := repository.CommitVideoTask(task, expected, nil)
		return err
	}
	_, err := repository.SaveVideoTask(task)
	return err
}

func NormalizeVideoTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done", "succeeded", "success":
		return "completed"
	case "failed", "fail", "error", "cancelled", "canceled":
		return "failed"
	case "running", "processing", "in_progress", "in-progress":
		return "processing"
	case "queued", "queue", "pending", "":
		return "queued"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func IsCompletedVideoTaskStatus(status string) bool {
	return NormalizeVideoTaskStatus(status) == "completed"
}

func IsFailedVideoTaskStatus(status string) bool {
	return NormalizeVideoTaskStatus(status) == "failed"
}

func videoTaskTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func firstVideoTaskValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeVideoTaskSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "workflow":
		return "workflow"
	case "canvas":
		return "canvas"
	case "video-workbench", "":
		return "video-workbench"
	default:
		return "video-workbench"
	}
}

func clampProgress(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
