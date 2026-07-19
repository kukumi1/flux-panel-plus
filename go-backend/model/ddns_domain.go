package model

type DdnsDomain struct {
	ID              int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderId      int64  `gorm:"column:provider_id;index" json:"providerId"`
	GroupId         int64  `gorm:"column:group_id;index" json:"groupId"`
	Domain          string `gorm:"column:domain" json:"domain"`
	RecordName      string `gorm:"column:record_name" json:"recordName"`
	RecordType      string `gorm:"column:record_type" json:"recordType"`
	AutoResolve     int    `gorm:"column:auto_resolve" json:"autoResolve"`
	CurrentRecord   string `gorm:"column:current_record" json:"currentRecord"`
	CurrentMemberId int64  `gorm:"column:current_member_id" json:"currentMemberId"`
	CreatedTime     int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime     int64  `gorm:"column:updated_time" json:"updatedTime"`
	Status          int    `gorm:"column:status" json:"status"`
}

func (DdnsDomain) TableName() string {
	return "ddns_domain"
}
