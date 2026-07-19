package service

import (
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
	"time"

	"gorm.io/gorm"
)

var validGroupTypes = map[string]bool{"entry": true, "exit": true, "forward": true}

func CreateGroup(d dto.GroupDto) dto.R {
	if !validGroupTypes[d.Type] {
		return dto.Err("分组类型无效")
	}

	now := time.Now().UnixMilli()
	group := model.NodeGroup{
		Name:        d.Name,
		Type:        d.Type,
		SwitchBack:  d.SwitchBack,
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		return replaceGroupMembers(tx, group.ID, d.Members)
	})
	if err != nil {
		return dto.Err("创建分组失败")
	}
	return dto.Ok(group)
}

func GetAllGroups() dto.R {
	var groups []model.NodeGroup
	DB.Order("inx ASC, created_time DESC").Find(&groups)

	result := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		result = append(result, map[string]interface{}{
			"id":             g.ID,
			"name":           g.Name,
			"type":           g.Type,
			"switchBack":     g.SwitchBack,
			"activeMemberId": g.ActiveMemberId,
			"lastSwitchTime": g.LastSwitchTime,
			"inx":            g.Inx,
			"members":        groupMembersView(g.ID),
		})
	}
	return dto.Ok(result)
}

func groupMembersView(groupId int64) []map[string]interface{} {
	var members []model.GroupMember
	DB.Where("group_id = ?", groupId).Order("priority ASC").Find(&members)

	view := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		nodeName := ""
		if node := GetNodeById(m.NodeId); node != nil {
			nodeName = node.Name
		}
		view = append(view, map[string]interface{}{
			"id":       m.ID,
			"nodeId":   m.NodeId,
			"nodeName": nodeName,
			"memberIp": m.MemberIp,
			"priority": m.Priority,
			"online":   pkg.WS != nil && pkg.WS.IsNodeOnline(m.NodeId),
		})
	}
	return view
}

func UpdateGroup(d dto.GroupUpdateDto) dto.R {
	var group model.NodeGroup
	if err := DB.First(&group, d.ID).Error; err != nil {
		return dto.Err("分组不存在")
	}

	if d.Name != "" {
		group.Name = d.Name
	}
	if d.SwitchBack != nil {
		group.SwitchBack = *d.SwitchBack
	}
	group.UpdatedTime = time.Now().UnixMilli()

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&group).Error; err != nil {
			return err
		}
		if d.Members != nil {
			return replaceGroupMembers(tx, group.ID, d.Members)
		}
		return nil
	})
	if err != nil {
		return dto.Err("更新分组失败")
	}
	return dto.Ok("更新成功")
}

func DeleteGroup(id int64) dto.R {
	var count int64
	DB.Model(&model.DdnsDomain{}).Where("group_id = ?", id).Count(&count)
	if count > 0 {
		return dto.Err("该分组正在被 DDNS 域名使用，无法删除")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&model.GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.NodeGroup{}, id).Error
	})
	if err != nil {
		return dto.Err("删除分组失败")
	}
	return dto.Ok("删除成功")
}

func UpdateGroupOrder(items []dto.OrderItem) dto.R {
	for _, item := range items {
		DB.Model(&model.NodeGroup{}).Where("id = ?", item.ID).Update("inx", item.Inx)
	}
	return dto.Ok("排序已更新")
}

func replaceGroupMembers(tx *gorm.DB, groupId int64, members []dto.GroupMemberDto) error {
	if err := tx.Where("group_id = ?", groupId).Delete(&model.GroupMember{}).Error; err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, m := range members {
		member := model.GroupMember{
			GroupId:     groupId,
			NodeId:      m.NodeId,
			MemberIp:    m.MemberIp,
			Priority:    m.Priority,
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
	}
	return nil
}

// memberIP resolves the public IP that DNS records should point at for a
// member: an explicit override wins, otherwise the node's server IP.
func memberIP(m model.GroupMember) string {
	if m.MemberIp != "" {
		return m.MemberIp
	}
	if node := GetNodeById(m.NodeId); node != nil {
		if node.ServerIp != "" {
			return node.ServerIp
		}
		return node.Ip
	}
	return ""
}
