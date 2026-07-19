package dto

type RouteDto struct {
	Name          string  `json:"name" binding:"required"`
	NodeIds       []int64 `json:"nodeIds" binding:"required"`
	Protocol      string  `json:"protocol"`
	TcpListenAddr string  `json:"tcpListenAddr"`
	UdpListenAddr string  `json:"udpListenAddr"`
	InterfaceName string  `json:"interfaceName"`
}

type RouteUpdateDto struct {
	ID            int64   `json:"id" binding:"required"`
	Name          string  `json:"name" binding:"required"`
	NodeIds       []int64 `json:"nodeIds" binding:"required"`
	Protocol      string  `json:"protocol"`
	TcpListenAddr string  `json:"tcpListenAddr"`
	UdpListenAddr string  `json:"udpListenAddr"`
	InterfaceName string  `json:"interfaceName"`
	Status        *int    `json:"status"`
}
