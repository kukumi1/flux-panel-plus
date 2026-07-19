package service

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"time"
)

// WriteSystemLog appends an entry to the system event timeline. Failures are
// swallowed on purpose: logging must never break the caller's control flow.
func WriteSystemLog(logType, level, message string) {
	DB.Create(&model.SystemLog{
		Type:        logType,
		Level:       level,
		Message:     message,
		CreatedTime: time.Now().UnixMilli(),
	})
}

func GetSystemLogs(logType string, limit int) dto.R {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := DB.Model(&model.SystemLog{})
	if logType != "" {
		query = query.Where("type = ?", logType)
	}
	var logs []model.SystemLog
	query.Order("id DESC").Limit(limit).Find(&logs)
	return dto.Ok(logs)
}
