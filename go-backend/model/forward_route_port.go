package model

type ForwardRoutePort struct {
	ID          int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ForwardId   int64 `gorm:"column:forward_id;index" json:"forwardId"`
	RouteId     int64 `gorm:"column:route_id;index" json:"routeId"`
	HopOrder    int   `gorm:"column:hop_order" json:"hopOrder"`
	NodeId      int64 `gorm:"column:node_id;index" json:"nodeId"`
	RelayPort   int   `gorm:"column:relay_port" json:"relayPort"`
	CreatedTime int64 `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime int64 `gorm:"column:updated_time" json:"updatedTime"`
}

func (ForwardRoutePort) TableName() string {
	return "forward_route_port"
}
