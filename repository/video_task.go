package repository

import (
	"errors"
	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm"
	"time"
)

func SaveFailedTranslatedVideoTask(task model.VideoTask, refundLog model.CreditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&model.VideoTask{}).Where("id = ? AND status NOT IN ?", task.ID, []string{"completed", "failed", "cancelled", "canceled"}).Select("*").Updates(&task)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 || refundLog.Amount <= 0 {
			return nil
		}
		user, found, err := refundUserCredits(tx, task.UserID, refundLog.Amount, refundLog.CreatedAt)
		if err != nil {
			return err
		}
		if !found {
			return gorm.ErrRecordNotFound
		}
		refundLog.Balance = user.Credits
		return tx.Create(&refundLog).Error
	})
}

func SaveVideoTask(task model.VideoTask) (model.VideoTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	return task, db.Save(&task).Error
}

func ReserveVideoTask(task model.VideoTask, log model.CreditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if task.Credits > 0 {
			update := tx.Model(&model.User{}).Where("id = ? AND credits >= CAST(? AS DECIMAL(20,2))", task.UserID, task.Credits).Updates(map[string]any{
				"credits": gorm.Expr("ROUND(credits - CAST(? AS DECIMAL(20,2)), 2)", task.Credits), "updated_at": task.CreatedAt,
			})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 0 {
				return errors.New("算力点不足")
			}
			var user model.User
			if err := tx.First(&user, "id = ?", task.UserID).Error; err != nil {
				return err
			}
			log.Balance = user.Credits
			if err := tx.Create(&log).Error; err != nil {
				return err
			}
		}
		return tx.Create(&task).Error
	})
}

func GetVideoTaskByID(id string) (model.VideoTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.VideoTask{}, false, err
	}
	var task model.VideoTask
	err = db.First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return task, false, nil
	}
	return task, err == nil, err
}

func GetVideoTask(id string) (model.VideoTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.VideoTask{}, false, err
	}
	var task model.VideoTask
	err = db.First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.VideoTask{}, false, nil
	}
	if err != nil {
		return model.VideoTask{}, false, err
	}
	return task, true, nil
}

func GetUserVideoTask(userID string, id string) (model.VideoTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.VideoTask{}, false, err
	}
	var task model.VideoTask
	err = db.First(&task, "user_id = ? AND (hidden = ? OR hidden IS NULL) AND (id = ? OR upstream_task_id = ? OR upstream_video_id = ? OR client_task_id = ?)", userID, false, id, id, id, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.VideoTask{}, false, nil
	}
	if err != nil {
		return model.VideoTask{}, false, err
	}
	return task, true, nil
}

func ListUserVideoTasks(userID string, source string, limit int) ([]model.VideoTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.VideoTask
	query := db.Where("user_id = ? AND (hidden = ? OR hidden IS NULL)", userID, false)
	if source != "" {
		if source == "video-workbench" {
			query = query.Where("(source = ? OR source = '' OR source IS NULL)", source)
		} else {
			query = query.Where("source = ?", source)
		}
	}
	err = query.
		Where("(status IN ? OR phase IN ?)", []string{"queued", "in_progress", "processing", "running"}, []string{model.GenerationUncertain, model.GenerationRecovery}).
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// Hiding a card must not erase the idempotency record or interrupt a paid job.
func DeleteUserVideoTask(userID string, id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.VideoTask{}).
		Where("user_id = ? AND (id = ? OR upstream_task_id = ? OR upstream_video_id = ? OR client_task_id = ?)", userID, id, id, id, id).
		Update("hidden", true).Error
}

func ListDueVideoTasks(limit int) ([]model.VideoTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var tasks []model.VideoTask
	err = db.Where("status IN ?", []string{"queued", "in_progress", "processing", "running"}).
		Where("(phase = '' OR phase IS NULL OR phase NOT IN ?)", []string{model.GenerationSubmitting, model.GenerationUncertain, model.GenerationRecovery}).
		Where("(next_poll_at = '' OR next_poll_at IS NULL OR next_poll_at <= ?)", time.Now().UTC().Format(time.RFC3339Nano)).
		Where("(workflow_ref = '' OR workflow_ref IS NULL)").
		Order("COALESCE(last_polled_at, '') ASC").Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

func HasPendingVideoSubmissions() (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var count int64
	err = db.Model(&model.VideoTask{}).Where("phase = ?", model.GenerationSubmitting).Count(&count).Error
	return count > 0, err
}

func MarkStaleVideoSubmissions(before, now string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.VideoTask{}).Where("phase = ? AND updated_at < ?", model.GenerationSubmitting, before).Updates(map[string]any{
		"phase": model.GenerationUncertain, "status": "failed", "error": "提交结果无法确认；系统未自动重发。请先查询上游任务状态，再决定是否重新生成。", "completed_at": now, "updated_at": now, "revision": gorm.Expr("revision + 1"),
	}).Error
}

func DeleteFinishedVideoTasksBefore(before string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.
		Where("completed_at <> ? AND completed_at < ?", "", before).
		Where("status IN ?", []string{"completed", "failed", "cancelled", "canceled"}).
		Where("client_task_id = '' OR client_task_id IS NULL").
		Delete(&model.VideoTask{}).Error
}

// CommitVideoTask uses a revision guard: late polls cannot overwrite a newer state.
// Task state, refund balance and refund ledger are committed together.
func CommitVideoTask(task model.VideoTask, expectedRevision int64, refund *model.CreditLog) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	changed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&model.VideoTask{}).Where("id = ? AND revision = ?", task.ID, expectedRevision).
			Select("phase", "revision", "status", "progress", "seconds", "size", "video_url", "error", "error_detail", "upstream_task_id", "upstream_video_id", "response_body", "last_response", "updated_at", "started_at", "completed_at", "last_polled_at", "next_poll_at", "poll_failures", "refunded_at").Updates(&task)
		if update.Error != nil || update.RowsAffected == 0 {
			return update.Error
		}
		changed = true
		if refund == nil || task.Credits <= 0 {
			return nil
		}
		user, found, err := refundUserCredits(tx, task.UserID, task.Credits, task.UpdatedAt)
		if err != nil {
			return err
		}
		if !found {
			return gorm.ErrRecordNotFound
		}
		refund.Balance = user.Credits
		return tx.Create(refund).Error
	})
	return changed && err == nil, err
}
