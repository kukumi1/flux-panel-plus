package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

type cloudflareCredential struct {
	ApiToken string `json:"apiToken"`
}

type cloudflare struct {
	token  string
	client *http.Client
}

func newCloudflare(credentialJSON string) (Provider, error) {
	var cred cloudflareCredential
	if err := json.Unmarshal([]byte(credentialJSON), &cred); err != nil {
		return nil, fmt.Errorf("cloudflare credential invalid: %w", err)
	}
	if cred.ApiToken == "" {
		return nil, fmt.Errorf("cloudflare credential missing apiToken")
	}
	return &cloudflare{token: cred.ApiToken, client: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *cloudflare) SetRecord(domain, recordName, recordType, ip string) error {
	zoneID, err := c.zoneID(domain)
	if err != nil {
		return err
	}
	name := fqdn(domain, recordName)

	recordID, err := c.recordID(zoneID, name, recordType)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"type":    recordType,
		"name":    name,
		"content": ip,
		"ttl":     60,
		"proxied": false,
	}
	if recordID == "" {
		return c.do(http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), payload, nil)
	}
	return c.do(http.MethodPut, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), payload, nil)
}

func (c *cloudflare) zoneID(domain string) (string, error) {
	var out struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.do(http.MethodGet, "/zones?name="+url.QueryEscape(domain), nil, &out); err != nil {
		return "", err
	}
	if len(out.Result) == 0 {
		return "", fmt.Errorf("cloudflare zone not found for domain %q", domain)
	}
	return out.Result[0].ID, nil
}

func (c *cloudflare) recordID(zoneID, name, recordType string) (string, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?type=%s&name=%s", zoneID, url.QueryEscape(recordType), url.QueryEscape(name))
	var out struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	if len(out.Result) == 0 {
		return "", nil
	}
	return out.Result[0].ID, nil
}

func (c *cloudflare) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, cloudflareAPIBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("cloudflare response parse failed (HTTP %d): %w", resp.StatusCode, err)
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return fmt.Errorf("cloudflare API error %d: %s", envelope.Errors[0].Code, envelope.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare API request failed (HTTP %d)", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
