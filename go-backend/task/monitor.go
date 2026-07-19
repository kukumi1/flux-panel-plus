package task

import (
	"encoding/json"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
	"flux-panel/go-backend/service"
	"strconv"
	"strings"
	"sync"
	"time"
)

func StartLatencyMonitor() {
	go func() {
		// Wait for DB and WS to be ready
		time.Sleep(10 * time.Second)

		for {
			interval := getMonitorInterval()
			runLatencyCheck()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
}

func getMonitorInterval() int {
	var cfg model.ViteConfig
	if err := service.DB.Where("name = ?", "monitor_interval").First(&cfg).Error; err == nil {
		if v, err := strconv.Atoi(cfg.Value); err == nil && v > 0 {
			return v
		}
	}
	return 60
}

type tcpCheck struct {
	nodeId int64
	ip     string
	port   int
}

type checkTask struct {
	forward    model.Forward
	checks     []tcpCheck
	targetAddr string
}

func runLatencyCheck() {
	if pkg.WS == nil {
		return
	}

	var forwards []model.Forward
	service.DB.Where("status = 1").Find(&forwards)

	if len(forwards) == 0 {
		return
	}

	now := time.Now().Unix()

	// Pre-load tunnels to avoid repeated DB queries.
	tunnelMap := make(map[int64]*model.Tunnel)
	for _, f := range forwards {
		if f.RouteId != 0 || f.TunnelId == 0 {
			continue
		}
		if _, ok := tunnelMap[f.TunnelId]; !ok {
			var tunnel model.Tunnel
			if err := service.DB.First(&tunnel, f.TunnelId).Error; err == nil {
				tunnelMap[f.TunnelId] = &tunnel
			}
		}
	}

	var tasks []checkTask
	for _, f := range forwards {
		if f.RouteId != 0 {
			tasks = append(tasks, buildRouteLatencyTasks(f)...)
			continue
		}

		tunnel, ok := tunnelMap[f.TunnelId]
		if !ok {
			continue
		}
		nodeId := tunnel.InNodeId
		if !pkg.WS.IsNodeOnline(nodeId) {
			continue
		}
		for _, addr := range splitRemoteAddrs(f.RemoteAddr) {
			targetIp := extractIp(addr)
			targetPort := extractPort(addr)
			if targetIp == "" || targetPort <= 0 {
				continue
			}
			tasks = append(tasks, checkTask{
				forward:    f,
				targetAddr: addr,
				checks:     []tcpCheck{{nodeId: nodeId, ip: targetIp, port: targetPort}},
			})
		}
	}

	if len(tasks) == 0 {
		return
	}

	// Run checks concurrently with a concurrency limit.
	concurrency := 10
	if len(tasks) < concurrency {
		concurrency = len(tasks)
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(ct checkTask) {
			defer wg.Done()
			defer func() { <-sem }()

			record := model.MonitorLatency{
				ForwardId:  ct.forward.ID,
				TargetAddr: ct.targetAddr,
				Latency:    -1,
				Success:    false,
				RecordTime: now,
			}
			if len(ct.checks) > 0 {
				record.NodeId = ct.checks[0].nodeId
			}

			totalLatency := 0.0
			success := len(ct.checks) > 0
			for _, check := range ct.checks {
				latency, ok := runTcpPingCheck(check)
				if !ok {
					success = false
					break
				}
				totalLatency += latency
			}
			if success {
				record.Success = true
				record.Latency = totalLatency
			}

			service.DB.Create(&record)
		}(t)
	}
	wg.Wait()
}

func buildRouteLatencyTasks(f model.Forward) []checkTask {
	var hops []model.RouteHop
	if err := service.DB.Where("route_id = ?", f.RouteId).Order("hop_order ASC").Find(&hops).Error; err != nil || len(hops) < 2 {
		return nil
	}

	lastHop := hops[len(hops)-1]
	if !pkg.WS.IsNodeOnline(lastHop.NodeId) {
		return nil
	}

	tasks := make([]checkTask, 0)
	for _, addr := range splitRemoteAddrs(f.RemoteAddr) {
		targetIp := extractIp(addr)
		targetPort := extractPort(addr)
		if targetIp == "" || targetPort <= 0 {
			continue
		}
		tasks = append(tasks, checkTask{
			forward:    f,
			targetAddr: addr,
			checks:     []tcpCheck{{nodeId: lastHop.NodeId, ip: targetIp, port: targetPort}},
		})
	}
	return tasks
}

func splitRemoteAddrs(remoteAddr string) []string {
	parts := strings.Split(remoteAddr, ",")
	result := make([]string, 0, len(parts))
	for _, addr := range parts {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			result = append(result, addr)
		}
	}
	return result
}

func runTcpPingCheck(check tcpCheck) (float64, bool) {
	tcpPingData := map[string]interface{}{
		"ip":      check.ip,
		"port":    check.port,
		"count":   2,
		"timeout": 3000,
	}

	result := pkg.WS.SendMsg(check.nodeId, tcpPingData, "TcpPing")
	if result == nil || result.Msg != "OK" || result.Data == nil {
		return 0, false
	}
	dataBytes, err := json.Marshal(result.Data)
	if err != nil {
		return 0, false
	}
	var tcpPingResp struct {
		Success     bool    `json:"success"`
		AverageTime float64 `json:"averageTime"`
	}
	if err := json.Unmarshal(dataBytes, &tcpPingResp); err != nil || !tcpPingResp.Success {
		return 0, false
	}
	return tcpPingResp.AverageTime, true
}

// StartRouteLatencyMonitor periodically measures the inter-node RTT along each
// enabled route (each hop probing the next hop) and caches the latest value so
// the route view can show latency between routed nodes.
func StartRouteLatencyMonitor() {
	go func() {
		time.Sleep(15 * time.Second)
		for {
			interval := getMonitorInterval()
			runRouteLatencyCheck()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
}

func runRouteLatencyCheck() {
	if pkg.WS == nil {
		return
	}

	var routes []model.Route
	service.DB.Where("status = 1").Find(&routes)
	now := time.Now().Unix()

	for _, route := range routes {
		var hops []model.RouteHop
		if err := service.DB.Where("route_id = ?", route.ID).Order("hop_order ASC").Find(&hops).Error; err != nil || len(hops) < 2 {
			continue
		}

		total := 0.0
		success := true
		for i := 0; i < len(hops)-1; i++ {
			src := hops[i]
			dst := hops[i+1]
			if !pkg.WS.IsNodeOnline(src.NodeId) {
				success = false
				break
			}
			relayPort := lookupRouteRelayPort(route.ID, dst.NodeId)
			if relayPort <= 0 || dst.NodeIp == "" {
				success = false
				break
			}
			latency, ok := runTcpPingCheck(tcpCheck{nodeId: src.NodeId, ip: dst.NodeIp, port: relayPort})
			if !ok {
				success = false
				break
			}
			total += latency
		}
		service.SetRouteLatency(route.ID, total, success, now)
	}
}

// lookupRouteRelayPort returns the port a downstream hop node listens on for the
// route (allocated when a forward uses it); 0 when no active forward exists.
func lookupRouteRelayPort(routeId int64, nodeId int64) int {
	var frp model.ForwardRoutePort
	if err := service.DB.Where("route_id = ? AND node_id = ?", routeId, nodeId).
		Order("id DESC").First(&frp).Error; err != nil {
		return 0
	}
	return frp.RelayPort
}

func extractIp(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if strings.HasPrefix(address, "[") {
		closeBracket := strings.Index(address, "]")
		if closeBracket > 1 {
			return address[1:closeBracket]
		}
	}
	lastColon := strings.LastIndex(address, ":")
	if lastColon > 0 {
		return address[:lastColon]
	}
	return address
}

func extractPort(address string) int {
	address = strings.TrimSpace(address)
	if address == "" {
		return -1
	}
	if strings.HasPrefix(address, "[") {
		closeBracket := strings.Index(address, "]")
		if closeBracket > 1 && closeBracket+1 < len(address) && address[closeBracket+1] == ':' {
			portStr := address[closeBracket+2:]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return -1
			}
			return port
		}
	}
	lastColon := strings.LastIndex(address, ":")
	if lastColon > 0 && lastColon+1 < len(address) {
		port, err := strconv.Atoi(address[lastColon+1:])
		if err != nil {
			return -1
		}
		return port
	}
	return -1
}
