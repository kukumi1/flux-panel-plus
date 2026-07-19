package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	xrecorder "github.com/go-gost/x/recorder"
)

var auditReportURL string
var auditNodeSecret string
var auditQueue = make(chan ConnectionAuditEvent, 1024)
var auditOnce sync.Once
var singBoxAuditOnce sync.Once

type ConnectionAuditEvent struct {
	ServiceName string `json:"serviceName"`
	ClientAddr  string `json:"clientAddr"`
	TargetAddr  string `json:"targetAddr"`
	Protocol    string `json:"protocol"`
	UpBytes     int64  `json:"upBytes"`
	DownBytes   int64  `json:"downBytes"`
	DurationMs  int64  `json:"durationMs"`
	StartedTime int64  `json:"startedTime"`
	EndedTime   int64  `json:"endedTime"`
	Error       string `json:"error"`
}

type connectionAuditUpload struct {
	Events []ConnectionAuditEvent `json:"events"`
}

func init() {
	xrecorder.AuditRecordHook = reportHandlerAudit
}

func reportHandlerAudit(_ context.Context, ro *xrecorder.HandlerRecorderObject) {
	if ro == nil || ro.Service == "" || ro.ClientIP == "" {
		return
	}

	targetAddr := auditTargetFromRecorder(ro)
	if targetAddr == "" && ro.Err == "" {
		return
	}

	started := ro.Time
	if started.IsZero() {
		started = time.Now()
	}
	durationMs := ro.Duration.Milliseconds()
	ended := started.Add(ro.Duration)
	if ro.Duration <= 0 {
		ended = time.Now()
	}

	enqueueConnectionAudit(ConnectionAuditEvent{
		ServiceName: ro.Service,
		ClientAddr:  auditClientAddr(ro),
		TargetAddr:  targetAddr,
		Protocol:    auditProtocolFromRecorder(ro, targetAddr),
		UpBytes:     int64(ro.OutputBytes),
		DownBytes:   int64(ro.InputBytes),
		DurationMs:  durationMs,
		StartedTime: started.UnixMilli(),
		EndedTime:   ended.UnixMilli(),
		Error:       strings.TrimSpace(ro.Err),
	})
}

func auditClientAddr(ro *xrecorder.HandlerRecorderObject) string {
	if strings.TrimSpace(ro.ClientIP) != "" {
		return strings.TrimSpace(ro.ClientIP)
	}
	return strings.TrimSpace(ro.RemoteAddr)
}

func auditTargetFromRecorder(ro *xrecorder.HandlerRecorderObject) string {
	if ro.HTTP != nil {
		if host := strings.TrimSpace(ro.HTTP.Host); host != "" {
			return cleanAuditTarget(host)
		}
	}
	if ro.TLS != nil {
		if serverName := strings.TrimSpace(ro.TLS.ServerName); serverName != "" {
			return cleanAuditTarget(defaultPortTarget(serverName, 443))
		}
	}
	if ro.DNS != nil {
		if name := strings.TrimSpace(ro.DNS.Name); name != "" {
			return cleanAuditTarget(strings.TrimSuffix(name, "."))
		}
		if question := strings.TrimSpace(ro.DNS.Question); question != "" {
			return cleanAuditTarget(strings.TrimSuffix(question, "."))
		}
	}
	if host := strings.TrimSpace(ro.Host); host != "" {
		return cleanAuditTarget(host)
	}
	if dst := strings.TrimSpace(ro.Dst); dst != "" {
		return cleanAuditTarget(dst)
	}
	return ""
}

func cleanAuditTarget(target string) string {
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")
	target = strings.TrimSuffix(target, "/")
	return strings.TrimSuffix(target, ".")
}

func defaultPortTarget(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if strings.Count(host, ":") == 1 {
		parts := strings.Split(host, ":")
		if _, err := strconv.Atoi(parts[1]); err == nil {
			return host
		}
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

func auditProtocolFromRecorder(ro *xrecorder.HandlerRecorderObject, target string) string {
	if ro.DNS != nil {
		return "dns"
	}
	if ro.HTTP != nil {
		method := strings.ToUpper(strings.TrimSpace(ro.HTTP.Method))
		if method == "CONNECT" || auditTargetPort(target) == 443 {
			return "https"
		}
		return "http"
	}
	if ro.TLS != nil {
		return "https"
	}
	if port := auditTargetPort(target); port != 0 {
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
		}
	}
	if proto := strings.ToLower(strings.TrimSpace(ro.Proto)); proto != "" {
		return proto
	}
	if strings.HasSuffix(strings.ToLower(ro.Service), "_udp") {
		return "udp"
	}
	return "tcp"
}

func auditTargetPort(target string) int {
	_, portText, err := net.SplitHostPort(target)
	if err == nil {
		port, _ := strconv.Atoi(portText)
		return port
	}
	if strings.Count(target, ":") == 1 {
		parts := strings.Split(target, ":")
		port, _ := strconv.Atoi(parts[1])
		return port
	}
	return 0
}

func SetAuditReportURL(addr string, secret string, useTLS bool) {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	auditReportURL = scheme + "://" + addr + "/flow/audit?secret=" + secret
	auditNodeSecret = secret
	auditOnce.Do(func() {
		go auditReporterLoop(context.Background())
	})
	singBoxAuditOnce.Do(func() {
		go tailSingBoxAccessLog(context.Background(), defaultSingBoxAccessLogPath)
	})
}

func enqueueConnectionAudit(event ConnectionAuditEvent) {
	if auditReportURL == "" || event.ServiceName == "" || event.ClientAddr == "" {
		return
	}
	select {
	case auditQueue <- event:
	default:
		// Drop audit events under pressure; forwarding must remain the priority.
	}
}

func auditReporterLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	batch := make([]ConnectionAuditEvent, 0, 100)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := sendAuditBatch(ctx, batch); err != nil {
			fmt.Printf("send audit report failed: %v\n", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case event := <-auditQueue:
			batch = append(batch, event)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func sendAuditBatch(ctx context.Context, events []ConnectionAuditEvent) error {
	body, err := json.Marshal(connectionAuditUpload{Events: events})
	if err != nil {
		return err
	}
	requestBody := body
	if httpAESCrypto != nil {
		if encryptedData, err := httpAESCrypto.Encrypt(body); err == nil {
			requestBody, _ = json.Marshal(map[string]interface{}{
				"encrypted": true,
				"data":      encryptedData,
				"timestamp": time.Now().Unix(),
			})
		}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", auditReportURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Audit-Reporter/1.0")
	if auditNodeSecret != "" {
		req.Header.Set("X-Node-Secret", auditNodeSecret)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return nil
}
