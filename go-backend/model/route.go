package model

type Route struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string `gorm:"column:name" json:"name"`
	Protocol      string `gorm:"column:protocol" json:"protocol"`
	TcpListenAddr string `gorm:"column:tcp_listen_addr" json:"tcpListenAddr"`
	UdpListenAddr string `gorm:"column:udp_listen_addr" json:"udpListenAddr"`
	InterfaceName string `gorm:"column:interface_name" json:"interfaceName"`
	CreatedTime   int64  `gorm:"column:created_time" json:"createdTime"`
	UpdatedTime   int64  `gorm:"column:updated_time" json:"updatedTime"`
	Status        int    `gorm:"column:status" json:"status"`
	Inx           int    `gorm:"column:inx" json:"inx"`
}

func (Route) TableName() string {
	return "route"
}
