package service

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
	"fmt"
	"log"
	"strconv"
	"strings"
)

func CleanNodeConfigs(nodeIdStr string, gostConfig dto.GostConfigDto) {
	nodeId, err := strconv.ParseInt(nodeIdStr, 10, 64)
	if err != nil {
		return
	}

	// Get all tunnels that use this node as inNode or outNode
	var tunnels []model.Tunnel
	DB.Where("in_node_id = ? OR out_node_id = ?", nodeId, nodeId).Find(&tunnels)

	// Build set of valid service names, chain names, and limiter names
	validServices := make(map[string]bool)
	validChains := make(map[string]bool)
	validLimiters := make(map[string]bool)

	for _, tunnel := range tunnels {
		var forwards []model.Forward
		DB.Where("tunnel_id = ?", tunnel.ID).Find(&forwards)

		for _, fwd := range forwards {
			// Get user tunnel for service name
			var ut model.UserTunnel
			utId := int64(0)
			DB.Where("user_id = ? AND tunnel_id = ?", fwd.UserId, fwd.TunnelId).Limit(1).Find(&ut)
			if ut.ID != 0 {
				utId = ut.ID
			}

			serviceName := strconv.FormatInt(fwd.ID, 10) + "_" + strconv.FormatInt(fwd.UserId, 10) + "_" + strconv.FormatInt(utId, 10)

			if tunnel.InNodeId == nodeId {
				if fwd.ListenIp != "" && strings.Contains(fwd.ListenIp, ",") {
					ips := strings.Split(fwd.ListenIp, ",")
					for i := range ips {
						suffix := fmt.Sprintf("_%d", i)
						validServices[serviceName+suffix+"_tcp"] = true
						validServices[serviceName+suffix+"_udp"] = true
					}
				} else {
					validServices[serviceName+"_tcp"] = true
					validServices[serviceName+"_udp"] = true
				}
				if tunnel.Type == 2 {
					validChains[serviceName+"_chains"] = true
				}
			}
			if tunnel.OutNodeId == nodeId && tunnel.Type == 2 {
				validServices[serviceName+"_tls"] = true
			}

			// Limiter
			if ut.SpeedId != nil && *ut.SpeedId > 0 {
				validLimiters[strconv.FormatInt(*ut.SpeedId, 10)] = true
			}
		}
	}

	// Include multi-hop route forward services and chains.
	var routeForwards []model.Forward
	DB.Table("forward").Select("DISTINCT forward.*").Joins("JOIN route_hop ON route_hop.route_id = forward.route_id").Where("forward.route_id != 0 AND route_hop.node_id = ?", nodeId).Scan(&routeForwards)
	for _, fwd := range routeForwards {
		serviceName := strconv.FormatInt(fwd.ID, 10) + "_" + strconv.FormatInt(fwd.UserId, 10) + "_0"
		var hops []model.RouteHop
		DB.Where("route_id = ?", fwd.RouteId).Order("hop_order ASC").Find(&hops)
		var ports []model.ForwardRoutePort
		DB.Where("forward_id = ?", fwd.ID).Find(&ports)

		for i, hop := range hops {
			if hop.NodeId == nodeId {
				if i == 0 {
					validServices[serviceName+"_tcp"] = true
					validServices[serviceName+"_udp"] = true
				}
				if i < len(hops)-1 {
					validChains[routeChainName(serviceName, i)] = true
				}
			}
		}
		for _, port := range ports {
			if port.NodeId == nodeId {
				validServices[routeRelayServiceName(serviceName, port.HopOrder)] = true
			}
		}
	}

	// Clean orphaned services
	for _, svc := range gostConfig.Services {
		if !validServices[svc.Name] {
			log.Printf("清理孤儿服务: %s on node %d", svc.Name, nodeId)
			// Determine if it's a single service or tcp/udp pair
			if strings.HasSuffix(svc.Name, "_tls") {
				baseName := strings.TrimSuffix(svc.Name, "_tls")
				pkg.DeleteRemoteService(nodeId, baseName)
			} else if strings.HasSuffix(svc.Name, "_tcp") || strings.HasSuffix(svc.Name, "_udp") {
				baseName := svc.Name[:len(svc.Name)-4]
				pkg.DeleteService(nodeId, baseName)
			}
		}
	}

	// Clean orphaned chains
	for _, chain := range gostConfig.Chains {
		if !validChains[chain.Name] {
			log.Printf("清理孤儿链: %s on node %d", chain.Name, nodeId)
			baseName := strings.TrimSuffix(chain.Name, "_chains")
			pkg.DeleteChains(nodeId, baseName)
		}
	}

	// Clean orphaned limiters
	for _, limiter := range gostConfig.Limiters {
		if !validLimiters[limiter.Name] {
			log.Printf("清理孤儿限速器: %s on node %d", limiter.Name, nodeId)
			limiterId, err := strconv.ParseInt(limiter.Name, 10, 64)
			if err == nil {
				pkg.DeleteLimiters(nodeId, limiterId)
			}
		}
	}
}
