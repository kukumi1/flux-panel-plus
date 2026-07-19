package model

type SystemLog struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Type        string `gorm:"column:type;index" json:"type"`
	Level       string `gorm:"column:level" json:"level"`
	Message     string `gorm:"column:message;type:text" json:"message"`
	CreatedTime int64  `gorm:"column:created_time;index" json:"createdTime"`
}

func (SystemLog) TableName() string {
	return "system_log"
}
