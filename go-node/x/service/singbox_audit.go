package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultSingBoxAccessLogPath = "/var/log/sing-box/access.log"

type SingBoxAuditStatus struct {
	LogPath       string `json:"logPath"`
	LogReadable   bool   `json:"logReadable"`
	TailerRunning bool   `json:"tailerRunning"`
	LastEventTime int64  `json:"lastEventTime"`
	LastErrorTime int64  `json:"lastErrorTime"`
	LastError     string `json:"lastError"`
}

var singBoxAuditStatus = struct {
	sync.RWMutex
	value SingBoxAuditStatus
}{value: SingBoxAuditStatus{LogPath: defaultSingBoxAccessLogPath}}

func GetSingBoxAuditStatus() SingBoxAuditStatus {
	singBoxAuditStatus.RLock()
	defer singBoxAuditStatus.RUnlock()
	return singBoxAuditStatus.value
}

func updateSingBoxAuditStatus(update func(*SingBoxAuditStatus)) {
	singBoxAuditStatus.Lock()
	defer singBoxAuditStatus.Unlock()
	update(&singBoxAuditStatus.value)
}

var (
	singBoxLogIDPattern        = regexp.MustCompile(`\b(?:TRACE|DEBUG|INFO|WARN|ERROR)\s+\[([^\]\s]+)(?:\s[^\]]*)?\]`)
	singBoxComponentTagPattern = regexp.MustCompile(`\b(?:inbound|outbound)/[^\[]+\[([^\]]+)\]`)
	singBoxFromPattern         = regexp.MustCompile(`(?i)\bfrom\s+([^\s,;]+)`)
	singBoxToPattern           = regexp.MustCompile(`(?i)\bto\s+([^\s,;]+)`)
	singBoxFlowPattern         = regexp.MustCompile(`(?i)\b(tcp|udp)\s+([^\s,;]+)\s+(?:-->|->|=>)\s+([^\s,;]+)`)
	singBoxArrowPattern        = regexp.MustCompile(`([^\s,;]+)\s+(?:-->|->|=>)\s+([^\s,;]+)`)
)

type singBoxAuditPartial struct {
	serviceName string
	clientAddr  string
	targetAddr  string
	protocol    string
	errorText   string
	started     time.Time
	updated     time.Time
}

func tailSingBoxAccessLog(ctx context.Context, path string) {
	updateSingBoxAuditStatus(func(status *SingBoxAuditStatus) {
		status.LogPath = path
		status.TailerRunning = true
	})
	pending := make(map[string]*singBoxAuditPartial)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		file, err := os.Open(path)
		if err != nil {
			updateSingBoxAuditStatus(func(status *SingBoxAuditStatus) {
				status.LogReadable = false
				status.LastError = err.Error()
			})
			if !sleepContext(ctx, 30*time.Second) {
				return
			}
			continue
		}

		updateSingBoxAuditStatus(func(status *SingBoxAuditStatus) {
			status.LogReadable = true
			status.LastError = ""
		})
		readSingBoxAccessLog(ctx, file, path, pending)
		file.Close()

		if !sleepContext(ctx, time.Second) {
			return
		}
	}
}

func readSingBoxAccessLog(ctx context.Context, file *os.File, path string, pending map[string]*singBoxAuditPartial) {
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return
	}
	reader := bufio.NewReader(file)
	lastCleanup := time.Now()
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if event, ok := parseSingBoxAccessLogLine(line, time.Now(), pending); ok {
				enqueueConnectionAudit(event)
				updateSingBoxAuditStatus(func(status *SingBoxAuditStatus) {
					status.LastEventTime = time.Now().UnixMilli()
					if event.Error != "" {
						status.LastErrorTime = status.LastEventTime
						status.LastError = event.Error
					}
				})
			}
		}

		now := time.Now()
		if now.Sub(lastCleanup) >= time.Minute {
			cleanupSingBoxAuditPartials(pending, now)
			lastCleanup = now
		}

		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			return
		}
		if rotated(file, path) {
			return
		}
		if !sleepContext(ctx, time.Second) {
			return
		}
	}
}

func parseSingBoxAccessLogLine(line string, now time.Time, pending map[string]*singBoxAuditPartial) (ConnectionAuditEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return ConnectionAuditEvent{}, false
	}
	if event, ok := parseSingBoxJSONAccessLogLine(line, now); ok {
		return event, true
	}
	if event, ok := parseSingBoxFlowLine(line, now); ok {
		return event, true
	}

	id := singBoxLogID(line)
	if id == "" {
		return ConnectionAuditEvent{}, false
	}
	partial := pending[id]
	if partial == nil {
		partial = &singBoxAuditPartial{started: now}
		pending[id] = partial
	}
	partial.updated = now

	if serviceName := singBoxServiceName(line); serviceName != "" && partial.serviceName == "" {
		partial.serviceName = serviceName
	}
	if clientAddr := singBoxAddressMatch(singBoxFromPattern, line); clientAddr != "" {
		partial.clientAddr = clientAddr
	}
	if targetAddr := singBoxAddressMatch(singBoxToPattern, line); targetAddr != "" {
		partial.targetAddr = cleanAuditTarget(targetAddr)
	}
	if protocol := singBoxProtocol(line, partial.targetAddr); protocol != "" {
		partial.protocol = protocol
	}
	if errorText := singBoxErrorText(line); errorText != "" {
		partial.errorText = errorText
	}

	if partial.clientAddr == "" || (partial.targetAddr == "" && partial.errorText == "") {
		return ConnectionAuditEvent{}, false
	}
	delete(pending, id)
	return singBoxPartialEvent(partial, now), true
}

func parseSingBoxJSONAccessLogLine(line string, now time.Time) (ConnectionAuditEvent, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return ConnectionAuditEvent{}, false
	}
	clientAddr := firstJSONString(data, "clientAddr", "client_addr", "client", "source", "src", "from", "remote_addr", "remoteAddr")
	targetAddr := firstJSONString(data, "targetAddr", "target_addr", "target", "destination", "dest", "dst", "server", "domain")
	if targetAddr == "" {
		if message := firstJSONString(data, "message", "msg"); message != "" {
			return parseSingBoxFlowLine(message, now)
		}
	}
	if clientAddr == "" || targetAddr == "" {
		return ConnectionAuditEvent{}, false
	}
	protocol := singBoxProtocol(" "+firstJSONString(data, "protocol", "network", "proto")+" ", targetAddr)
	serviceName := firstJSONString(data, "serviceName", "service", "inbound", "inboundTag", "inbound_tag", "tag")
	return ConnectionAuditEvent{
		ServiceName: singBoxDefaultServiceName(serviceName),
		ClientAddr:  cleanSingBoxAddress(clientAddr),
		TargetAddr:  cleanAuditTarget(cleanSingBoxAddress(targetAddr)),
		Protocol:    protocol,
		StartedTime: now.UnixMilli(),
		EndedTime:   now.UnixMilli(),
		Error:       firstJSONString(data, "error", "err"),
	}, true
}

func parseSingBoxFlowLine(line string, now time.Time) (ConnectionAuditEvent, bool) {
	protocol := ""
	clientAddr := ""
	targetAddr := ""
	if matches := singBoxFlowPattern.FindStringSubmatch(line); len(matches) == 4 {
		protocol = strings.ToLower(matches[1])
		clientAddr = matches[2]
		targetAddr = matches[3]
	} else if matches := singBoxArrowPattern.FindStringSubmatch(line); len(matches) == 3 {
		clientAddr = matches[1]
		targetAddr = matches[2]
	}
	clientAddr = cleanSingBoxAddress(clientAddr)
	targetAddr = cleanAuditTarget(cleanSingBoxAddress(targetAddr))
	if clientAddr == "" || targetAddr == "" || !looksLikeAddress(clientAddr) || !looksLikeAddress(targetAddr) {
		return ConnectionAuditEvent{}, false
	}
	protocol = singBoxProtocol(" "+protocol+" "+line, targetAddr)
	return ConnectionAuditEvent{
		ServiceName: singBoxDefaultServiceName(singBoxServiceName(line)),
		ClientAddr:  clientAddr,
		TargetAddr:  targetAddr,
		Protocol:    protocol,
		StartedTime: now.UnixMilli(),
		EndedTime:   now.UnixMilli(),
		Error:       singBoxErrorText(line),
	}, true
}

func singBoxPartialEvent(partial *singBoxAuditPartial, now time.Time) ConnectionAuditEvent {
	started := partial.started
	if started.IsZero() {
		started = now
	}
	protocol := partial.protocol
	if protocol == "" {
		protocol = singBoxProtocol("", partial.targetAddr)
	}
	return ConnectionAuditEvent{
		ServiceName: singBoxDefaultServiceName(partial.serviceName),
		ClientAddr:  cleanSingBoxAddress(partial.clientAddr),
		TargetAddr:  cleanAuditTarget(cleanSingBoxAddress(partial.targetAddr)),
		Protocol:    protocol,
		DurationMs:  now.Sub(started).Milliseconds(),
		StartedTime: started.UnixMilli(),
		EndedTime:   now.UnixMilli(),
		Error:       partial.errorText,
	}
}

func singBoxLogID(line string) string {
	matches := singBoxLogIDPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func singBoxServiceName(line string) string {
	matches := singBoxComponentTagPattern.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			return "sing-box:" + strings.TrimSpace(match[1])
		}
	}
	return ""
}

func singBoxAddressMatch(pattern *regexp.Regexp, line string) string {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return ""
	}
	return cleanSingBoxAddress(matches[1])
}

func cleanSingBoxAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.Trim(addr, "\"'`,;()")
	addr = strings.TrimRight(addr, ":")
	addr = strings.TrimPrefix(addr, "tcp://")
	addr = strings.TrimPrefix(addr, "udp://")
	addr = strings.TrimPrefix(addr, "tls://")
	return strings.TrimSpace(addr)
}

func looksLikeAddress(addr string) bool {
	if addr == "" {
		return false
	}
	if strings.Contains(addr, "://") {
		return false
	}
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return true
	}
	if strings.Count(addr, ":") == 1 {
		parts := strings.Split(addr, ":")
		if len(parts) == 2 {
			port, err := strconv.Atoi(parts[1])
			return err == nil && port > 0
		}
	}
	return strings.Contains(addr, ".")
}

func singBoxProtocol(line string, target string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, " udp ") || strings.Contains(lower, "network=udp") || strings.Contains(lower, "network:udp") {
		if auditTargetPort(target) == 53 {
			return "dns"
		}
		return "udp"
	}
	if strings.Contains(lower, " tcp ") || strings.Contains(lower, "network=tcp") || strings.Contains(lower, "network:tcp") {
		if port := auditTargetPort(target); port != 0 {
			return protocolFromPort(port)
		}
		return "tcp"
	}
	if port := auditTargetPort(target); port != 0 {
		return protocolFromPort(port)
	}
	return "tcp"
}

func protocolFromPort(port int) string {
	switch port {
	case 22:
		return "ssh"
	case 53:
		return "dns"
	case 80:
		return "http"
	case 443:
		return "https"
	case 25, 465, 587:
		return "smtp"
	default:
		return "tcp"
	}
}

func singBoxErrorText(line string) string {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "error") && !strings.Contains(lower, "failed") && !strings.Contains(lower, "reject") {
		return ""
	}
	if idx := strings.Index(line, ": "); idx >= 0 && idx+2 < len(line) {
		return strings.TrimSpace(line[idx+2:])
	}
	return strings.TrimSpace(line)
}

func singBoxDefaultServiceName(serviceName string) string {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return "sing-box"
	}
	if strings.HasPrefix(serviceName, "sing-box") {
		return serviceName
	}
	return "sing-box:" + serviceName
}

func firstJSONString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatInt(int64(typed), 10)
		}
	}
	return ""
}

func cleanupSingBoxAuditPartials(pending map[string]*singBoxAuditPartial, now time.Time) {
	for id, partial := range pending {
		if partial == nil || now.Sub(partial.updated) > 10*time.Minute {
			delete(pending, id)
		}
	}
}

func rotated(file *os.File, path string) bool {
	pos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return true
	}
	if info.Size() < pos {
		return true
	}
	current, err := os.Stat(path)
	if err != nil {
		return true
	}
	return !os.SameFile(info, current)
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
