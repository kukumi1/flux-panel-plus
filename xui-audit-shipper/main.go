package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type auditEvent struct {
	ServiceName string `json:"serviceName"`
	ClientAddr  string `json:"clientAddr"`
	ClientEmail string `json:"clientEmail"`
	TargetAddr  string `json:"targetAddr"`
	Protocol    string `json:"protocol"`
	StartedTime int64  `json:"startedTime"`
	EndedTime   int64  `json:"endedTime"`
}

type auditUpload struct {
	Events []auditEvent `json:"events"`
}

type config struct {
	panelURL string
	secret   string
	logPath  string
	interval time.Duration
}

// xray access log: "<time> from <src> accepted <net>:<dst> [<inbound> >> <out>] email: <email>"
var accessLine = regexp.MustCompile(`from (\S+) accepted (tcp|udp):(\S+) \[([^\]]+)\] email: (\S+)`)

func main() {
	cfg := loadConfig()
	queue := make(chan auditEvent, 1024)
	go reporter(cfg, queue)
	log.Printf("xui-audit-shipper started: panel=%s log=%s", cfg.panelURL, cfg.logPath)
	tail(cfg.logPath, queue)
}

func loadConfig() config {
	cfg := config{
		panelURL: strings.TrimRight(getenv("PANEL_URL", ""), "/"),
		secret:   getenv("INGEST_SECRET", ""),
		logPath:  getenv("LOG_PATH", "/var/log/x-ui/access.log"),
		interval: 5 * time.Second,
	}
	if cfg.panelURL == "" || cfg.secret == "" {
		log.Fatal("PANEL_URL and INGEST_SECRET are required")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// tail follows the access log, emitting only lines written after startup and
// surviving log rotation. Forwarding correctness is never at stake here, so
// events are best-effort: dropped on backpressure or transient read errors.
func tail(path string, queue chan<- auditEvent) {
	var offset int64 = -1
	for {
		file, err := os.Open(path)
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		size := int64(0)
		if info, err := file.Stat(); err == nil {
			size = info.Size()
		}
		switch {
		case offset < 0:
			offset = size
		case offset > size:
			offset = 0
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			file.Close()
			time.Sleep(time.Second)
			continue
		}
		reader := bufio.NewReader(file)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				if event, ok := parseLine(line); ok {
					select {
					case queue <- event:
					default:
					}
				}
			}
			if err != nil {
				break
			}
		}
		offset, _ = file.Seek(0, io.SeekCurrent)
		file.Close()
		time.Sleep(time.Second)
	}
}

func parseLine(line string) (auditEvent, bool) {
	m := accessLine.FindStringSubmatch(line)
	if m == nil {
		return auditEvent{}, false
	}
	src, network, dst, inbound, email := m[1], m[2], m[3], m[4], m[5]

	if host, _, err := net.SplitHostPort(src); err == nil {
		if host == "127.0.0.1" || host == "::1" {
			return auditEvent{}, false
		}
	}

	port := 0
	if _, portStr, err := net.SplitHostPort(dst); err == nil {
		port, _ = strconv.Atoi(portStr)
	}
	if port == 53 {
		return auditEvent{}, false
	}

	inboundTag := strings.TrimSpace(strings.SplitN(inbound, ">>", 2)[0])
	now := time.Now().UnixMilli()
	return auditEvent{
		ServiceName: "xray:" + inboundTag,
		ClientAddr:  src,
		ClientEmail: email,
		TargetAddr:  dst,
		Protocol:    protocolFromPort(port, network),
		StartedTime: now,
		EndedTime:   now,
	}, true
}

func protocolFromPort(port int, network string) string {
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
	if network != "" {
		return network
	}
	return "tcp"
}

func reporter(cfg config, queue <-chan auditEvent) {
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	batch := make([]auditEvent, 0, 100)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := send(cfg, batch); err != nil {
			log.Printf("send failed (%d events dropped): %v", len(batch), err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case event := <-queue:
			batch = append(batch, event)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func send(cfg config, events []auditEvent) error {
	body, err := json.Marshal(auditUpload{Events: events})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", cfg.panelURL+"/flow/audit?secret="+cfg.secret, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Secret", cfg.secret)
	req.Header.Set("User-Agent", "xui-audit-shipper/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
