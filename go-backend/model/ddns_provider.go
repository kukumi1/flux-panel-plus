package model

type DdnsProvider struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"column:name" json:"name"`
	Type        string `gorm:"column:type" json:"type"`
	Credential  string `gorm:"column:credential;type:text" json:"-"`
	CreatedTime int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime int64  `gorm:"column:updated_time" json:"updatedTime"`
	Status      int    `gorm:"column:status" json:"status"`
}

func (DdnsProvider) TableName() string {
	return "ddns_provider"
}
