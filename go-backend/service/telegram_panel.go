package service

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
)

const telegramPanelPageSize = 6

type tgInlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type tgInlineKeyboardMarkup struct {
	InlineKeyboard [][]tgInlineKeyboardButton `json:"inline_keyboard"`
}

type telegramTunnelView struct {
	model.Tunnel
	UserTunnelID int64 `json:"userTunnelId"`
}

type telegramNodeView struct {
	Node   model.Node
	Online bool
	Info   *pkg.NodeSystemInfo
}

type telegramAuditFilter struct {
	ForwardID int64
	NodeID    int64
}

var telegramDiagnosisJobs sync.Map

type telegramDiagnosisJobKey struct {
	ChatID int64
	Kind   string
	ID     int64
}

type telegramForwardDiagnosis struct {
	ForwardID   int64             `json:"forwardId"`
	ForwardName string            `json:"forwardName"`
	TunnelType  string            `json:"tunnelType"`
	Results     []DiagnosisResult `json:"results"`
}

type telegramWizard struct {
	Kind       string
	Step       string
	UserID     int64
	ExpiresAt  time.Time
	TunnelType int
	Tunnel     dto.TunnelDto
	Forward    dto.ForwardDto
}

var telegramWizardState = struct {
	sync.Mutex
	items map[int64]*telegramWizard
}{items: make(map[int64]*telegramWizard)}

const telegramWizardTTL = 15 * time.Minute

func handleTelegramCallback(token string, callback *tgCallbackQuery) {
	if callback == nil {
		return
	}
	go telegramAnswerCallback(token, callback.ID, "")
	if callback.Message == nil || callback.Message.Chat == nil {
		return
	}

	chatID := callback.Message.Chat.ID
	messageID := callback.Message.MessageID
	user, ok := telegramUserByChat(chatID)
	if !ok {
		telegramPresent(token, chatID, messageID, "账号已解绑或绑定失效，请重新发送 /bind <绑定码>。", nil)
		return
	}

	data := strings.TrimSpace(callback.Data)
	if data == "menu" {
		telegramShowMainMenu(token, chatID, messageID, user)
		return
	}
	if data == "usage" {
		telegramPresent(token, chatID, messageID, telegramUsageText(user), telegramBackKeyboard("menu"))
		return
	}
	if data == "login" {
		telegramHandleLogin(token, chatID, user)
		return
	}
	if data == "tn:new" {
		telegramStartTunnelWizard(token, chatID, messageID, user)
		return
	}
	if data == "fw:new" {
		telegramStartForwardWizard(token, chatID, messageID, user)
		return
	}
	if strings.HasPrefix(data, "wiz:") {
		telegramHandleWizardCallback(token, chatID, messageID, user, data)
		return
	}

	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		telegramPresent(token, chatID, messageID, "操作已失效，请返回主菜单重试。", telegramBackKeyboard("menu"))
		return
	}

	switch parts[0] {
	case "tn":
		telegramHandleTunnelCallback(token, chatID, messageID, user, parts)
	case "fw":
		telegramHandleForwardCallback(token, chatID, messageID, user, parts)
	case "nd":
		telegramHandleNodeCallback(token, chatID, messageID, user, parts)
	case "au":
		telegramHandleAuditCallback(token, chatID, messageID, user, parts)
	default:
		telegramPresent(token, chatID, messageID, "操作已失效，请返回主菜单重试。", telegramBackKeyboard("menu"))
	}
}

func telegramWizardRead(chatID, userID int64) (telegramWizard, bool) {
	telegramWizardState.Lock()
	defer telegramWizardState.Unlock()
	wizard, ok := telegramWizardState.items[chatID]
	if !ok || wizard.UserID != userID || time.Now().After(wizard.ExpiresAt) {
		if ok && time.Now().After(wizard.ExpiresAt) {
			delete(telegramWizardState.items, chatID)
		}
		return telegramWizard{}, false
	}
	copy := *wizard
	return copy, true
}

func telegramWizardStart(chatID, userID int64, wizard telegramWizard) {
	wizard.UserID = userID
	wizard.ExpiresAt = time.Now().Add(telegramWizardTTL)
	telegramWizardState.Lock()
	telegramWizardState.items[chatID] = &wizard
	telegramWizardState.Unlock()
}

func telegramWizardClear(chatID int64) {
	telegramWizardState.Lock()
	delete(telegramWizardState.items, chatID)
	telegramWizardState.Unlock()
}

func telegramWizardUpdate(chatID, userID int64, kind, step string, update func(*telegramWizard)) (telegramWizard, bool) {
	telegramWizardState.Lock()
	defer telegramWizardState.Unlock()
	wizard, ok := telegramWizardState.items[chatID]
	if !ok || wizard.UserID != userID || wizard.Kind != kind || (step != "" && wizard.Step != step) || time.Now().After(wizard.ExpiresAt) {
		return telegramWizard{}, false
	}
	update(wizard)
	wizard.ExpiresAt = time.Now().Add(telegramWizardTTL)
	return *wizard, true
}

func telegramCancelWizard(token string, chatID int64, user *model.User) {
	_, ok := telegramWizardRead(chatID, user.ID)
	telegramWizardClear(chatID)
	if ok {
		telegramSend(token, chatID, "已取消当前新增向导。")
	} else {
		telegramSend(token, chatID, "当前没有进行中的新增向导。")
	}
}

func telegramWizardCancelKeyboard() *tgInlineKeyboardMarkup {
	return telegramKeyboard(telegramRow(telegramButton("✖️ 取消", "wiz:cancel")))
}

func telegramWizardPrompt(token string, chatID, messageID int64, text string) {
	telegramPresent(token, chatID, messageID, text, telegramWizardCancelKeyboard())
}

func telegramStartTunnelWizard(token string, chatID, messageID int64, user *model.User) {
	if user.RoleId != adminRoleID {
		telegramPresent(token, chatID, messageID, "只有管理员可以新增隧道。", telegramBackKeyboard("tn:l:0"))
		return
	}
	telegramWizardStart(chatID, user.ID, telegramWizard{Kind: "tunnel", Step: "type"})
	keyboard := telegramKeyboard(
		telegramRow(telegramButton("单节点端口转发", "wiz:tn:type:1")),
		telegramRow(telegramButton("双节点隧道转发", "wiz:tn:type:2")),
		telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
	)
	telegramPresent(token, chatID, messageID, "新增隧道 · 第 1/6 步\n\n请选择隧道类型：", keyboard)
}

func telegramStartForwardWizard(token string, chatID, messageID int64, user *model.User) {
	telegramWizardStart(chatID, user.ID, telegramWizard{Kind: "forward", Step: "mode"})
	if user.RoleId == adminRoleID {
		keyboard := telegramKeyboard(
			telegramRow(telegramButton("使用隧道", "wiz:fw:mode:tunnel"), telegramButton("使用路由", "wiz:fw:mode:route")),
			telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
		)
		telegramPresent(token, chatID, messageID, "新增转发 · 第 1/8 步\n\n请选择转发路径：", keyboard)
		return
	}
	telegramWizardUpdate(chatID, user.ID, "forward", "mode", func(w *telegramWizard) {
		w.Forward = dto.ForwardDto{Strategy: "round", ListenIp: "::"}
		w.Step = "path"
		w.Forward.RouteId = 0
	})
	telegramShowForwardWizardPaths(token, chatID, messageID, user, false)
}

func telegramHandleWizardCallback(token string, chatID, messageID int64, user *model.User, data string) {
	if data == "wiz:cancel" {
		telegramWizardClear(chatID)
		telegramPresent(token, chatID, messageID, "已取消当前新增向导。", telegramBackKeyboard("menu"))
		return
	}
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok {
		telegramPresent(token, chatID, messageID, "新增向导已过期，请重新点击新增。", telegramBackKeyboard("menu"))
		return
	}
	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		telegramPresent(token, chatID, messageID, "向导操作参数无效。", telegramWizardCancelKeyboard())
		return
	}
	if wizard.Kind == "tunnel" {
		telegramHandleTunnelWizardCallback(token, chatID, messageID, user, parts)
		return
	}
	telegramHandleForwardWizardCallback(token, chatID, messageID, user, parts)
}

func telegramHandleTunnelWizardCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	if user.RoleId != adminRoleID {
		telegramCancelWizard(token, chatID, user)
		return
	}
	if len(parts) >= 4 && parts[1] == "tn" && parts[2] == "type" {
		tunnelType, ok := telegramParseInt(parts, 3)
		if !ok || (tunnelType != 1 && tunnelType != 2) {
			return
		}
		_, ok = telegramWizardUpdate(chatID, user.ID, "tunnel", "type", func(w *telegramWizard) {
			w.TunnelType = int(tunnelType)
			w.Tunnel = dto.TunnelDto{Name: "", Type: int(tunnelType), Protocol: "tcp+udp"}
			w.Step = "in_node"
		})
		if ok {
			telegramShowTunnelWizardNodes(token, chatID, messageID, user, "in_node", 0)
		}
		return
	}
	if len(parts) >= 3 && parts[1] == "tn" && parts[2] == "inlist" {
		page, ok := telegramParseInt(parts, 3)
		if ok {
			telegramShowTunnelWizardNodes(token, chatID, messageID, user, "in_node", int(page))
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "tn" && parts[2] == "in" {
		nodeID, ok := telegramParseInt(parts, 3)
		if !ok {
			return
		}
		wizard, ok := telegramWizardUpdate(chatID, user.ID, "tunnel", "in_node", func(w *telegramWizard) {
			w.Tunnel.InNodeId = nodeID
			if w.TunnelType == 1 {
				w.Tunnel.OutNodeId = &nodeID
				w.Step = "name"
			} else {
				w.Step = "out_node"
			}
		})
		if !ok {
			return
		}
		if wizard.TunnelType == 1 {
			telegramPromptTunnelName(token, chatID, messageID)
		} else {
			telegramShowTunnelWizardNodes(token, chatID, messageID, user, "out_node", 0)
		}
		return
	}
	if len(parts) >= 3 && parts[1] == "tn" && parts[2] == "outlist" {
		page, ok := telegramParseInt(parts, 3)
		if ok {
			telegramShowTunnelWizardNodes(token, chatID, messageID, user, "out_node", int(page))
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "tn" && parts[2] == "out" {
		nodeID, ok := telegramParseInt(parts, 3)
		if !ok {
			return
		}
		_, ok = telegramWizardUpdate(chatID, user.ID, "tunnel", "out_node", func(w *telegramWizard) {
			w.Tunnel.OutNodeId = &nodeID
			w.Step = "protocol"
		})
		if ok {
			telegramShowTunnelProtocols(token, chatID, messageID)
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "tn" && parts[2] == "proto" {
		protocol := parts[3]
		allowed := map[string]bool{"tls": true, "mtls": true, "wss": true, "mwss": true, "quic": true, "grpc": true, "ws": true, "mws": true, "kcp": true}
		if !allowed[protocol] {
			return
		}
		_, ok := telegramWizardUpdate(chatID, user.ID, "tunnel", "protocol", func(w *telegramWizard) {
			w.Tunnel.Protocol = protocol
			w.Step = "name"
		})
		if ok {
			telegramPromptTunnelName(token, chatID, messageID)
		}
		return
	}
	if len(parts) >= 3 && parts[1] == "tn" && parts[2] == "confirm" {
		telegramCreateTunnelFromWizard(token, chatID, messageID, user)
		return
	}
}

func telegramShowTunnelWizardNodes(token string, chatID, messageID int64, user *model.User, step string, page int) {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok {
		return
	}
	var nodes []model.Node
	DB.Order("inx ASC, created_time DESC").Find(&nodes)
	if step == "out_node" {
		filtered := nodes[:0]
		for _, node := range nodes {
			if node.ID != wizard.Tunnel.InNodeId {
				filtered = append(filtered, node)
			}
		}
		nodes = filtered
	}
	page, start, end, totalPages := telegramPageBounds(len(nodes), page, telegramPanelPageSize)
	title := "入口节点"
	if step == "out_node" {
		title = "出口节点（已排除入口节点）"
	}
	rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
	for _, node := range nodes[start:end] {
		icon := "⚪"
		if pkg.WS != nil && pkg.WS.IsNodeOnline(node.ID) {
			icon = "🟢"
		}
		rows = append(rows, telegramRow(telegramButton(fmt.Sprintf("%s #%d %s", icon, node.ID, telegramTruncate(node.Name, 24)), fmt.Sprintf("wiz:tn:%s:%d", map[string]string{"in_node": "in", "out_node": "out"}[step], node.ID))))
	}
	callback := fmt.Sprintf("wiz:tn:%slist:%%d", map[string]string{"in_node": "in", "out_node": "out"}[step])
	rows = append(rows, telegramPaginationRow(page, totalPages, callback)...)
	rows = append(rows, telegramRow(telegramButton("✖️ 取消", "wiz:cancel")))
	telegramPresent(token, chatID, messageID, fmt.Sprintf("新增隧道 · 请选择%s（第 %d/%d 页）：", title, page+1, totalPages), telegramKeyboard(rows...))
}

func telegramShowTunnelProtocols(token string, chatID, messageID int64) {
	protocols := []string{"tls", "mtls", "wss", "mwss", "quic", "grpc", "ws", "mws", "kcp"}
	rows := make([][]tgInlineKeyboardButton, 0, 4)
	for i := 0; i < len(protocols); i += 3 {
		end := i + 3
		if end > len(protocols) {
			end = len(protocols)
		}
		row := make([]tgInlineKeyboardButton, 0, end-i)
		for _, protocol := range protocols[i:end] {
			row = append(row, telegramButton(strings.ToUpper(protocol), "wiz:tn:proto:"+protocol))
		}
		rows = append(rows, row)
	}
	rows = append(rows, telegramRow(telegramButton("✖️ 取消", "wiz:cancel")))
	telegramPresent(token, chatID, messageID, "新增隧道 · 选择隧道协议：", telegramKeyboard(rows...))
}

func telegramPromptTunnelName(token string, chatID, messageID int64) {
	telegramWizardPrompt(token, chatID, messageID, "新增隧道 · 请输入隧道名称（名称必须唯一）：")
}

func telegramHandleForwardWizardCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "mode" {
		mode := parts[3]
		if mode != "tunnel" && (mode != "route" || user.RoleId != adminRoleID) {
			return
		}
		_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "mode", func(w *telegramWizard) {
			w.Forward = dto.ForwardDto{Strategy: "round", ListenIp: "::"}
			w.Forward.RouteId = 0
			w.Forward.TunnelId = 0
			w.Step = "path"
			if mode == "route" {
				w.Forward.RouteId = -1
			}
		})
		if ok {
			telegramShowForwardWizardPaths(token, chatID, messageID, user, mode == "route")
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "pathlist" {
		page, ok := telegramParseInt(parts, 3)
		if ok {
			wizard, exists := telegramWizardRead(chatID, user.ID)
			if exists {
				telegramShowForwardWizardPathPage(token, chatID, messageID, user, wizard.Forward.RouteId == -1, int(page))
			}
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "path" {
		pathID, ok := telegramParseInt(parts, 3)
		if !ok {
			return
		}
		wizard, exists := telegramWizardRead(chatID, user.ID)
		if !exists {
			return
		}
		_, ok = telegramWizardUpdate(chatID, user.ID, "forward", "path", func(w *telegramWizard) {
			if wizard.Forward.RouteId == -1 {
				w.Forward.RouteId = pathID
				w.Forward.TunnelId = 0
			} else {
				w.Forward.TunnelId = pathID
				w.Forward.RouteId = 0
			}
			w.Step = "name"
		})
		if ok {
			telegramWizardPrompt(token, chatID, messageID, "新增转发 · 请输入转发名称：")
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "port" {
		if parts[3] == "auto" {
			_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "port_choice", func(w *telegramWizard) {
				w.Forward.InPort = nil
				w.Step = "strategy"
			})
			if ok {
				telegramShowForwardStrategies(token, chatID, messageID)
			}
			return
		}
		if parts[3] == "manual" {
			_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "port_choice", func(w *telegramWizard) { w.Step = "port" })
			if ok {
				telegramWizardPrompt(token, chatID, messageID, "新增转发 · 请输入入口端口（1-65535）：")
			}
			return
		}
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "strategy" {
		allowed := map[string]bool{"round": true, "random": true, "fifo": true, "hash": true}
		if !allowed[parts[3]] {
			return
		}
		_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "strategy", func(w *telegramWizard) { w.Forward.Strategy = parts[3]; w.Step = "listen" })
		if ok {
			telegramShowForwardListenOptions(token, chatID, messageID)
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "listen" {
		listen := map[string]string{"all": "::", "v4": "0.0.0.0"}[parts[3]]
		if parts[3] == "custom" {
			_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "listen", func(w *telegramWizard) { w.Step = "listen_custom" })
			if ok {
				telegramWizardPrompt(token, chatID, messageID, "新增转发 · 请输入监听地址（可多个，用逗号分隔）：")
			}
			return
		}
		if listen == "" {
			return
		}
		_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "listen", func(w *telegramWizard) { w.Forward.ListenIp = listen; w.Step = "interface" })
		if ok {
			telegramShowForwardInterfaceOptions(token, chatID, messageID)
		}
		return
	}
	if len(parts) >= 4 && parts[1] == "fw" && parts[2] == "iface" {
		if parts[3] == "default" {
			_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "interface", func(w *telegramWizard) { w.Forward.InterfaceName = ""; w.Step = "confirm" })
			if ok {
				telegramShowForwardWizardConfirm(token, chatID, messageID, user)
			}
			return
		}
		if parts[3] == "custom" {
			_, ok := telegramWizardUpdate(chatID, user.ID, "forward", "interface", func(w *telegramWizard) { w.Step = "interface_custom" })
			if ok {
				telegramWizardPrompt(token, chatID, messageID, "新增转发 · 请输入出口网卡名或出口 IP：")
			}
		}
		return
	}
	if len(parts) >= 3 && parts[1] == "fw" && parts[2] == "confirm" {
		telegramCreateForwardFromWizard(token, chatID, messageID, user)
	}
}

func telegramShowForwardWizardPaths(token string, chatID, messageID int64, user *model.User, route bool) {
	telegramShowForwardWizardPathPage(token, chatID, messageID, user, route, 0)
}

func telegramShowForwardWizardPathPage(token string, chatID, messageID int64, user *model.User, route bool, page int) {
	if route {
		var routes []model.Route
		DB.Where("status = 1").Order("inx ASC, created_time DESC").Find(&routes)
		page, start, end, totalPages := telegramPageBounds(len(routes), page, telegramPanelPageSize)
		rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
		for _, item := range routes[start:end] {
			rows = append(rows, telegramRow(telegramButton(fmt.Sprintf("#%d %s", item.ID, telegramTruncate(item.Name, 28)), fmt.Sprintf("wiz:fw:path:%d", item.ID))))
		}
		rows = append(rows, telegramPaginationRow(page, totalPages, "wiz:fw:pathlist:%d")...)
		rows = append(rows, telegramRow(telegramButton("✖️ 取消", "wiz:cancel")))
		telegramPresent(token, chatID, messageID, fmt.Sprintf("新增转发 · 请选择路由（第 %d/%d 页）：", page+1, totalPages), telegramKeyboard(rows...))
		return
	}
	tunnels := telegramTunnelsForUser(user)
	active := make([]telegramTunnelView, 0, len(tunnels))
	for _, item := range tunnels {
		if item.Status == tunnelStatusActive {
			active = append(active, item)
		}
	}
	page, start, end, totalPages := telegramPageBounds(len(active), page, telegramPanelPageSize)
	rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
	for _, item := range active[start:end] {
		rows = append(rows, telegramRow(telegramButton(fmt.Sprintf("#%d %s", item.ID, telegramTruncate(item.Name, 28)), fmt.Sprintf("wiz:fw:path:%d", item.ID))))
	}
	rows = append(rows, telegramPaginationRow(page, totalPages, "wiz:fw:pathlist:%d")...)
	rows = append(rows, telegramRow(telegramButton("✖️ 取消", "wiz:cancel")))
	telegramPresent(token, chatID, messageID, fmt.Sprintf("新增转发 · 请选择启用中的隧道（第 %d/%d 页）：", page+1, totalPages), telegramKeyboard(rows...))
}

func telegramShowForwardStrategies(token string, chatID, messageID int64) {
	rows := [][]tgInlineKeyboardButton{
		telegramRow(telegramButton("轮询 round", "wiz:fw:strategy:round"), telegramButton("随机 random", "wiz:fw:strategy:random")),
		telegramRow(telegramButton("故障转移 fifo", "wiz:fw:strategy:fifo"), telegramButton("哈希 hash", "wiz:fw:strategy:hash")),
		telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
	}
	telegramPresent(token, chatID, messageID, "新增转发 · 选择负载策略：", telegramKeyboard(rows...))
}

func telegramShowForwardListenOptions(token string, chatID, messageID int64) {
	telegramPresent(token, chatID, messageID, "新增转发 · 选择监听地址：", telegramKeyboard(
		telegramRow(telegramButton("全部接口 ::", "wiz:fw:listen:all"), telegramButton("仅 IPv4", "wiz:fw:listen:v4")),
		telegramRow(telegramButton("自定义地址", "wiz:fw:listen:custom")),
		telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
	))
}

func telegramShowForwardInterfaceOptions(token string, chatID, messageID int64) {
	telegramPresent(token, chatID, messageID, "新增转发 · 是否指定出口网卡/IP？", telegramKeyboard(
		telegramRow(telegramButton("默认路由", "wiz:fw:iface:default"), telegramButton("自定义", "wiz:fw:iface:custom")),
		telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
	))
}

func telegramHandleWizardText(token string, chatID int64, user *model.User, text string) bool {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok {
		return false
	}
	rawValue := strings.TrimSpace(text)
	value := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawValue, "\r", " "), "\n", " "))
	if value == "" {
		telegramSend(token, chatID, "输入不能为空；发送 /cancel 可取消当前向导。")
		return true
	}
	switch wizard.Step {
	case "name":
		if len([]rune(value)) > 80 {
			telegramSend(token, chatID, "名称不能超过 80 个字符，请重新输入。")
			return true
		}
		if wizard.Kind == "tunnel" {
			_, updated := telegramWizardUpdate(chatID, user.ID, "tunnel", "name", func(w *telegramWizard) { w.Tunnel.Name = value; w.Step = "interface" })
			if updated {
				telegramWizardPrompt(token, chatID, 0, "新增隧道 · 可选：请输入网卡名/IP；直接发送 - 使用默认路由：")
			}
		} else {
			_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "name", func(w *telegramWizard) { w.Forward.Name = value; w.Step = "target" })
			if updated {
				telegramWizardPrompt(token, chatID, 0, "新增转发 · 请输入目标地址，例如 1.2.3.4:8080；多个目标用换行或逗号分隔：")
			}
		}
		return true
	case "interface":
		if value == "-" {
			value = ""
		}
		if wizard.Kind == "tunnel" {
			_, updated := telegramWizardUpdate(chatID, user.ID, "tunnel", "interface", func(w *telegramWizard) { w.Tunnel.InterfaceName = value; w.Step = "confirm" })
			if updated {
				telegramShowTunnelWizardConfirm(token, chatID, 0, user)
			}
		} else {
			_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "interface_custom", func(w *telegramWizard) { w.Forward.InterfaceName = value; w.Step = "confirm" })
			if updated {
				telegramShowForwardWizardConfirm(token, chatID, 0, user)
			}
		}
		return true
	case "target":
		remote, err := telegramNormalizeRemoteAddr(rawValue)
		if err != "" {
			telegramSend(token, chatID, err)
			return true
		}
		_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "target", func(w *telegramWizard) { w.Forward.RemoteAddr = remote; w.Step = "port_choice" })
		if updated {
			telegramPresent(token, chatID, 0, "新增转发 · 选择入口端口：", telegramKeyboard(
				telegramRow(telegramButton("自动分配", "wiz:fw:port:auto"), telegramButton("手动输入", "wiz:fw:port:manual")),
				telegramRow(telegramButton("✖️ 取消", "wiz:cancel")),
			))
		}
		return true
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			telegramSend(token, chatID, "端口必须是 1-65535 的整数，请重新输入。")
			return true
		}
		_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "port", func(w *telegramWizard) { w.Forward.InPort = &port; w.Step = "strategy" })
		if updated {
			telegramShowForwardStrategies(token, chatID, 0)
		}
		return true
	case "listen_custom":
		if len([]rune(value)) > 200 {
			telegramSend(token, chatID, "监听地址过长，请重新输入。")
			return true
		}
		_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "listen_custom", func(w *telegramWizard) { w.Forward.ListenIp = value; w.Step = "interface" })
		if updated {
			telegramShowForwardInterfaceOptions(token, chatID, 0)
		}
		return true
	case "interface_custom":
		if len([]rune(value)) > 128 {
			telegramSend(token, chatID, "网卡名或 IP 过长，请重新输入。")
			return true
		}
		_, updated := telegramWizardUpdate(chatID, user.ID, "forward", "interface_custom", func(w *telegramWizard) { w.Forward.InterfaceName = value; w.Step = "confirm" })
		if updated {
			telegramShowForwardWizardConfirm(token, chatID, 0, user)
		}
		return true
	}
	telegramSend(token, chatID, "请使用当前消息中的按钮继续，或发送 /cancel 取消。")
	return true
}

func telegramShowTunnelWizardConfirm(token string, chatID, messageID int64, user *model.User) {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok {
		return
	}
	inNode := GetNodeById(wizard.Tunnel.InNodeId)
	outNode := GetNodeById(wizard.Tunnel.InNodeId)
	if wizard.Tunnel.OutNodeId != nil {
		outNode = GetNodeById(*wizard.Tunnel.OutNodeId)
	}
	text := fmt.Sprintf("新增隧道 · 请确认\n\n名称：%s\n类型：%s\n入口：%s\n出口：%s\n协议：%s\n网卡：%s", wizard.Tunnel.Name, telegramTunnelTypeText(wizard.Tunnel.Type), telegramNodeName(inNode), telegramNodeName(outNode), strings.ToUpper(wizard.Tunnel.Protocol), telegramEmptyText(wizard.Tunnel.InterfaceName))
	telegramPresent(token, chatID, messageID, text, telegramKeyboard(
		telegramRow(telegramButton("✅ 确认创建", "wiz:tn:confirm")),
		telegramRow(telegramButton("🔁 重新开始", "tn:new"), telegramButton("✖️ 取消", "wiz:cancel")),
	))
}

func telegramCreateTunnelFromWizard(token string, chatID, messageID int64, user *model.User) {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok || wizard.Step != "confirm" || user.RoleId != adminRoleID {
		return
	}
	res := CreateTunnel(wizard.Tunnel)
	if res.Code != 0 {
		telegramPresent(token, chatID, messageID, "创建隧道失败："+res.Msg, telegramKeyboard(telegramRow(telegramButton("🔁 重试", "wiz:tn:confirm")), telegramRow(telegramButton("✖️ 取消", "wiz:cancel"))))
		return
	}
	telegramWizardClear(chatID)
	var tunnel model.Tunnel
	if !telegramDecode(res.Data, &tunnel) {
		telegramPresent(token, chatID, messageID, "隧道创建成功。", telegramBackKeyboard("tn:l:0"))
		return
	}
	telegramPresent(token, chatID, messageID, fmt.Sprintf("隧道创建成功：#%d %s", tunnel.ID, tunnel.Name), telegramKeyboard(telegramRow(telegramButton("查看隧道", fmt.Sprintf("tn:v:%d:0", tunnel.ID))), telegramRow(telegramButton("返回列表", "tn:l:0"))))
}

func telegramShowForwardWizardConfirm(token string, chatID, messageID int64, user *model.User) {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok {
		return
	}
	path := fmt.Sprintf("隧道 #%d", wizard.Forward.TunnelId)
	if wizard.Forward.RouteId != 0 {
		var route model.Route
		DB.First(&route, wizard.Forward.RouteId)
		path = fmt.Sprintf("路由 #%d %s", route.ID, route.Name)
	} else {
		var tunnel model.Tunnel
		if DB.First(&tunnel, wizard.Forward.TunnelId).Error == nil {
			path = fmt.Sprintf("隧道 #%d %s", tunnel.ID, tunnel.Name)
		}
	}
	port := "自动分配"
	if wizard.Forward.InPort != nil {
		port = strconv.Itoa(*wizard.Forward.InPort)
	}
	text := fmt.Sprintf("新增转发 · 请确认\n\n名称：%s\n路径：%s\n目标：%s\n入口端口：%s\n策略：%s\n监听：%s\n网卡：%s", wizard.Forward.Name, path, telegramTruncate(wizard.Forward.RemoteAddr, 220), port, wizard.Forward.Strategy, telegramEmptyText(wizard.Forward.ListenIp), telegramEmptyText(wizard.Forward.InterfaceName))
	telegramPresent(token, chatID, messageID, text, telegramKeyboard(
		telegramRow(telegramButton("✅ 确认创建", "wiz:fw:confirm")),
		telegramRow(telegramButton("🔁 重新开始", "fw:new"), telegramButton("✖️ 取消", "wiz:cancel")),
	))
}

func telegramCreateForwardFromWizard(token string, chatID, messageID int64, user *model.User) {
	wizard, ok := telegramWizardRead(chatID, user.ID)
	if !ok || wizard.Step != "confirm" {
		return
	}
	res := CreateForward(wizard.Forward, user.ID, user.RoleId, user.User)
	if res.Code != 0 {
		telegramPresent(token, chatID, messageID, "创建转发失败："+res.Msg, telegramKeyboard(telegramRow(telegramButton("🔁 重试", "wiz:fw:confirm")), telegramRow(telegramButton("✖️ 取消", "wiz:cancel"))))
		return
	}
	telegramWizardClear(chatID)
	var forward model.Forward
	query := DB.Where("user_id = ? AND name = ? AND remote_addr = ?", user.ID, wizard.Forward.Name, wizard.Forward.RemoteAddr).Order("id DESC").First(&forward)
	if query.Error != nil {
		telegramPresent(token, chatID, messageID, "转发创建成功。", telegramBackKeyboard("fw:l:0"))
		return
	}
	telegramPresent(token, chatID, messageID, fmt.Sprintf("转发创建成功：#%d %s", forward.ID, forward.Name), telegramKeyboard(telegramRow(telegramButton("查看转发", fmt.Sprintf("fw:v:%d:0", forward.ID))), telegramRow(telegramButton("返回列表", "fw:l:0"))))
}

func telegramNormalizeRemoteAddr(value string) (string, string) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\r' })
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		address := strings.TrimSpace(part)
		if address == "" {
			continue
		}
		port := extractPortFromAddress(address)
		if extractIpFromAddress(address) == "" || port < 1 || port > 65535 {
			return "", fmt.Sprintf("目标地址格式错误：%s，请使用 host:port。", address)
		}
		addresses = append(addresses, address)
	}
	if len(addresses) == 0 {
		return "", "至少需要一个目标地址。"
	}
	return strings.Join(addresses, ","), ""
}

func telegramHandleTunnelCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	switch parts[1] {
	case "l":
		if page, ok := telegramParseInt(parts, 2); ok {
			telegramShowTunnels(token, chatID, messageID, user, int(page))
			return
		}
	case "v":
		id, idOK := telegramParseInt(parts, 2)
		page, pageOK := telegramParseInt(parts, 3)
		if idOK && pageOK {
			telegramShowTunnel(token, chatID, messageID, user, id, int(page))
			return
		}
	case "d":
		id, idOK := telegramParseInt(parts, 2)
		page, pageOK := telegramParseInt(parts, 3)
		if idOK && pageOK {
			telegramDiagnoseTunnel(token, chatID, messageID, user, id, int(page))
			return
		}
	}
	telegramPresent(token, chatID, messageID, "隧道操作参数无效。", telegramBackKeyboard("tn:l:0"))
}

func telegramHandleForwardCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	switch parts[1] {
	case "l":
		if page, ok := telegramParseInt(parts, 2); ok {
			telegramShowForwards(token, chatID, messageID, user, int(page))
			return
		}
	case "v":
		id, idOK := telegramParseInt(parts, 2)
		page, pageOK := telegramParseInt(parts, 3)
		if idOK && pageOK {
			telegramShowForward(token, chatID, messageID, user, id, int(page))
			return
		}
	case "t":
		id, idOK := telegramParseInt(parts, 2)
		enable, enableOK := telegramParseInt(parts, 3)
		page, pageOK := telegramParseInt(parts, 4)
		if idOK && enableOK && pageOK && (enable == 0 || enable == 1) {
			var resultMessage string
			if enable == 1 {
				res := ResumeForward(id, user.ID, user.RoleId)
				resultMessage = telegramFormatGenericResult(res.Msg, res.Data, res.Code == 0)
			} else {
				res := PauseForward(id, user.ID, user.RoleId)
				resultMessage = telegramFormatGenericResult(res.Msg, res.Data, res.Code == 0)
			}
			telegramShowForwardWithNotice(token, chatID, messageID, user, id, int(page), resultMessage)
			return
		}
	case "d":
		id, idOK := telegramParseInt(parts, 2)
		page, pageOK := telegramParseInt(parts, 3)
		if idOK && pageOK {
			telegramDiagnoseForward(token, chatID, messageID, user, id, int(page))
			return
		}
	}
	telegramPresent(token, chatID, messageID, "转发操作参数无效。", telegramBackKeyboard("fw:l:0"))
}

func telegramHandleNodeCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	switch parts[1] {
	case "l":
		if page, ok := telegramParseInt(parts, 2); ok {
			telegramShowNodes(token, chatID, messageID, user, int(page))
			return
		}
	case "v":
		id, idOK := telegramParseInt(parts, 2)
		page, pageOK := telegramParseInt(parts, 3)
		if idOK && pageOK {
			telegramShowNode(token, chatID, messageID, user, id, int(page))
			return
		}
	}
	telegramPresent(token, chatID, messageID, "节点操作参数无效。", telegramBackKeyboard("nd:l:0"))
}

func telegramHandleAuditCallback(token string, chatID, messageID int64, user *model.User, parts []string) {
	filter := telegramAuditFilter{}
	var page int64
	var ok bool
	switch parts[1] {
	case "l":
		page, ok = telegramParseInt(parts, 2)
	case "f":
		filter.ForwardID, ok = telegramParseInt(parts, 2)
		if ok {
			page, ok = telegramParseInt(parts, 3)
		}
	case "n":
		filter.NodeID, ok = telegramParseInt(parts, 2)
		if ok {
			page, ok = telegramParseInt(parts, 3)
		}
	}
	if !ok {
		telegramPresent(token, chatID, messageID, "审计筛选参数无效。", telegramBackKeyboard("au:l:0"))
		return
	}
	telegramShowAudits(token, chatID, messageID, user, filter, int(page))
}

func telegramShowMainMenu(token string, chatID, messageID int64, user *model.User) {
	role := "用户"
	if user.RoleId == adminRoleID {
		role = "管理员"
	}
	text := fmt.Sprintf("Flux Panel 手机控制台\n\n账号：%s（%s）\n请选择要操作的功能：", user.User, role)
	keyboard := telegramKeyboard(
		telegramRow(telegramButton("🛣 隧道管理", "tn:l:0"), telegramButton("🔀 转发管理", "fw:l:0")),
		telegramRow(telegramButton("🖥 节点监控", "nd:l:0"), telegramButton("🔎 流量审计", "au:l:0")),
		telegramRow(telegramButton("📊 账号用量", "usage"), telegramButton("🔑 登录面板", "login")),
	)
	telegramPresent(token, chatID, messageID, text, keyboard)
}

func telegramShowTunnels(token string, chatID, messageID int64, user *model.User, page int) {
	tunnels := telegramTunnelsForUser(user)
	page, start, end, totalPages := telegramPageBounds(len(tunnels), page, telegramPanelPageSize)

	var text strings.Builder
	fmt.Fprintf(&text, "隧道管理（%d 条，第 %d/%d 页）\n", len(tunnels), page+1, totalPages)
	if len(tunnels) == 0 {
		text.WriteString("\n暂无可用隧道。")
	} else {
		text.WriteString("\n点击隧道查看详情和诊断。")
	}

	rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
	if user.RoleId == adminRoleID {
		rows = append(rows, telegramRow(telegramButton("➕ 新增隧道", "tn:new")))
	}
	for _, tunnel := range tunnels[start:end] {
		label := fmt.Sprintf("%s #%d %s", telegramStatusIcon(tunnel.Status == tunnelStatusActive), tunnel.ID, telegramTruncate(tunnel.Name, 28))
		rows = append(rows, telegramRow(telegramButton(label, fmt.Sprintf("tn:v:%d:%d", tunnel.ID, page))))
	}
	rows = append(rows, telegramPaginationRow(page, totalPages, "tn:l:%d")...)
	rows = append(rows, telegramRow(telegramButton("🏠 主菜单", "menu")))
	telegramPresent(token, chatID, messageID, text.String(), telegramKeyboard(rows...))
}

func telegramShowTunnelFromCommand(token string, chatID int64, user *model.User, arg string) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		telegramSend(token, chatID, "用法：/tunnel <隧道ID>")
		return
	}
	telegramShowTunnel(token, chatID, 0, user, id, 0)
}

func telegramShowTunnel(token string, chatID, messageID int64, user *model.User, id int64, page int) {
	tunnel, ok := telegramTunnelByID(user, id)
	if !ok {
		telegramPresent(token, chatID, messageID, "隧道不存在或无权查看。", telegramBackKeyboard(fmt.Sprintf("tn:l:%d", page)))
		return
	}

	inNode := GetNodeById(tunnel.InNodeId)
	outNode := GetNodeById(tunnel.OutNodeId)
	inName := telegramNodeName(inNode)
	outName := "直连目标"
	if tunnel.Type == tunnelTypeTunnelForward {
		outName = telegramNodeName(outNode)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "隧道 #%d · %s\n\n", tunnel.ID, tunnel.Name)
	fmt.Fprintf(&text, "状态：%s\n", telegramEnabledText(tunnel.Status == tunnelStatusActive))
	fmt.Fprintf(&text, "类型：%s\n", telegramTunnelTypeText(tunnel.Type))
	fmt.Fprintf(&text, "链路：%s → %s\n", inName, outName)
	if tunnel.Protocol != "" {
		fmt.Fprintf(&text, "协议：%s\n", strings.ToUpper(tunnel.Protocol))
	}
	if tunnel.InIp != "" || tunnel.OutIp != "" {
		fmt.Fprintf(&text, "地址：%s → %s\n", telegramEmptyText(tunnel.InIp), telegramEmptyText(tunnel.OutIp))
	}
	if tunnel.Flow > 0 {
		fmt.Fprintf(&text, "隧道流量：%d GB\n", tunnel.Flow)
	}
	if tunnel.TrafficRatio > 0 {
		fmt.Fprintf(&text, "流量倍率：%.2f×\n", tunnel.TrafficRatio)
	}

	rows := make([][]tgInlineKeyboardButton, 0, 3)
	if user.RoleId == adminRoleID && tunnel.Type == tunnelTypeTunnelForward {
		rows = append(rows, telegramRow(telegramButton("🩺 诊断链路", fmt.Sprintf("tn:d:%d:%d", tunnel.ID, page))))
	}
	rows = append(rows,
		telegramRow(telegramButton("🔄 刷新", fmt.Sprintf("tn:v:%d:%d", tunnel.ID, page)), telegramButton("⬅️ 返回列表", fmt.Sprintf("tn:l:%d", page))),
		telegramRow(telegramButton("🏠 主菜单", "menu")),
	)
	telegramPresent(token, chatID, messageID, text.String(), telegramKeyboard(rows...))
}

func telegramDiagnoseTunnel(token string, chatID, messageID int64, user *model.User, id int64, page int) {
	if user.RoleId != adminRoleID {
		telegramPresent(token, chatID, messageID, "隧道诊断仅管理员可执行。", telegramBackKeyboard(fmt.Sprintf("tn:v:%d:%d", id, page)))
		return
	}
	if _, ok := telegramTunnelByID(user, id); !ok {
		telegramPresent(token, chatID, messageID, "隧道不存在。", telegramBackKeyboard(fmt.Sprintf("tn:l:%d", page)))
		return
	}
	telegramStartBackgroundDiagnosis(token, chatID, "tunnel", id, "隧道链路诊断", func(progressMessageID int64) {
		res := DiagnoseTunnel(id)
		text := "隧道诊断\n\n" + telegramFormatGenericResult(res.Msg, res.Data, res.Code == 0)
		keyboard := telegramKeyboard(
			telegramRow(telegramButton("🔄 再次诊断", fmt.Sprintf("tn:d:%d:%d", id, page)), telegramButton("⬅️ 返回详情", fmt.Sprintf("tn:v:%d:%d", id, page))),
			telegramRow(telegramButton("🏠 主菜单", "menu")),
		)
		telegramEditWithKeyboard(token, chatID, progressMessageID, text, keyboard)
	})
}

func telegramShowForwards(token string, chatID, messageID int64, user *model.User, page int) {
	forwards := telegramForwardsForUser(user)
	page, start, end, totalPages := telegramPageBounds(len(forwards), page, telegramPanelPageSize)

	var text strings.Builder
	fmt.Fprintf(&text, "转发管理（%d 条，第 %d/%d 页）\n", len(forwards), page+1, totalPages)
	if len(forwards) == 0 {
		text.WriteString("\n暂无转发规则。")
	} else {
		text.WriteString("\n点击规则查看详情、启停、诊断和审计。")
	}

	rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
	rows = append(rows, telegramRow(telegramButton("➕ 新增转发", "fw:new")))
	for _, forward := range forwards[start:end] {
		label := fmt.Sprintf("%s #%d %s", telegramForwardStatusIcon(forward.Status), forward.ID, telegramTruncate(forward.Name, 28))
		rows = append(rows, telegramRow(telegramButton(label, fmt.Sprintf("fw:v:%d:%d", forward.ID, page))))
	}
	rows = append(rows, telegramPaginationRow(page, totalPages, "fw:l:%d")...)
	rows = append(rows, telegramRow(telegramButton("🏠 主菜单", "menu")))
	telegramPresent(token, chatID, messageID, text.String(), telegramKeyboard(rows...))
}

func telegramShowForwardFromCommand(token string, chatID int64, user *model.User, arg string) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		telegramSend(token, chatID, "用法：/forward <转发ID>")
		return
	}
	telegramShowForward(token, chatID, 0, user, id, 0)
}

func telegramShowForward(token string, chatID, messageID int64, user *model.User, id int64, page int) {
	telegramShowForwardWithNotice(token, chatID, messageID, user, id, page, "")
}

func telegramShowForwardWithNotice(token string, chatID, messageID int64, user *model.User, id int64, page int, notice string) {
	forward, ok := telegramForwardByID(user, id)
	if !ok {
		telegramPresent(token, chatID, messageID, "转发不存在或无权查看。", telegramBackKeyboard(fmt.Sprintf("fw:l:%d", page)))
		return
	}

	pathName := forward.TunnelName
	pathType := "隧道"
	if forward.RouteId != 0 {
		pathName = forward.RouteName
		pathType = "路由"
	}
	if pathName == "" {
		pathName = "未知"
	}

	var text strings.Builder
	if notice != "" {
		fmt.Fprintf(&text, "%s\n\n", notice)
	}
	fmt.Fprintf(&text, "转发 #%d · %s\n\n", forward.ID, forward.Name)
	fmt.Fprintf(&text, "状态：%s\n", telegramForwardStatusText(forward.Status))
	if user.RoleId == adminRoleID {
		fmt.Fprintf(&text, "用户：%s (#%d)\n", telegramEmptyText(forward.UserName), forward.UserId)
	}
	fmt.Fprintf(&text, "%s：%s\n", pathType, pathName)
	fmt.Fprintf(&text, "入口：%s:%d\n", telegramEmptyText(forward.InIp), forward.InPort)
	fmt.Fprintf(&text, "目标：%s\n", telegramTruncate(forward.RemoteAddr, 180))
	if forward.Strategy != "" {
		fmt.Fprintf(&text, "策略：%s\n", forward.Strategy)
	}
	fmt.Fprintf(&text, "流量：↑ %s  ↓ %s  合计 %s\n", telegramFormatBytes(forward.OutFlow), telegramFormatBytes(forward.InFlow), telegramFormatBytes(forward.InFlow+forward.OutFlow))

	toggleText := "⏸ 暂停"
	toggleValue := 0
	if forward.Status != forwardStatusActive {
		toggleText = "▶️ 恢复"
		toggleValue = 1
	}
	keyboard := telegramKeyboard(
		telegramRow(telegramButton(toggleText, fmt.Sprintf("fw:t:%d:%d:%d", forward.ID, toggleValue, page)), telegramButton("🩺 诊断", fmt.Sprintf("fw:d:%d:%d", forward.ID, page))),
		telegramRow(telegramButton("🔎 此规则审计", fmt.Sprintf("au:f:%d:0", forward.ID)), telegramButton("🔄 刷新", fmt.Sprintf("fw:v:%d:%d", forward.ID, page))),
		telegramRow(telegramButton("⬅️ 返回列表", fmt.Sprintf("fw:l:%d", page)), telegramButton("🏠 主菜单", "menu")),
	)
	telegramPresent(token, chatID, messageID, text.String(), keyboard)
}

func telegramDiagnoseForward(token string, chatID, messageID int64, user *model.User, id int64, page int) {
	if _, ok := telegramForwardByID(user, id); !ok {
		telegramPresent(token, chatID, messageID, "转发不存在或无权诊断。", telegramBackKeyboard(fmt.Sprintf("fw:l:%d", page)))
		return
	}
	telegramStartBackgroundDiagnosis(token, chatID, "forward", id, "转发链路诊断", func(progressMessageID int64) {
		res := DiagnoseForward(id, user.ID, user.RoleId)
		text := telegramForwardDiagnosisText(res.Msg, res.Data, res.Code == 0)
		keyboard := telegramKeyboard(
			telegramRow(telegramButton("🔄 再次诊断", fmt.Sprintf("fw:d:%d:%d", id, page)), telegramButton("⬅️ 返回详情", fmt.Sprintf("fw:v:%d:%d", id, page))),
			telegramRow(telegramButton("🏠 主菜单", "menu")),
		)
		telegramEditWithKeyboard(token, chatID, progressMessageID, text, keyboard)
	})
}

func telegramStartBackgroundDiagnosis(token string, chatID int64, kind string, id int64, title string, run func(progressMessageID int64)) {
	key := telegramDiagnosisJobKey{ChatID: chatID, Kind: kind, ID: id}
	if _, loaded := telegramDiagnosisJobs.LoadOrStore(key, struct{}{}); loaded {
		telegramSend(token, chatID, title+"正在执行中，请等待当前结果。")
		return
	}

	progressMessageID, ok := telegramSendMessage(token, chatID, title+"已开始。\n\n诊断将在后台执行，你可以继续操作其他菜单。", telegramKeyboard(
		telegramRow(telegramButton("🏠 主菜单", "menu")),
	))
	if !ok {
		telegramDiagnosisJobs.Delete(key)
		telegramSend(token, chatID, title+"启动失败，请稍后重试。")
		return
	}

	go func() {
		defer telegramDiagnosisJobs.Delete(key)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[Telegram] %s 后台任务异常: %v", title, recovered)
				telegramEditWithKeyboard(token, chatID, progressMessageID, title+"执行异常，请稍后重试。", telegramKeyboard(
					telegramRow(telegramButton("🏠 主菜单", "menu")),
				))
			}
		}()
		run(progressMessageID)
	}()
}

func telegramShowNodes(token string, chatID, messageID int64, user *model.User, page int) {
	nodes := telegramNodesForUser(user)
	page, start, end, totalPages := telegramPageBounds(len(nodes), page, telegramPanelPageSize)
	onlineCount := 0
	for _, node := range nodes {
		if node.Online {
			onlineCount++
		}
	}

	var text strings.Builder
	fmt.Fprintf(&text, "节点监控（在线 %d/%d，第 %d/%d 页）\n", onlineCount, len(nodes), page+1, totalPages)
	if len(nodes) == 0 {
		text.WriteString("\n暂无可访问节点。")
	} else {
		text.WriteString("\n点击节点查看实时指标。")
	}

	rows := make([][]tgInlineKeyboardButton, 0, end-start+2)
	for _, node := range nodes[start:end] {
		label := fmt.Sprintf("%s #%d %s", telegramStatusIcon(node.Online), node.Node.ID, telegramTruncate(node.Node.Name, 28))
		rows = append(rows, telegramRow(telegramButton(label, fmt.Sprintf("nd:v:%d:%d", node.Node.ID, page))))
	}
	rows = append(rows, telegramPaginationRow(page, totalPages, "nd:l:%d")...)
	rows = append(rows, telegramRow(telegramButton("🔄 刷新", fmt.Sprintf("nd:l:%d", page)), telegramButton("🏠 主菜单", "menu")))
	telegramPresent(token, chatID, messageID, text.String(), telegramKeyboard(rows...))
}

func telegramShowNodeFromCommand(token string, chatID int64, user *model.User, arg string) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		telegramSend(token, chatID, "用法：/node <节点ID>")
		return
	}
	telegramShowNode(token, chatID, 0, user, id, 0)
}

func telegramShowNode(token string, chatID, messageID int64, user *model.User, id int64, page int) {
	node, ok := telegramNodeByID(user, id)
	if !ok {
		telegramPresent(token, chatID, messageID, "节点不存在或无权查看。", telegramBackKeyboard(fmt.Sprintf("nd:l:%d", page)))
		return
	}

	var text strings.Builder
	fmt.Fprintf(&text, "节点 #%d · %s\n\n", node.Node.ID, node.Node.Name)
	fmt.Fprintf(&text, "状态：%s\n", telegramOnlineText(node.Online))
	if node.Node.GroupName != "" {
		fmt.Fprintf(&text, "分组：%s\n", node.Node.GroupName)
	}
	if user.RoleId == adminRoleID {
		fmt.Fprintf(&text, "服务器：%s\n", telegramEmptyText(node.Node.ServerIp))
		fmt.Fprintf(&text, "节点版本：%s\n", telegramEmptyText(node.Node.Version))
	}
	if node.Online && node.Info != nil && user.RoleId == adminRoleID {
		fmt.Fprintf(&text, "CPU：%.1f%%\n", node.Info.CPUUsage)
		fmt.Fprintf(&text, "内存：%.1f%%\n", node.Info.MemoryUsage)
		fmt.Fprintf(&text, "运行时间：%s\n", telegramHumanUptime(node.Info.Uptime))
		fmt.Fprintf(&text, "网卡流量：收 %s / 发 %s\n", telegramFormatBytes(telegramUint64ToInt64(node.Info.BytesReceived)), telegramFormatBytes(telegramUint64ToInt64(node.Info.BytesTransmitted)))
		if node.Info.Runtime != "" {
			fmt.Fprintf(&text, "运行环境：%s\n", node.Info.Runtime)
		}
		fmt.Fprintf(&text, "Xray：%s", telegramRunningText(node.Info.XrayRunning))
		if node.Info.XrayVersion != "" {
			fmt.Fprintf(&text, " (%s)", node.Info.XrayVersion)
		}
		text.WriteString("\n")
		fmt.Fprintf(&text, "Sing-box 审计：%s\n", telegramAuditHealthText(node.Info.SingBoxAudit))
		if node.Info.SingBoxAudit.LastEventTime > 0 {
			fmt.Fprintf(&text, "最近审计：%s\n", telegramFormatUnixTime(node.Info.SingBoxAudit.LastEventTime))
		}
		if node.Info.SingBoxAudit.LastError != "" {
			fmt.Fprintf(&text, "审计错误：%s\n", telegramTruncate(node.Info.SingBoxAudit.LastError, 160))
		}
	}

	keyboard := telegramKeyboard(
		telegramRow(telegramButton("🔎 此节点审计", fmt.Sprintf("au:n:%d:0", node.Node.ID)), telegramButton("🔄 刷新", fmt.Sprintf("nd:v:%d:%d", node.Node.ID, page))),
		telegramRow(telegramButton("⬅️ 返回列表", fmt.Sprintf("nd:l:%d", page)), telegramButton("🏠 主菜单", "menu")),
	)
	telegramPresent(token, chatID, messageID, text.String(), keyboard)
}

func telegramShowAudits(token string, chatID, messageID int64, user *model.User, filter telegramAuditFilter, page int) {
	records, total, page := telegramAuditsForUser(user, filter, page)
	_, _, _, totalPages := telegramPageBounds(int(total), page, telegramPanelPageSize)

	var text strings.Builder
	fmt.Fprintf(&text, "%s（%d 条，第 %d/%d 页）\n", telegramAuditTitle(filter), total, page+1, totalPages)
	if len(records) == 0 {
		text.WriteString("\n暂无符合条件的审计记录。")
	}
	for _, record := range records {
		text.WriteString("\n\n")
		recordTime := record.EndedTime
		if recordTime <= 0 {
			recordTime = record.StartedTime
		}
		fmt.Fprintf(&text, "%s · %s · %s\n", telegramFormatUnixTime(recordTime), telegramAuditSource(record), telegramEmptyText(strings.ToUpper(record.Protocol)))
		fmt.Fprintf(&text, "%s → %s\n", telegramTruncate(telegramEmptyText(record.ClientAddr), 48), telegramTruncate(telegramEmptyText(record.TargetAddr), 72))
		fmt.Fprintf(&text, "↑ %s  ↓ %s", telegramFormatBytes(record.UpBytes), telegramFormatBytes(record.DownBytes))
		if record.DurationMs > 0 {
			fmt.Fprintf(&text, "  %s", telegramDurationText(record.DurationMs))
		}
		if record.Error != "" {
			fmt.Fprintf(&text, "\n错误：%s", telegramTruncate(record.Error, 100))
		}
	}

	rows := make([][]tgInlineKeyboardButton, 0, 3)
	callbackFormat := telegramAuditPageCallback(filter)
	rows = append(rows, telegramPaginationRow(page, totalPages, callbackFormat)...)
	if filter.ForwardID != 0 || filter.NodeID != 0 {
		rows = append(rows, telegramRow(telegramButton("🧹 清除筛选", "au:l:0")))
	}
	rows = append(rows, telegramRow(telegramButton("🔄 刷新", fmt.Sprintf(callbackFormat, page)), telegramButton("🏠 主菜单", "menu")))
	telegramPresent(token, chatID, messageID, text.String(), telegramKeyboard(rows...))
}

func telegramTunnelsForUser(user *model.User) []telegramTunnelView {
	var tunnels []telegramTunnelView
	if user.RoleId == adminRoleID {
		DB.Model(&model.Tunnel{}).Order("inx ASC, created_time DESC").Scan(&tunnels)
		return tunnels
	}
	res := GetUserAccessibleTunnels(user.ID, user.RoleId)
	if res.Code != 0 || !telegramDecode(res.Data, &tunnels) {
		return nil
	}
	sort.SliceStable(tunnels, func(i, j int) bool {
		if tunnels[i].Inx == tunnels[j].Inx {
			return tunnels[i].CreatedTime > tunnels[j].CreatedTime
		}
		return tunnels[i].Inx < tunnels[j].Inx
	})
	return tunnels
}

func telegramTunnelByID(user *model.User, id int64) (telegramTunnelView, bool) {
	for _, tunnel := range telegramTunnelsForUser(user) {
		if tunnel.ID == id {
			return tunnel, true
		}
	}
	return telegramTunnelView{}, false
}

func telegramForwardsForUser(user *model.User) []ForwardWithTunnel {
	res := GetAllForwards(user.ID, user.RoleId)
	if res.Code != 0 {
		return nil
	}
	if forwards, ok := res.Data.([]ForwardWithTunnel); ok {
		return forwards
	}
	var forwards []ForwardWithTunnel
	if !telegramDecode(res.Data, &forwards) {
		return nil
	}
	return forwards
}

func telegramForwardByID(user *model.User, id int64) (ForwardWithTunnel, bool) {
	for _, forward := range telegramForwardsForUser(user) {
		if forward.ID == id {
			return forward, true
		}
	}
	return ForwardWithTunnel{}, false
}

func telegramNodesForUser(user *model.User) []telegramNodeView {
	var nodes []model.Node
	if user.RoleId == adminRoleID {
		DB.Order("inx ASC, created_time DESC").Find(&nodes)
	} else {
		res := GetUserAccessibleNodes(user.ID, user.RoleId, false, false)
		var accessible []struct {
			ID int64 `json:"id"`
		}
		if res.Code != 0 || !telegramDecode(res.Data, &accessible) || len(accessible) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(accessible))
		for _, item := range accessible {
			ids = append(ids, item.ID)
		}
		DB.Where("id IN ?", ids).Order("inx ASC, created_time DESC").Find(&nodes)
	}

	views := make([]telegramNodeView, 0, len(nodes))
	for _, node := range nodes {
		view := telegramNodeView{Node: node}
		if pkg.WS != nil {
			view.Online = pkg.WS.IsNodeOnline(node.ID)
			if user.RoleId == adminRoleID && view.Online {
				view.Info = pkg.WS.GetNodeSystemInfo(node.ID)
			}
		}
		views = append(views, view)
	}
	return views
}

func telegramNodeByID(user *model.User, id int64) (telegramNodeView, bool) {
	for _, node := range telegramNodesForUser(user) {
		if node.Node.ID == id {
			return node, true
		}
	}
	return telegramNodeView{}, false
}

func telegramAuditsForUser(user *model.User, filter telegramAuditFilter, page int) ([]model.ConnectionAudit, int64, int) {
	visibleWhere, visibleArgs := connectionAuditVisibleScope()
	query := DB.Model(&model.ConnectionAudit{}).Where(visibleWhere, visibleArgs...)
	if user.RoleId != adminRoleID {
		query = query.Where("user_id = ?", user.ID)
	}
	if filter.ForwardID != 0 {
		query = query.Where("forward_id = ?", filter.ForwardID)
	}
	if filter.NodeID != 0 {
		query = query.Where("node_id = ?", filter.NodeID)
	}

	var total int64
	query.Count(&total)
	page, _, _, _ = telegramPageBounds(int(total), page, telegramPanelPageSize)
	var records []model.ConnectionAudit
	query.Order("ended_time DESC, id DESC").Limit(telegramPanelPageSize).Offset(page * telegramPanelPageSize).Find(&records)
	return records, total, page
}

func telegramForwardDiagnosisText(message string, data interface{}, success bool) string {
	if !success {
		return "转发诊断失败\n\n" + telegramEmptyText(message)
	}
	var report telegramForwardDiagnosis
	if !telegramDecode(data, &report) || len(report.Results) == 0 {
		return "转发诊断\n\n" + telegramFormatGenericResult(message, data, true)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "转发诊断 · %s\n", telegramEmptyText(report.ForwardName))
	if report.TunnelType != "" {
		fmt.Fprintf(&text, "类型：%s\n", report.TunnelType)
	}
	for _, result := range report.Results {
		fmt.Fprintf(&text, "\n%s %s\n", telegramStatusIcon(result.Success), telegramEmptyText(result.Description))
		fmt.Fprintf(&text, "%s → %s:%d\n", telegramEmptyText(result.NodeName), telegramEmptyText(result.TargetIp), result.TargetPort)
		if result.Success {
			fmt.Fprintf(&text, "延迟 %.2f ms，丢包 %.1f%%", result.AverageTime, result.PacketLoss)
		} else {
			text.WriteString(telegramEmptyText(result.Message))
		}
	}
	return text.String()
}

func telegramFormatGenericResult(message string, data interface{}, success bool) string {
	if !success {
		return "失败：" + telegramEmptyText(message)
	}
	if data == nil {
		return telegramEmptyText(message)
	}
	if value, ok := data.(string); ok {
		return value
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return telegramEmptyText(message)
	}
	return telegramLimitMessage(string(encoded), 3200)
}

func telegramAuditTitle(filter telegramAuditFilter) string {
	if filter.ForwardID != 0 {
		return fmt.Sprintf("流量审计 · 转发 #%d", filter.ForwardID)
	}
	if filter.NodeID != 0 {
		return fmt.Sprintf("流量审计 · 节点 #%d", filter.NodeID)
	}
	return "流量审计 · 最近记录"
}

func telegramAuditPageCallback(filter telegramAuditFilter) string {
	if filter.ForwardID != 0 {
		return fmt.Sprintf("au:f:%d:%%d", filter.ForwardID)
	}
	if filter.NodeID != 0 {
		return fmt.Sprintf("au:n:%d:%%d", filter.NodeID)
	}
	return "au:l:%d"
}

func telegramAuditSource(record model.ConnectionAudit) string {
	parts := make([]string, 0, 3)
	if record.NodeName != "" {
		parts = append(parts, record.NodeName)
	}
	if record.ForwardName != "" {
		parts = append(parts, record.ForwardName)
	} else if record.ForwardId != 0 {
		parts = append(parts, fmt.Sprintf("转发 #%d", record.ForwardId))
	}
	if record.UserName != "" {
		parts = append(parts, record.UserName)
	}
	if len(parts) == 0 {
		return telegramEmptyText(record.ServiceName)
	}
	return strings.Join(parts, " / ")
}

func telegramFormatUnixTime(value int64) string {
	if value <= 0 {
		return "未知时间"
	}
	if value > 10_000_000_000 {
		return time.UnixMilli(value).Format("01-02 15:04:05")
	}
	return time.Unix(value, 0).Format("01-02 15:04:05")
}

func telegramDurationText(durationMs int64) string {
	if durationMs < 1000 {
		return fmt.Sprintf("%d ms", durationMs)
	}
	if durationMs < 60_000 {
		return fmt.Sprintf("%.1f s", float64(durationMs)/1000)
	}
	return fmt.Sprintf("%.1f min", float64(durationMs)/60_000)
}

func telegramHumanUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%d天 %d小时", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

func telegramAuditHealthText(info pkg.SingBoxAuditInfo) string {
	if info.TailerRunning && info.LogReadable {
		return "正常"
	}
	if info.LastError != "" {
		return "异常"
	}
	if info.LogReadable {
		return "日志可读，采集器未运行"
	}
	return "未启用或日志不可读"
}

func telegramPageBounds(total, page, pageSize int) (int, int, int, int) {
	if pageSize <= 0 {
		pageSize = telegramPanelPageSize
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return page, start, end, totalPages
}

func telegramPaginationRow(page, totalPages int, callbackFormat string) [][]tgInlineKeyboardButton {
	row := make([]tgInlineKeyboardButton, 0, 2)
	if page > 0 {
		row = append(row, telegramButton("⬅️ 上一页", fmt.Sprintf(callbackFormat, page-1)))
	}
	if page+1 < totalPages {
		row = append(row, telegramButton("下一页 ➡️", fmt.Sprintf(callbackFormat, page+1)))
	}
	if len(row) == 0 {
		return nil
	}
	return [][]tgInlineKeyboardButton{row}
}

func telegramParseInt(parts []string, index int) (int64, bool) {
	if index < 0 || index >= len(parts) {
		return 0, false
	}
	value, err := strconv.ParseInt(parts[index], 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func telegramDecode(value interface{}, target interface{}) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return json.Unmarshal(encoded, target) == nil
}

func telegramPresent(token string, chatID, messageID int64, text string, keyboard *tgInlineKeyboardMarkup) {
	if messageID > 0 {
		telegramEditWithKeyboard(token, chatID, messageID, telegramLimitMessage(text, 4000), keyboard)
		return
	}
	telegramSendWithKeyboard(token, chatID, telegramLimitMessage(text, 4000), keyboard)
}

func telegramKeyboard(rows ...[]tgInlineKeyboardButton) *tgInlineKeyboardMarkup {
	nonEmpty := make([][]tgInlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		if len(row) > 0 {
			nonEmpty = append(nonEmpty, row)
		}
	}
	return &tgInlineKeyboardMarkup{InlineKeyboard: nonEmpty}
}

func telegramBackKeyboard(callback string) *tgInlineKeyboardMarkup {
	return telegramKeyboard(telegramRow(telegramButton("⬅️ 返回", callback)))
}

func telegramButton(text, callback string) tgInlineKeyboardButton {
	return tgInlineKeyboardButton{Text: text, CallbackData: callback}
}

func telegramRow(buttons ...tgInlineKeyboardButton) []tgInlineKeyboardButton {
	return buttons
}

func telegramNodeName(node *model.Node) string {
	if node == nil {
		return "未知节点"
	}
	return node.Name
}

func telegramTunnelTypeText(tunnelType int) string {
	if tunnelType == tunnelTypeTunnelForward {
		return "双节点隧道"
	}
	return "单节点端口转发"
}

func telegramForwardStatusIcon(status int) string {
	switch status {
	case forwardStatusActive:
		return "🟢"
	case forwardStatusPaused:
		return "⏸"
	default:
		return "🔴"
	}
}

func telegramForwardStatusText(status int) string {
	switch status {
	case forwardStatusActive:
		return "运行中"
	case forwardStatusPaused:
		return "已暂停"
	default:
		return "异常"
	}
}

func telegramStatusIcon(ok bool) string {
	if ok {
		return "🟢"
	}
	return "🔴"
}

func telegramEnabledText(enabled bool) string {
	if enabled {
		return "启用"
	}
	return "停用"
}

func telegramOnlineText(online bool) string {
	if online {
		return "在线"
	}
	return "离线"
}

func telegramRunningText(running bool) string {
	if running {
		return "运行中"
	}
	return "未运行"
}

func telegramEmptyText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	return value
}

func telegramTruncate(value string, maxRunes int) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return telegramLimitMessage(value, maxRunes)
}

func telegramLimitMessage(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

func telegramUint64ToInt64(value uint64) int64 {
	const maxInt64 = uint64(^uint64(0) >> 1)
	if value > maxInt64 {
		return int64(maxInt64)
	}
	return int64(value)
}
