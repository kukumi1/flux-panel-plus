package model

type GroupMember struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupId     int64  `gorm:"column:group_id;index" json:"groupId"`
	NodeId      int64  `gorm:"column:node_id" json:"nodeId"`
	MemberIp    string `gorm:"column:member_ip" json:"memberIp"`
	Priority    int    `gorm:"column:priority" json:"priority"`
	CreatedTime int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime int64  `gorm:"column:updated_time" json:"updatedTime"`
}

func (GroupMember) TableName() string {
	return "group_member"
}
