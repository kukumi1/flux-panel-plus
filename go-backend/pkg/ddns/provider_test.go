package ddns

import "testing"

func TestFqdn(t *testing.T) {
	cases := []struct {
		domain     string
		recordName string
		want       string
	}{
		{"example.com", "vpn", "vpn.example.com"},
		{"example.com", "", "example.com"},
		{"example.com", "@", "example.com"},
		{"example.com", "vpn.example.com", "vpn.example.com"},
		{"example.com", "example.com", "example.com"},
		{"example.com.", "vpn.", "vpn.example.com"},
		{"example.com", "a.b", "a.b.example.com"},
	}
	for _, c := range cases {
		if got := fqdn(c.domain, c.recordName); got != c.want {
			t.Errorf("fqdn(%q, %q) = %q, want %q", c.domain, c.recordName, got, c.want)
		}
	}
}

func TestNewUnsupported(t *testing.T) {
	if _, err := New("route53", "{}"); err == nil {
		t.Error("expected error for unsupported provider type")
	}
}

func TestNewNotImplemented(t *testing.T) {
	for _, typ := range []string{"aliyun", "tencent", "huawei"} {
		if _, err := New(typ, "{}"); err == nil {
			t.Errorf("expected not-implemented error for %q", typ)
		}
	}
}

func TestNewCloudflareRequiresToken(t *testing.T) {
	if _, err := New("cloudflare", `{}`); err == nil {
		t.Error("expected error when apiToken missing")
	}
	if _, err := New("cloudflare", `{"apiToken":"t"}`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewWebhookRequiresURL(t *testing.T) {
	if _, err := New("webhook", `{}`); err == nil {
		t.Error("expected error when url missing")
	}
	if _, err := New("webhook", `{"url":"https://example.com/{ip}"}`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
