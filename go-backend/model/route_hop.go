package model

type RouteHop struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	RouteId     int64  `gorm:"column:route_id;index" json:"routeId"`
	HopOrder    int    `gorm:"column:hop_order" json:"hopOrder"`
	NodeId      int64  `gorm:"column:node_id" json:"nodeId"`
	NodeIp      string `gorm:"column:node_ip" json:"nodeIp"`
	CreatedTime int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime int64  `gorm:"column:updated_time" json:"updatedTime"`
}

func (RouteHop) TableName() string {
	return "route_hop"
}
