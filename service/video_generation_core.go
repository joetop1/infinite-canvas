package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

var ErrVideoTaskKeyConflict = errors.New("同一个视频任务编号对应了不同请求；请刷新页面后再生成")

// VideoSubmission is the provider-specific outcome normalized by the adapter.
type VideoSubmission struct {
	ParameterTranslationSnapshot string
	UpstreamTaskID, UpstreamVideoID  string
	Status                           string
	Progress                         int
	Seconds, Size, VideoURL          string
	ResponseBody, Error, ErrorDetail string
	Accepted, Rejected               bool
}

type VideoGenerationInput struct {
	ParameterTranslationSnapshot string
	UserID, UserDisplayName, Model, ChannelID, UserChannelID, ChannelName string
	Source, SourceID, ClientTaskID, Fingerprint                           string
	Seconds, Size, RequestBody, BillingPath                               string
	Credits                                                               float64
}

// SubmitVideoGeneration stores a durable task and reserves credits before invoking
// a provider. Repeating the client key never repeats the paid provider request.
func SubmitVideoGeneration(ctx context.Context, input VideoGenerationInput, submit func(context.Context) (VideoSubmission, error)) (model.VideoTask, bool, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ClientTaskID = strings.TrimSpace(input.ClientTaskID)
	if input.UserID == "" || input.ClientTaskID == "" || input.Fingerprint == "" || submit == nil {
		return model.VideoTask{}, false, errors.New("视频任务信息不完整")
	}
	// Older releases used the client key itself as the row ID; recognize those
	// records during rollout so a transport retry does not create a second job.
	if previous, found, err := repository.GetUserVideoTask(input.UserID, input.ClientTaskID); err != nil {
		return model.VideoTask{}, false, err
	} else if found && previous.ClientTaskID == input.ClientTaskID && previous.RequestFingerprint == "" {
		return previous, false, nil
	}
	key := sha256.Sum256([]byte(input.UserID + "\x00" + input.ClientTaskID))
	id := "video-task-" + hex.EncodeToString(key[:])
	if previous, found, err := repository.GetVideoTaskByID(id); err != nil {
		return model.VideoTask{}, false, err
	} else if found {
		if previous.RequestFingerprint != input.Fingerprint {
			return model.VideoTask{}, false, ErrVideoTaskKeyConflict
		}
		return previous, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := model.VideoTask{
		ID: id, ClientTaskID: input.ClientTaskID, RequestFingerprint: input.Fingerprint,
		ParameterTranslationSnapshot: input.ParameterTranslationSnapshot,
		UserID: input.UserID, UserDisplayName: input.UserDisplayName, Model: input.Model,
		ChannelID: input.ChannelID, UserChannelID: input.UserChannelID, ChannelName: input.ChannelName,
		Source: normalizeVideoTaskSource(input.Source), SourceID: strings.TrimSpace(input.SourceID),
		Status: "queued", Phase: model.GenerationSubmitting, Revision: 1,
		Seconds: input.Seconds, Size: input.Size, RequestBody: input.RequestBody,
		Credits: normalizeCredits(input.Credits), CreatedAt: now, UpdatedAt: now,
	}
	logExtra, _ := json.Marshal(map[string]string{"model": input.Model, "path": input.BillingPath})
	creditLog := model.CreditLog{ID: newID("credit"), UserID: input.UserID, Type: model.CreditLogTypeAIConsume,
		Amount: -task.Credits, Remark: "调用模型 " + input.Model, Extra: string(logExtra), CreatedAt: now}
	if err := repository.ReserveVideoTask(task, creditLog); err != nil {
		if previous, found, readErr := repository.GetVideoTaskByID(id); readErr == nil && found {
			if previous.RequestFingerprint != input.Fingerprint {
				return model.VideoTask{}, false, ErrVideoTaskKeyConflict
			}
			return previous, false, nil
		}
		return model.VideoTask{}, false, err
	}

	// A disconnected browser must not cancel an already-reserved provider submission.
	requestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	result, submitErr := submit(requestCtx)
	if result.ParameterTranslationSnapshot != "" {
		task.ParameterTranslationSnapshot = result.ParameterTranslationSnapshot
	}
	task.UpstreamTaskID, task.UpstreamVideoID = strings.TrimSpace(result.UpstreamTaskID), strings.TrimSpace(result.UpstreamVideoID)
	task.Progress, task.Seconds, task.Size = clampProgress(result.Progress), firstVideoTaskValue(result.Seconds, task.Seconds), firstVideoTaskValue(result.Size, task.Size)
	task.VideoURL, task.ResponseBody = strings.TrimSpace(result.VideoURL), result.ResponseBody
	task.UpdatedAt, task.Revision = time.Now().UTC().Format(time.RFC3339Nano), task.Revision+1
	var refund *model.CreditLog
	switch {
	case submitErr != nil:
		task.Phase, task.Status = model.GenerationUncertain, "failed"
		task.Error = "提交结果无法确认；系统未自动重发。请先查询上游任务状态，再决定是否重新生成。"
		task.ErrorDetail = submitErr.Error()
		task.CompletedAt = task.UpdatedAt
	case result.Rejected:
		task.Phase, task.Status = model.GenerationRejected, "failed"
		task.Error = firstVideoTaskValue(result.Error, result.ErrorDetail, "上游拒绝了视频请求")
		task.ErrorDetail, task.CompletedAt = result.ErrorDetail, task.UpdatedAt
		if task.Credits > 0 {
			refundExtra, _ := json.Marshal(map[string]string{"model": input.Model, "path": input.BillingPath})
			refund = &model.CreditLog{ID: newID("credit"), UserID: input.UserID, Type: model.CreditLogTypeAIRefund,
				Amount: task.Credits, Remark: "上游拒绝请求返还 " + input.Model, Extra: string(refundExtra), CreatedAt: task.UpdatedAt}
			task.RefundedAt = task.UpdatedAt
		}
	case !result.Accepted || (task.UpstreamTaskID == "" && task.UpstreamVideoID == "" && task.VideoURL == ""):
		task.Phase, task.Status = model.GenerationUncertain, "failed"
		task.Error = "上游已收到或可能已收到请求，但未返回可追踪编号。系统未重发以避免重复扣费。"
		task.ErrorDetail, task.CompletedAt = result.ErrorDetail, task.UpdatedAt
	default:
		task.Status = NormalizeVideoTaskStatus(result.Status)
		if task.Status == "" {
			task.Status = "processing"
		}
		switch task.Status {
		case "completed":
			task.Phase, task.Progress, task.CompletedAt = model.GenerationCompleted, 100, task.UpdatedAt
		case "failed":
			task.Phase, task.Error, task.CompletedAt = model.GenerationFailed, firstVideoTaskValue(result.Error, result.ErrorDetail, "视频生成失败"), task.UpdatedAt
		default:
			task.Phase = model.GenerationRunning
			task.StartedAt = task.UpdatedAt
		}
		task.Error, task.ErrorDetail = result.Error, result.ErrorDetail
	}
	changed, err := repository.CommitVideoTask(task, 1, refund)
	if err != nil {
		return task, true, fmt.Errorf("视频任务已保存，但更新提交状态失败：%w", err)
	}
	if !changed {
		if saved, ok, readErr := repository.GetVideoTaskByID(task.ID); readErr == nil && ok {
			task = saved
		}
	}
	if task.Phase == model.GenerationRunning {
		WakeVideoTaskPoller()
	}
	return task, true, nil
}
