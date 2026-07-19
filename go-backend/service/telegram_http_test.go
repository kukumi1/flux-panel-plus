package service

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramHTTPClientReusesConnection(t *testing.T) {
	var newConnections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	client := newTelegramHTTPClient(2 * time.Second)
	defer client.CloseIdleConnections()
	for i := 0; i < 2; i++ {
		resp, err := client.Post(server.URL, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		_, consumeErr := telegramConsumeResponse(resp)
		closeErr := resp.Body.Close()
		if consumeErr != nil {
			t.Fatalf("request %d response consume failed: %v", i+1, consumeErr)
		}
		if closeErr != nil {
			t.Fatalf("request %d response close failed: %v", i+1, closeErr)
		}
	}

	if got := newConnections.Load(); got != 1 {
		t.Fatalf("new connections = %d, want 1", got)
	}
}

func TestTelegramConsumeResponseReturnsErrorBodyAndDrains(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader("  invalid callback  ")),
	}
	body, err := telegramConsumeResponse(resp)
	if err != nil {
		t.Fatalf("telegramConsumeResponse() error = %v", err)
	}
	if body != "invalid callback" {
		t.Fatalf("telegramConsumeResponse() body = %q", body)
	}
}
