package service

import (
	"strings"
	"testing"

	"flux-panel/go-backend/model"
)

func TestShouldStoreConnectionAudit(t *testing.T) {
	tests := []struct {
		name   string
		record model.ConnectionAudit
		want   bool
	}{
		{
			name: "drops route transport audit",
			record: model.ConnectionAudit{
				ServiceName: "7_2_0",
				RouteId:     2,
				TargetAddr:  "relay.example.com:30009",
			},
			want: false,
		},
		{
			name: "keeps zero byte sing-box final destination audit",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				TargetHost:  "linux.do",
				TargetPort:  443,
				TargetAddr:  "linux.do:443",
			},
			want: true,
		},
		{
			name: "drops sing-box connectivity probe regardless of bytes",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				TargetHost:  "www.gstatic.com",
				TargetPort:  443,
				TargetAddr:  "www.gstatic.com:443",
				DownBytes:   1,
			},
			want: false,
		},
		{
			name: "drops direct GOST transport audit",
			record: model.ConnectionAudit{
				ServiceName: "3_2_0",
				TargetAddr:  "example.com:443",
			},
			want: false,
		},
		{
			name: "drops DNS query by port regardless of bytes",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				TargetHost:  "8.8.8.8",
				TargetPort:  53,
				TargetAddr:  "8.8.8.8:53",
				Protocol:    "dns",
				UpBytes:     64,
				DownBytes:   128,
			},
			want: false,
		},
		{
			name: "drops browser extension config noise",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				TargetHost:  "config.immersivetranslate.com",
				TargetPort:  443,
				TargetAddr:  "config.immersivetranslate.com:443",
				Protocol:    "https",
			},
			want: false,
		},
		{
			name: "drops failed handshake with no target",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				Protocol:    "tcp",
				Error:       "process connection from 203.0.113.25:32795: shadowsocks: serve TCP from 203.0.113.25:32795: cipher: message authentication failed",
			},
			want: false,
		},
		{
			name: "keeps blocked attempt that reached a target",
			record: model.ConnectionAudit{
				ServiceName: "sing-box:Shadowsocks-32669.json",
				TargetHost:  "claude.ai",
				TargetPort:  443,
				TargetAddr:  "claude.ai:443",
				Error:       "connection refused",
			},
			want: true,
		},
		{
			name: "keeps xray access audit",
			record: model.ConnectionAudit{
				ServiceName: "xray:inbound-443",
				TargetHost:  "youtube.com",
				TargetPort:  443,
				TargetAddr:  "youtube.com:443",
				ClientEmail: "f4rpa0dp",
			},
			want: true,
		},
		{
			name: "drops xray dns query by port",
			record: model.ConnectionAudit{
				ServiceName: "xray:inbound-443",
				TargetHost:  "8.8.8.8",
				TargetPort:  53,
				TargetAddr:  "8.8.8.8:53",
				ClientEmail: "f4rpa0dp",
			},
			want: false,
		},
		{
			name: "keeps gost domain audit",
			record: model.ConnectionAudit{
				ServiceName: "gost:7_2_0",
				TargetHost:  "github.com",
				TargetPort:  443,
				TargetAddr:  "github.com:443",
			},
			want: true,
		},
		{
			name: "keeps gost direct ip audit",
			record: model.ConnectionAudit{
				ServiceName: "gost:5_2_0",
				TargetHost:  "1.1.1.1",
				TargetPort:  443,
				TargetAddr:  "1.1.1.1:443",
			},
			want: true,
		},
		{
			name: "drops gost dns query by port",
			record: model.ConnectionAudit{
				ServiceName: "gost:7_2_0",
				TargetHost:  "8.8.8.8",
				TargetPort:  53,
				TargetAddr:  "8.8.8.8:53",
			},
			want: false,
		},
		{
			name: "drops gost connectivity probe noise",
			record: model.ConnectionAudit{
				ServiceName: "gost:7_2_0",
				TargetHost:  "captive.apple.com",
				TargetPort:  443,
				TargetAddr:  "captive.apple.com:443",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStoreConnectionAudit(tt.record); got != tt.want {
				t.Fatalf("shouldStoreConnectionAudit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConnectionAuditVisibleScope(t *testing.T) {
	where, args := connectionAuditVisibleScope()
	if !containsAll(where, []string{"service_name LIKE ?", "target_host", "target_port <> ?", "NOT IN"}) {
		t.Fatalf("where = %q", where)
	}
	if len(args) != len(singBoxNoiseTargetList)+4 {
		t.Fatalf("args len = %d", len(args))
	}
	if args[0] != "sing-box%" {
		t.Fatalf("first arg = %v", args[0])
	}
	if args[1] != "xray%" {
		t.Fatalf("second arg = %v", args[1])
	}
	if args[2] != "gost%" {
		t.Fatalf("third arg = %v", args[2])
	}
	if args[3] != dnsTargetPort {
		t.Fatalf("fourth arg = %v", args[3])
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
