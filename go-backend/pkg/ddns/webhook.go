package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// webhookCredential configures a generic HTTP callback used as an escape hatch
// for registrars without a native provider. Placeholders {domain} {name}
// {fqdn} {type} {ip} are substituted in url and body before the request.
type webhookCredential struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type webhook struct {
	cred   webhookCredential
	client *http.Client
}

func newWebhook(credentialJSON string) (Provider, error) {
	var cred webhookCredential
	if err := json.Unmarshal([]byte(credentialJSON), &cred); err != nil {
		return nil, fmt.Errorf("webhook credential invalid: %w", err)
	}
	if strings.TrimSpace(cred.URL) == "" {
		return nil, fmt.Errorf("webhook credential missing url")
	}
	if cred.Method == "" {
		cred.Method = http.MethodGet
	}
	return &webhook{cred: cred, client: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (w *webhook) SetRecord(domain, recordName, recordType, ip string) error {
	replacer := strings.NewReplacer(
		"{domain}", domain,
		"{name}", recordName,
		"{fqdn}", fqdn(domain, recordName),
		"{type}", recordType,
		"{ip}", ip,
	)

	target := replacer.Replace(w.cred.URL)
	var reader io.Reader
	if w.cred.Body != "" {
		reader = bytes.NewReader([]byte(replacer.Replace(w.cred.Body)))
	}

	req, err := http.NewRequest(strings.ToUpper(w.cred.Method), target, reader)
	if err != nil {
		return err
	}
	for k, v := range w.cred.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
