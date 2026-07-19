package ddns

import (
	"fmt"
	"strings"
)

// Provider updates a DNS A/AAAA record so it points at a given IP.
type Provider interface {
	SetRecord(domain, recordName, recordType, ip string) error
}

// New builds a Provider from its type and a JSON-encoded credential blob.
func New(providerType, credentialJSON string) (Provider, error) {
	switch providerType {
	case "cloudflare":
		return newCloudflare(credentialJSON)
	case "webhook":
		return newWebhook(credentialJSON)
	case "aliyun", "tencent", "huawei":
		return nil, fmt.Errorf("ddns provider %q not implemented yet", providerType)
	default:
		return nil, fmt.Errorf("unsupported ddns provider type: %q", providerType)
	}
}

// SupportedTypes lists provider types that are fully implemented.
func SupportedTypes() []string {
	return []string{"cloudflare", "webhook"}
}

// fqdn resolves recordName against domain into a fully-qualified name.
// recordName may be a bare subdomain ("vpn"), "@"/"" for the apex, or an
// already-qualified name ("vpn.example.com").
func fqdn(domain, recordName string) string {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	recordName = strings.TrimSuffix(strings.TrimSpace(recordName), ".")
	if recordName == "" || recordName == "@" {
		return domain
	}
	if recordName == domain || strings.HasSuffix(recordName, "."+domain) {
		return recordName
	}
	return recordName + "." + domain
}
