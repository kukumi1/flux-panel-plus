package model

type StatisticsXrayFlow struct {
	ID         int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	InboundId  int64 `gorm:"column:inbound_id;index" json:"inboundId"`
	UpFlow     int64 `gorm:"column:up_flow" json:"upFlow"`
	DownFlow   int64 `gorm:"column:down_flow" json:"downFlow"`
	RecordTime int64 `gorm:"column:record_time;index" json:"recordTime"`
}

func (StatisticsXrayFlow) TableName() string {
	return "statistics_xray_flow"
}
