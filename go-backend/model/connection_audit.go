package model

type ConnectionAudit struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	NodeId      int64  `gorm:"column:node_id;index" json:"nodeId"`
	NodeName    string `gorm:"column:node_name" json:"nodeName"`
	ServiceName string `gorm:"column:service_name;index" json:"serviceName"`
	ForwardId   int64  `gorm:"column:forward_id;index" json:"forwardId"`
	ForwardName string `gorm:"column:forward_name" json:"forwardName"`
	RouteId     int64  `gorm:"column:route_id;index" json:"routeId"`
	RouteName   string `gorm:"column:route_name" json:"routeName"`
	TunnelId    int64  `gorm:"column:tunnel_id;index" json:"tunnelId"`
	TunnelName  string `gorm:"column:tunnel_name" json:"tunnelName"`
	UserId      int64  `gorm:"column:user_id;index" json:"userId"`
	UserName    string `gorm:"column:user_name" json:"userName"`
	ClientAddr  string `gorm:"column:client_addr;index" json:"clientAddr"`
	ClientIp    string `gorm:"column:client_ip;index" json:"clientIp"`
	ClientPort  int    `gorm:"column:client_port" json:"clientPort"`
	ClientEmail string `gorm:"column:client_email;index" json:"clientEmail"`
	SourceType  string `gorm:"column:source_type;index" json:"sourceType"`
	TargetHost  string `gorm:"column:target_host;index" json:"targetHost"`
	TargetPort  int    `gorm:"column:target_port;index" json:"targetPort"`
	TargetAddr  string `gorm:"column:target_addr" json:"targetAddr"`
	Protocol    string `gorm:"column:protocol;index" json:"protocol"`
	UpBytes     int64  `gorm:"column:up_bytes" json:"upBytes"`
	DownBytes   int64  `gorm:"column:down_bytes" json:"downBytes"`
	DurationMs  int64  `gorm:"column:duration_ms" json:"durationMs"`
	StartedTime int64  `gorm:"column:started_time;index" json:"startedTime"`
	EndedTime   int64  `gorm:"column:ended_time;index" json:"endedTime"`
	Error       string `gorm:"column:error" json:"error"`
	CreatedTime int64  `gorm:"column:created_time;index" json:"createdTime"`
}

func (ConnectionAudit) TableName() string {
	return "connection_audit"
}
