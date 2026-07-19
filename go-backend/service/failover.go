package service

import (
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
	"fmt"
	"time"
)

const failoverDebounceMs = int64(60 * 1000)

func memberHealthy(m model.GroupMember) bool {
	return pkg.WS != nil && pkg.WS.IsNodeOnline(m.NodeId)
}

func firstHealthy(members []model.GroupMember) (model.GroupMember, bool) {
	for _, m := range members {
		if memberHealthy(m) {
			return m, true
		}
	}
	return model.GroupMember{}, false
}

func activeGroupMember(groupId int64) (model.GroupMember, bool) {
	var members []model.GroupMember
	DB.Where("group_id = ?", groupId).Order("priority ASC").Find(&members)
	return firstHealthy(members)
}

// EvaluateFailover walks every group once and switches the active member (plus
// its bound DDNS records) when the current member is unhealthy, or — for
// switch-back groups — when a higher-priority member has recovered.
func EvaluateFailover() {
	var groups []model.NodeGroup
	DB.Find(&groups)
	for i := range groups {
		evaluateGroupFailover(&groups[i])
	}
}

func evaluateGroupFailover(g *model.NodeGroup) {
	var members []model.GroupMember
	DB.Where("group_id = ?", g.ID).Order("priority ASC").Find(&members)
	if len(members) == 0 {
		return
	}

	best, ok := firstHealthy(members)
	if !ok {
		return
	}

	if g.SwitchBack != 1 && g.ActiveMemberId != 0 {
		for _, m := range members {
			if m.NodeId == g.ActiveMemberId && memberHealthy(m) {
				return
			}
		}
	}

	if best.NodeId == g.ActiveMemberId {
		return
	}

	now := time.Now().UnixMilli()
	if g.LastSwitchTime != 0 && now-g.LastSwitchTime < failoverDebounceMs {
		return
	}

	applyGroupSwitch(g, best)
}

func applyGroupSwitch(g *model.NodeGroup, target model.GroupMember) {
	g.ActiveMemberId = target.NodeId
	g.LastSwitchTime = time.Now().UnixMilli()
	DB.Model(&model.NodeGroup{}).Where("id = ?", g.ID).Updates(map[string]interface{}{
		"active_member_id": g.ActiveMemberId,
		"last_switch_time": g.LastSwitchTime,
	})

	nodeName := ""
	if node := GetNodeById(target.NodeId); node != nil {
		nodeName = node.Name
	}
	WriteSystemLog("failover", "info", fmt.Sprintf("分组 %s 切换到成员 %s", g.Name, nodeName))

	var domains []model.DdnsDomain
	DB.Where("group_id = ? AND auto_resolve = 1", g.ID).Find(&domains)
	for i := range domains {
		_ = applyDdnsRecord(&domains[i], target)
	}
}
