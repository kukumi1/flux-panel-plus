package model

type NodeGroup struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string `gorm:"column:name" json:"name"`
	Type           string `gorm:"column:type" json:"type"`
	SwitchBack     int    `gorm:"column:switch_back" json:"switchBack"`
	ActiveMemberId int64  `gorm:"column:active_member_id" json:"activeMemberId"`
	LastSwitchTime int64  `gorm:"column:last_switch_time" json:"lastSwitchTime"`
	CreatedTime    int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime    int64  `gorm:"column:updated_time" json:"updatedTime"`
	Status         int    `gorm:"column:status" json:"status"`
	Inx            int    `gorm:"column:inx" json:"inx"`
}

func (NodeGroup) TableName() string {
	return "node_group"
}
