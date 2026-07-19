package service

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"sync"
	"time"
)

type RouteHopView struct {
	model.RouteHop
	NodeName string `json:"nodeName"`
}

type RouteWithHops struct {
	model.Route
	Hops    []RouteHopView `json:"hops"`
	Latency *float64       `json:"latency"` // inter-node RTT in ms across the hop chain; nil if unmeasured or last probe failed
}

// routeLatencyCache holds the latest inter-node latency probe per route, kept in
// memory because the route view only needs the current value, not history.
var routeLatencyCache sync.Map // routeId(int64) -> routeLatencySample

type routeLatencySample struct {
	latency    float64
	success    bool
	recordTime int64
}

// SetRouteLatency records the latest inter-node latency probe for a route.
func SetRouteLatency(routeId int64, latency float64, success bool, recordTime int64) {
	routeLatencyCache.Store(routeId, routeLatencySample{latency: latency, success: success, recordTime: recordTime})
}

func getRouteLatency(routeId int64) (float64, bool) {
	v, ok := routeLatencyCache.Load(routeId)
	if !ok {
		return 0, false
	}
	sample := v.(routeLatencySample)
	return sample.latency, sample.success
}

func CreateRoute(d dto.RouteDto) dto.R {
	nodes, errMsg := validateRouteNodes(d.NodeIds)
	if errMsg != "" {
		return dto.Err(errMsg)
	}

	var count int64
	DB.Model(&model.Route{}).Where("name = ?", d.Name).Count(&count)
	if count > 0 {
		return dto.Err("route name already exists")
	}

	protocol := normalizeRouteProtocol(d.Protocol)
	tcpListenAddr, udpListenAddr := normalizeRouteListenAddrs(d.TcpListenAddr, d.UdpListenAddr)

	var maxInx int
	DB.Model(&model.Route{}).Select("COALESCE(MAX(inx), 0)").Scan(&maxInx)

	now := time.Now().UnixMilli()
	route := model.Route{
		Name:          d.Name,
		Protocol:      protocol,
		TcpListenAddr: tcpListenAddr,
		UdpListenAddr: udpListenAddr,
		InterfaceName: d.InterfaceName,
		Status:        1,
		Inx:           maxInx + 1,
		CreatedTime:   now,
		UpdatedTime:   now,
	}

	tx := DB.Begin()
	if err := tx.Create(&route).Error; err != nil {
		tx.Rollback()
		return dto.Err("failed to create route")
	}

	for i, node := range nodes {
		hop := model.RouteHop{
			RouteId:     route.ID,
			HopOrder:    i,
			NodeId:      node.ID,
			NodeIp:      node.ServerIp,
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := tx.Create(&hop).Error; err != nil {
			tx.Rollback()
			return dto.Err("failed to create route hop")
		}
	}

	if err := tx.Commit().Error; err != nil {
		return dto.Err("failed to create route")
	}

	return dto.Ok(route)
}

func GetAllRoutes() dto.R {
	routes, errMsg := loadRoutesWithHops()
	if errMsg != "" {
		return dto.Err(errMsg)
	}
	return dto.Ok(routes)
}

func UpdateRoute(d dto.RouteUpdateDto) dto.R {
	var route model.Route
	if err := DB.First(&route, d.ID).Error; err != nil {
		return dto.Err("route not found")
	}

	var nameCount int64
	DB.Model(&model.Route{}).Where("name = ? AND id != ?", d.Name, d.ID).Count(&nameCount)
	if nameCount > 0 {
		return dto.Err("route name already exists")
	}

	var forwardCount int64
	DB.Model(&model.Forward{}).Where("route_id = ?", d.ID).Count(&forwardCount)
	if forwardCount > 0 {
		updates := map[string]interface{}{
			"name":         d.Name,
			"updated_time": time.Now().UnixMilli(),
		}
		if d.Status != nil {
			updates["status"] = *d.Status
		}
		if err := DB.Model(&route).Updates(updates).Error; err != nil {
			return dto.Err("failed to update route")
		}
		return dto.Ok("route updated")
	}

	nodes, errMsg := validateRouteNodes(d.NodeIds)
	if errMsg != "" {
		return dto.Err(errMsg)
	}

	protocol := normalizeRouteProtocol(d.Protocol)
	tcpListenAddr, udpListenAddr := normalizeRouteListenAddrs(d.TcpListenAddr, d.UdpListenAddr)
	now := time.Now().UnixMilli()

	tx := DB.Begin()
	if err := tx.Model(&route).Updates(map[string]interface{}{
		"name":            d.Name,
		"protocol":        protocol,
		"tcp_listen_addr": tcpListenAddr,
		"udp_listen_addr": udpListenAddr,
		"interface_name":  d.InterfaceName,
		"updated_time":    now,
	}).Error; err != nil {
		tx.Rollback()
		return dto.Err("failed to update route")
	}
	if d.Status != nil {
		tx.Model(&route).Update("status", *d.Status)
	}
	if err := tx.Where("route_id = ?", d.ID).Delete(&model.RouteHop{}).Error; err != nil {
		tx.Rollback()
		return dto.Err("failed to update route hops")
	}
	for i, node := range nodes {
		hop := model.RouteHop{RouteId: d.ID, HopOrder: i, NodeId: node.ID, NodeIp: node.ServerIp, CreatedTime: now, UpdatedTime: now}
		if err := tx.Create(&hop).Error; err != nil {
			tx.Rollback()
			return dto.Err("failed to update route hop")
		}
	}
	if err := tx.Commit().Error; err != nil {
		return dto.Err("failed to update route")
	}
	return dto.Ok("route updated")
}

func DeleteRoute(id int64) dto.R {
	var route model.Route
	if err := DB.First(&route, id).Error; err != nil {
		return dto.Err("route not found")
	}
	var count int64
	DB.Model(&model.Forward{}).Where("route_id = ?", id).Count(&count)
	if count > 0 {
		return dto.Err("route has forwards; delete forwards first")
	}
	tx := DB.Begin()
	if err := tx.Where("route_id = ?", id).Delete(&model.RouteHop{}).Error; err != nil {
		tx.Rollback()
		return dto.Err("failed to delete route hops")
	}
	if err := tx.Delete(&route).Error; err != nil {
		tx.Rollback()
		return dto.Err("failed to delete route")
	}
	if err := tx.Commit().Error; err != nil {
		return dto.Err("failed to delete route")
	}
	return dto.Ok("route deleted")
}

func LoadRouteWithHops(routeId int64) (*model.Route, []model.RouteHop, string) {
	var route model.Route
	if err := DB.First(&route, routeId).Error; err != nil {
		return nil, nil, "route not found"
	}
	var hops []model.RouteHop
	if err := DB.Where("route_id = ?", routeId).Order("hop_order ASC").Find(&hops).Error; err != nil {
		return nil, nil, "route hops not found"
	}
	if len(hops) < 2 {
		return nil, nil, "route requires at least two nodes"
	}
	return &route, hops, ""
}

func validateRouteNodes(nodeIds []int64) ([]model.Node, string) {
	if len(nodeIds) < 2 {
		return nil, "route requires at least two nodes"
	}
	seen := make(map[int64]bool, len(nodeIds))
	nodes := make([]model.Node, 0, len(nodeIds))
	for _, nodeId := range nodeIds {
		if nodeId <= 0 {
			return nil, "invalid route node"
		}
		if seen[nodeId] {
			return nil, "route nodes must be unique"
		}
		seen[nodeId] = true
		var node model.Node
		if err := DB.First(&node, nodeId).Error; err != nil {
			return nil, "route node not found"
		}
		nodes = append(nodes, node)
	}
	return nodes, ""
}

func loadRoutesWithHops() ([]RouteWithHops, string) {
	var routes []model.Route
	if err := DB.Order("inx ASC, created_time DESC").Find(&routes).Error; err != nil {
		return nil, "failed to list routes"
	}
	result := make([]RouteWithHops, 0, len(routes))
	for _, route := range routes {
		var hops []RouteHopView
		if err := DB.Table("route_hop").Select("route_hop.*, node.name as node_name").Joins("LEFT JOIN node ON node.id = route_hop.node_id").Where("route_hop.route_id = ?", route.ID).Order("route_hop.hop_order ASC").Scan(&hops).Error; err != nil {
			return nil, "failed to list route hops"
		}
		rw := RouteWithHops{Route: route, Hops: hops}
		if latency, ok := getRouteLatency(route.ID); ok {
			rw.Latency = &latency
		}
		result = append(result, rw)
	}
	return result, ""
}

func normalizeRouteProtocol(protocol string) string {
	if protocol == "" {
		return "tls"
	}
	return protocol
}

func normalizeRouteListenAddrs(tcpListenAddr string, udpListenAddr string) (string, string) {
	if tcpListenAddr == "" {
		tcpListenAddr = "::"
	}
	if udpListenAddr == "" {
		udpListenAddr = "::"
	}
	return tcpListenAddr, udpListenAddr
}
