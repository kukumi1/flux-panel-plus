package service

import (
	"sync"
	"testing"
	"time"

	"flux-panel/go-backend/pkg"
)

func TestTelegramDiagnosisJobKeySeparatesChatsAndTargets(t *testing.T) {
	jobs := sync.Map{}
	first := telegramDiagnosisJobKey{ChatID: 1, Kind: "forward", ID: 7}
	if _, loaded := jobs.LoadOrStore(first, struct{}{}); loaded {
		t.Fatal("first diagnosis job should be accepted")
	}
	if _, loaded := jobs.LoadOrStore(first, struct{}{}); !loaded {
		t.Fatal("duplicate diagnosis job should be rejected")
	}
	if _, loaded := jobs.LoadOrStore(telegramDiagnosisJobKey{ChatID: 2, Kind: "forward", ID: 7}, struct{}{}); loaded {
		t.Fatal("different chat should have an independent diagnosis job")
	}
	if _, loaded := jobs.LoadOrStore(telegramDiagnosisJobKey{ChatID: 1, Kind: "tunnel", ID: 7}, struct{}{}); loaded {
		t.Fatal("different diagnosis kind should have an independent job")
	}
}

func TestTelegramPageBounds(t *testing.T) {
	tests := []struct {
		name                         string
		total, page, size            int
		wantPage, wantStart, wantEnd int
		wantPages                    int
	}{
		{name: "empty", total: 0, page: 0, size: 6, wantPage: 0, wantStart: 0, wantEnd: 0, wantPages: 1},
		{name: "first", total: 13, page: 0, size: 6, wantPage: 0, wantStart: 0, wantEnd: 6, wantPages: 3},
		{name: "last", total: 13, page: 2, size: 6, wantPage: 2, wantStart: 12, wantEnd: 13, wantPages: 3},
		{name: "clamp high", total: 7, page: 99, size: 6, wantPage: 1, wantStart: 6, wantEnd: 7, wantPages: 2},
		{name: "clamp low", total: 7, page: -1, size: 6, wantPage: 0, wantStart: 0, wantEnd: 6, wantPages: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, start, end, pages := telegramPageBounds(tt.total, tt.page, tt.size)
			if page != tt.wantPage || start != tt.wantStart || end != tt.wantEnd || pages != tt.wantPages {
				t.Fatalf("telegramPageBounds() = (%d, %d, %d, %d), want (%d, %d, %d, %d)", page, start, end, pages, tt.wantPage, tt.wantStart, tt.wantEnd, tt.wantPages)
			}
		})
	}
}

func TestTelegramTruncate(t *testing.T) {
	if got := telegramTruncate("abc\ndef", 20); got != "abc def" {
		t.Fatalf("newline normalization = %q", got)
	}
	if got := telegramTruncate("中文测试文本", 5); got != "中文测试…" {
		t.Fatalf("rune truncation = %q", got)
	}
}

func TestTelegramLimitMessagePreservesLayout(t *testing.T) {
	if got := telegramLimitMessage("line 1\nline 2", 20); got != "line 1\nline 2" {
		t.Fatalf("message layout = %q", got)
	}
}

func TestTelegramHumanUptime(t *testing.T) {
	tests := map[uint64]string{
		59:     "0分钟",
		3660:   "1小时 1分钟",
		176400: "2天 1小时",
	}
	for input, want := range tests {
		if got := telegramHumanUptime(input); got != want {
			t.Fatalf("telegramHumanUptime(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestTelegramAuditHealthText(t *testing.T) {
	if got := telegramAuditHealthText(pkg.SingBoxAuditInfo{LogReadable: true, TailerRunning: true}); got != "正常" {
		t.Fatalf("healthy audit status = %q", got)
	}
	if got := telegramAuditHealthText(pkg.SingBoxAuditInfo{LastError: "failed"}); got != "异常" {
		t.Fatalf("error audit status = %q", got)
	}
	if got := telegramAuditHealthText(pkg.SingBoxAuditInfo{LogReadable: true}); got != "日志可读，采集器未运行" {
		t.Fatalf("readable audit status = %q", got)
	}
}

func TestTelegramCallbackParsing(t *testing.T) {
	parts := []string{"fw", "t", "42", "1", "3"}
	if id, ok := telegramParseInt(parts, 2); !ok || id != 42 {
		t.Fatalf("forward id = %d, ok = %v", id, ok)
	}
	if _, ok := telegramParseInt(parts, 9); ok {
		t.Fatal("out-of-range callback index should fail")
	}
	if _, ok := telegramParseInt([]string{"nd", "v", "-1"}, 2); ok {
		t.Fatal("negative callback value should fail")
	}
}

func TestTelegramNormalizeRemoteAddr(t *testing.T) {
	got, err := telegramNormalizeRemoteAddr("1.2.3.4:80\nexample.com:443")
	if err != "" || got != "1.2.3.4:80,example.com:443" {
		t.Fatalf("normalize remote address = %q, %q", got, err)
	}
	if _, err := telegramNormalizeRemoteAddr("127.0.0.1"); err == "" {
		t.Fatal("address without port should fail")
	}
	if _, err := telegramNormalizeRemoteAddr("example.com:70000"); err == "" {
		t.Fatal("out-of-range port should fail")
	}
}

func TestTelegramWizardStateIsolationAndExpiry(t *testing.T) {
	const chatID int64 = 91001
	const userID int64 = 92001
	telegramWizardClear(chatID)
	t.Cleanup(func() { telegramWizardClear(chatID) })

	telegramWizardStart(chatID, userID, telegramWizard{Kind: "forward", Step: "mode"})
	if _, ok := telegramWizardRead(chatID, userID+1); ok {
		t.Fatal("wizard must not be visible to another user")
	}
	updated, ok := telegramWizardUpdate(chatID, userID, "forward", "mode", func(w *telegramWizard) {
		w.Step = "path"
	})
	if !ok || updated.Step != "path" {
		t.Fatalf("wizard update = %#v, ok = %v", updated, ok)
	}
	if _, ok := telegramWizardUpdate(chatID, userID, "forward", "mode", func(*telegramWizard) {}); ok {
		t.Fatal("stale wizard step must not update")
	}

	telegramWizardState.Lock()
	telegramWizardState.items[chatID].ExpiresAt = time.Now().Add(-time.Second)
	telegramWizardState.Unlock()
	if _, ok := telegramWizardRead(chatID, userID); ok {
		t.Fatal("expired wizard should not be returned")
	}
}

func TestTelegramBotCommands(t *testing.T) {
	seen := make(map[string]bool, len(telegramBotCommands))
	for _, command := range telegramBotCommands {
		if command.Command == "" || command.Description == "" {
			t.Fatalf("invalid command definition: %#v", command)
		}
		if seen[command.Command] {
			t.Fatalf("duplicate command: %s", command.Command)
		}
		seen[command.Command] = true
	}
	for _, required := range []string{"menu", "new_tunnel", "new_forward", "nodes", "audit"} {
		if !seen[required] {
			t.Fatalf("missing required command: %s", required)
		}
	}
}
