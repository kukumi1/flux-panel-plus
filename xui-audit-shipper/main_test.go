package main

import "testing"

func TestParseLine(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		ok       bool
		email    string
		target   string
		service  string
		protocol string
	}{
		{
			name:     "https domain access",
			line:     "2026/06/29 03:30:36.622338 from 203.0.113.25:30000 accepted tcp:service.example.com:443 [inbound-443 >> direct] email: example-user",
			ok:       true,
			email:    "example-user",
			target:   "service.example.com:443",
			service:  "xray:inbound-443",
			protocol: "https",
		},
		{
			name: "skip api internal (no email)",
			line: "2026/06/29 03:30:31.003432 from 127.0.0.1:55936 accepted tcp:127.0.0.1:62789 [api -> api]",
			ok:   false,
		},
		{
			name: "skip dns query by port",
			line: "2026/06/29 03:30:36 from 1.2.3.4:5000 accepted udp:8.8.8.8:53 [inbound-443 >> direct] email: foo",
			ok:   false,
		},
		{
			name: "skip loopback source",
			line: "2026/06/29 03:30:36 from 127.0.0.1:5000 accepted tcp:example.com:443 [inbound-443 >> direct] email: bar",
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := parseLine(c.line)
			if ok != c.ok {
				t.Fatalf("ok=%v want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if ev.ClientEmail != c.email {
				t.Errorf("email=%q want %q", ev.ClientEmail, c.email)
			}
			if ev.TargetAddr != c.target {
				t.Errorf("target=%q want %q", ev.TargetAddr, c.target)
			}
			if ev.ServiceName != c.service {
				t.Errorf("service=%q want %q", ev.ServiceName, c.service)
			}
			if ev.Protocol != c.protocol {
				t.Errorf("protocol=%q want %q", ev.Protocol, c.protocol)
			}
		})
	}
}
