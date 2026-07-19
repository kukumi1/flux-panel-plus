package service

import (
	"encoding/json"
	"flux-panel/go-backend/config"
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"net"
	"strconv"
	"strings"
	"time"
)

const maxAuditBatchSize = 200
const singBoxAuditServicePrefix = "sing-box"
const xrayAuditServicePrefix = "xray"
const gostAuditServicePrefix = "gost"
const dnsTargetPort = 53

const (
	auditSourceSingBox = "singbox"
	auditSourceXray    = "xray"
	auditSourceGost    = "gost"
)

const auditIngestNodeName = "x-ui:DMIT"

var singBoxNoiseTargets = map[string]struct{}{
	"www.gstatic.com":               {},
	"cp.cloudflare.com":             {},
	"connectivitycheck.gstatic.com": {},
	"connectivitycheck.android.com": {},
	"clients3.google.com":           {},
	"captive.apple.com":             {},
	"www.msftconnecttest.com":       {},
	"www.msftncsi.com":              {},
	"detectportal.firefox.com":      {},
	"config.immersivetranslate.com": {},
}

var singBoxNoiseTargetList = []string{
	"www.gstatic.com",
	"cp.cloudflare.com",
	"connectivitycheck.gstatic.com",
	"connectivitycheck.android.com",
	"clients3.google.com",
	"captive.apple.com",
	"www.msftconnecttest.com",
	"www.msftncsi.com",
	"detectportal.firefox.com",
	"config.immersivetranslate.com",
}

func ProcessConnectionAuditUpload(rawData, secret string) string {
	node, ok := resolveAuditNode(secret)
	if !ok {
		return "ok"
	}

	decrypted := decryptIfNeeded(rawData, secret)
	var upload dto.ConnectionAuditUploadDto
	if err := json.Unmarshal([]byte(decrypted), &upload); err != nil {
		return "ok"
	}
	if len(upload.Events) == 0 {
		return "ok"
	}
	if len(upload.Events) > maxAuditBatchSize {
		upload.Events = upload.Events[:maxAuditBatchSize]
	}

	now := time.Now().UnixMilli()
	records := make([]model.ConnectionAudit, 0, len(upload.Events))
	for _, event := range upload.Events {
		record := buildConnectionAuditRecord(node, event, now)
		if record.ServiceName == "" || record.ClientIp == "" {
			continue
		}
		if !shouldStoreConnectionAudit(record) {
			continue
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return "ok"
	}
	DB.CreateInBatches(records, 100)
	return "ok"
}

// resolveAuditNode authenticates an audit upload. A real node matches by its
// secret; the configured AUDIT_INGEST_SECRET authorises external xray/x-ui
// sources (e.g. the DMIT log shipper) without registering a node row.
func resolveAuditNode(secret string) (model.Node, bool) {
	var node model.Node
	if err := DB.Where("secret = ?", secret).First(&node).Error; err == nil {
		return node, true
	}
	if secret != "" && config.Cfg != nil && secret == config.Cfg.AuditIngestSecret {
		return model.Node{Name: auditIngestNodeName}, true
	}
	return model.Node{}, false
}

func GetConnectionAudits(d dto.ConnectionAuditListDto) dto.R {
	page := d.Page
	if page <= 0 {
		page = 1
	}
	pageSize := d.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	visibleWhere, visibleArgs := connectionAuditVisibleScope()
	query := DB.Model(&model.ConnectionAudit{}).Where(visibleWhere, visibleArgs...)
	if d.ClientIp != "" {
		query = query.Where("client_ip LIKE ?", "%"+d.ClientIp+"%")
	}
	if d.ClientEmail != "" {
		query = query.Where("client_email LIKE ?", "%"+d.ClientEmail+"%")
	}
	if d.Target != "" {
		like := "%" + d.Target + "%"
		query = query.Where("target_host LIKE ? OR target_addr LIKE ?", like, like)
	}
	if d.Service != "" {
		query = query.Where("service_name LIKE ?", "%"+d.Service+"%")
	}
	if d.ForwardId != 0 {
		query = query.Where("forward_id = ?", d.ForwardId)
	}
	if d.NodeId != 0 {
		query = query.Where("node_id = ?", d.NodeId)
	}
	if d.NodeName != "" {
		query = query.Where("node_name = ?", d.NodeName)
	}
	if d.Protocol != "" && d.Protocol != "all" {
		query = query.Where("protocol = ?", d.Protocol)
	}
	if d.StartTime > 0 {
		query = query.Where("ended_time >= ?", d.StartTime)
	}
	if d.EndTime > 0 {
		query = query.Where("started_time <= ?", d.EndTime)
	}

	var total int64
	query.Count(&total)
	var list []model.ConnectionAudit
	query.Order("ended_time DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&list)

	return dto.Ok(map[string]interface{}{
		"list":     list,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func buildConnectionAuditRecord(node model.Node, event dto.ConnectionAuditEventDto, now int64) model.ConnectionAudit {
	clientIp, clientPort := splitHostPort(event.ClientAddr)
	targetHost, targetPort := splitHostPort(event.TargetAddr)
	serviceName := normalizeAuditServiceName(event.ServiceName)

	record := model.ConnectionAudit{
		NodeId:      node.ID,
		NodeName:    node.Name,
		ServiceName: serviceName,
		ClientAddr:  event.ClientAddr,
		ClientIp:    clientIp,
		ClientPort:  clientPort,
		ClientEmail: strings.TrimSpace(event.ClientEmail),
		TargetHost:  targetHost,
		TargetPort:  targetPort,
		TargetAddr:  event.TargetAddr,
		Protocol:    normalizeAuditProtocol(event.Protocol),
		UpBytes:     event.UpBytes,
		DownBytes:   event.DownBytes,
		DurationMs:  event.DurationMs,
		StartedTime: event.StartedTime,
		EndedTime:   event.EndedTime,
		Error:       event.Error,
		CreatedTime: now,
	}
	if record.EndedTime == 0 {
		record.EndedTime = now
	}
	if record.StartedTime == 0 {
		record.StartedTime = record.EndedTime - record.DurationMs
	}
	if record.StartedTime < 0 {
		record.StartedTime = record.EndedTime
	}

	switch {
	case isXrayAuditEvent(record):
		record.SourceType = auditSourceXray
	case isGostAuditEvent(record):
		record.SourceType = auditSourceGost
		forwardId, userId, _ := parseAuditServiceName(strings.TrimPrefix(serviceName, gostAuditServicePrefix+":"))
		record.ForwardId = forwardId
		record.UserId = userId
		enrichAuditForward(&record)
	default:
		record.SourceType = auditSourceSingBox
		forwardId, userId, _ := parseAuditServiceName(serviceName)
		record.ForwardId = forwardId
		record.UserId = userId
		enrichAuditForward(&record)
	}
	return record
}

// isXrayAuditEvent distinguishes xray/x-ui access events (carrying a client
// email or an "xray" service prefix) from sing-box forward events.
func isXrayAuditEvent(record model.ConnectionAudit) bool {
	return record.ClientEmail != "" || strings.HasPrefix(record.ServiceName, xrayAuditServicePrefix)
}

// isGostAuditEvent identifies native GOST handler audit events emitted by flux
// nodes; the agent prefixes them with "gost:" to opt into storage.
func isGostAuditEvent(record model.ConnectionAudit) bool {
	return strings.HasPrefix(record.ServiceName, gostAuditServicePrefix+":")
}

func enrichAuditForward(record *model.ConnectionAudit) {
	if record.ForwardId == 0 {
		return
	}
	var forward model.Forward
	if err := DB.First(&forward, record.ForwardId).Error; err != nil {
		return
	}
	record.ForwardName = forward.Name
	record.TunnelId = forward.TunnelId
	record.RouteId = forward.RouteId
	if record.UserId == 0 {
		record.UserId = forward.UserId
	}
	record.UserName = forward.UserName
	if record.TargetAddr == "" {
		record.TargetAddr = forward.RemoteAddr
		host, port := splitHostPort(firstAuditTarget(forward.RemoteAddr))
		record.TargetHost = host
		record.TargetPort = port
	}
	if record.TunnelId != 0 {
		var tunnel model.Tunnel
		if err := DB.First(&tunnel, record.TunnelId).Error; err == nil {
			record.TunnelName = tunnel.Name
		}
	}
	if record.RouteId != 0 {
		var route model.Route
		if err := DB.First(&route, record.RouteId).Error; err == nil {
			record.RouteName = route.Name
		}
	}
}

func shouldStoreConnectionAudit(record model.ConnectionAudit) bool {
	if !strings.HasPrefix(record.ServiceName, singBoxAuditServicePrefix) &&
		!strings.HasPrefix(record.ServiceName, xrayAuditServicePrefix) &&
		!strings.HasPrefix(record.ServiceName, gostAuditServicePrefix) {
		return false
	}
	return !isAuditNoise(record)
}

func isAuditNoise(record model.ConnectionAudit) bool {
	if !hasAuditTarget(record) {
		return true
	}
	if record.TargetPort == dnsTargetPort {
		return true
	}
	_, ok := singBoxNoiseTargets[strings.ToLower(strings.TrimSpace(record.TargetHost))]
	return ok
}

func hasAuditTarget(record model.ConnectionAudit) bool {
	return strings.TrimSpace(record.TargetHost) != "" || strings.TrimSpace(record.TargetAddr) != ""
}

func connectionAuditVisibleScope() (string, []interface{}) {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(singBoxNoiseTargetList)), ",")
	args := []interface{}{singBoxAuditServicePrefix + "%", xrayAuditServicePrefix + "%", gostAuditServicePrefix + "%", dnsTargetPort}
	for _, target := range singBoxNoiseTargetList {
		args = append(args, target)
	}
	where := "(service_name LIKE ? OR service_name LIKE ? OR service_name LIKE ?) " +
		"AND (COALESCE(target_host, '') <> '' OR COALESCE(target_addr, '') <> '') " +
		"AND target_port <> ? " +
		"AND LOWER(COALESCE(target_host, '')) NOT IN (" + placeholders + ")"
	return where, args
}
func normalizeAuditServiceName(name string) string {
	name = strings.TrimSpace(name)
	for _, suffix := range []string{"_tcp", "_udp"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	if strings.Contains(name, "_route_") {
		parts := strings.Split(name, "_route_")
		return parts[0]
	}
	return strings.TrimSuffix(name, "_tls")
}

func parseAuditServiceName(name string) (int64, int64, int64) {
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return 0, 0, 0
	}
	forwardId, _ := strconv.ParseInt(parts[0], 10, 64)
	userId, _ := strconv.ParseInt(parts[1], 10, 64)
	userTunnelId, _ := strconv.ParseInt(parts[2], 10, 64)
	return forwardId, userId, userTunnelId
}

func splitHostPort(addr string) (string, int) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Count(addr, ":") == 1 {
			parts := strings.Split(addr, ":")
			port, _ := strconv.Atoi(parts[1])
			return parts[0], port
		}
		return strings.Trim(addr, "[]"), 0
	}
	port, _ := strconv.Atoi(portText)
	return strings.Trim(host, "[]"), port
}

func firstAuditTarget(addr string) string {
	parts := strings.Split(addr, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func normalizeAuditProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return "tcp"
	}
	return protocol
}
