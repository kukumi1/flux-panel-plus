package service

import (
	"testing"
	"time"
)

func TestParseSingBoxAccessLogLinePairsInboundAndOutbound(t *testing.T) {
	pending := make(map[string]*singBoxAuditPartial)
	now := time.Unix(1710000000, 0)
	inbound := `+0800 2026-06-28 23:30:00 INFO [123456] inbound/vless[vless-in]: inbound connection from 203.0.113.10:52344`
	outbound := `+0800 2026-06-28 23:30:00 INFO [123456] outbound/direct[direct]: outbound connection to www.google.com:443`

	if _, ok := parseSingBoxAccessLogLine(inbound, now, pending); ok {
		t.Fatal("inbound-only line should wait for target")
	}
	event, ok := parseSingBoxAccessLogLine(outbound, now.Add(time.Second), pending)
	if !ok {
		t.Fatal("expected event after matching outbound line")
	}
	if event.ServiceName != "sing-box:vless-in" {
		t.Fatalf("service name = %q", event.ServiceName)
	}
	if event.ClientAddr != "203.0.113.10:52344" {
		t.Fatalf("client addr = %q", event.ClientAddr)
	}
	if event.TargetAddr != "www.google.com:443" {
		t.Fatalf("target addr = %q", event.TargetAddr)
	}
	if event.Protocol != "https" {
		t.Fatalf("protocol = %q", event.Protocol)
	}
	if len(pending) != 0 {
		t.Fatalf("pending entries remain: %d", len(pending))
	}
}

func TestParseSingBoxAccessLogLineSingleFlow(t *testing.T) {
	pending := make(map[string]*singBoxAuditPartial)
	line := `+0800 2026-06-28 23:31:00 INFO [abc] accepted udp 10.0.0.2:44444 -> 8.8.8.8:53`
	event, ok := parseSingBoxAccessLogLine(line, time.Unix(1710000000, 0), pending)
	if !ok {
		t.Fatal("expected single flow event")
	}
	if event.ClientAddr != "10.0.0.2:44444" {
		t.Fatalf("client addr = %q", event.ClientAddr)
	}
	if event.TargetAddr != "8.8.8.8:53" {
		t.Fatalf("target addr = %q", event.TargetAddr)
	}
	if event.Protocol != "dns" {
		t.Fatalf("protocol = %q", event.Protocol)
	}
}

func TestParseSingBoxJSONAccessLogLine(t *testing.T) {
	pending := make(map[string]*singBoxAuditPartial)
	line := `{"client_addr":"198.51.100.7:60000","target_addr":"claude.ai:443","network":"tcp","inbound":"mixed-in"}`
	event, ok := parseSingBoxAccessLogLine(line, time.Unix(1710000000, 0), pending)
	if !ok {
		t.Fatal("expected JSON event")
	}
	if event.ServiceName != "sing-box:mixed-in" {
		t.Fatalf("service name = %q", event.ServiceName)
	}
	if event.ClientAddr != "198.51.100.7:60000" {
		t.Fatalf("client addr = %q", event.ClientAddr)
	}
	if event.TargetAddr != "claude.ai:443" {
		t.Fatalf("target addr = %q", event.TargetAddr)
	}
	if event.Protocol != "https" {
		t.Fatalf("protocol = %q", event.Protocol)
	}
}

func TestParseSingBoxPanHKAccessLogSample(t *testing.T) {
	pending := make(map[string]*singBoxAuditPartial)
	now := time.Unix(1710000000, 0)
	from := `-0400 2026-06-28 11:35:39 INFO [3670248338 0ms] inbound/shadowsocks[Shadowsocks-32669.json]: inbound connection from 203.0.113.13:10607`
	to := `-0400 2026-06-28 11:35:39 INFO [3670248338 4ms] inbound/shadowsocks[Shadowsocks-32669.json]: inbound connection to www.gstatic.com:443`

	if _, ok := parseSingBoxAccessLogLine(from, now, pending); ok {
		t.Fatal("from-only line should wait for destination")
	}
	event, ok := parseSingBoxAccessLogLine(to, now.Add(4*time.Millisecond), pending)
	if !ok {
		t.Fatal("expected event from Pan-HK paired sample")
	}
	if event.ServiceName != "sing-box:Shadowsocks-32669.json" {
		t.Fatalf("service name = %q", event.ServiceName)
	}
	if event.ClientAddr != "203.0.113.13:10607" {
		t.Fatalf("client addr = %q", event.ClientAddr)
	}
	if event.TargetAddr != "www.gstatic.com:443" {
		t.Fatalf("target addr = %q", event.TargetAddr)
	}
	if event.Protocol != "https" {
		t.Fatalf("protocol = %q", event.Protocol)
	}
}

func TestParseSingBoxPanHKErrorSample(t *testing.T) {
	pending := make(map[string]*singBoxAuditPartial)
	now := time.Unix(1710000000, 0)
	from := `-0400 2026-06-28 11:06:37 INFO [746374099 0ms] inbound/shadowsocks[Shadowsocks-32669.json]: inbound connection from 198.51.100.24:15286`
	errLine := `-0400 2026-06-28 11:06:37 ERROR [746374099 0ms] inbound/shadowsocks[Shadowsocks-32669.json]: process connection from 198.51.100.24:15286: shadowsocks: serve TCP from 198.51.100.24:15286: cipher: message authentication failed`

	if _, ok := parseSingBoxAccessLogLine(from, now, pending); ok {
		t.Fatal("from-only line should wait for target or error")
	}
	event, ok := parseSingBoxAccessLogLine(errLine, now, pending)
	if !ok {
		t.Fatal("expected error event from Pan-HK sample")
	}
	if event.ClientAddr != "198.51.100.24:15286" {
		t.Fatalf("client addr = %q", event.ClientAddr)
	}
	if event.TargetAddr != "" {
		t.Fatalf("target addr = %q", event.TargetAddr)
	}
	if event.Error == "" {
		t.Fatal("expected non-empty error")
	}
}
