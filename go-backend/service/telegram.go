package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
)

const (
	telegramAPIBase  = "https://api.telegram.org/bot"
	telegramPollSecs = 50
	telegramBindTTL  = 10 * time.Minute
)

var telegramSendClient = newTelegramHTTPClient(15 * time.Second)

type tgBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

var telegramBotCommands = []tgBotCommand{
	{Command: "start", Description: "开始使用机器人"},
	{Command: "help", Description: "查看全部命令"},
	{Command: "bind", Description: "绑定面板账号"},
	{Command: "unbind", Description: "解除 Telegram 绑定"},
	{Command: "menu", Description: "打开手机控制台"},
	{Command: "login", Description: "获取面板登录链接"},
	{Command: "usage", Description: "查看账号流量和到期时间"},
	{Command: "tunnels", Description: "查看隧道列表"},
	{Command: "new_tunnel", Description: "新增隧道（管理员）"},
	{Command: "tunnel", Description: "查看隧道详情"},
	{Command: "forwards", Description: "查看转发列表"},
	{Command: "rules", Description: "查看转发规则（旧命令）"},
	{Command: "new_forward", Description: "新增转发"},
	{Command: "forward", Description: "查看转发详情"},
	{Command: "nodes", Description: "查看节点监控"},
	{Command: "node", Description: "查看节点详情"},
	{Command: "audit", Description: "查看流量审计"},
	{Command: "enable", Description: "恢复指定转发"},
	{Command: "disable", Description: "暂停指定转发"},
	{Command: "cancel", Description: "取消当前新增向导"},
	{Command: "users", Description: "管理员查看用户概览"},
	{Command: "reset", Description: "管理员重置用户流量"},
}

func newTelegramHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 5 * time.Minute
	transport.ForceAttemptHTTP2 = true
	return &http.Client{Timeout: timeout, Transport: transport}
}

type telegramManager struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

var tgManager = &telegramManager{}

type tgBindEntry struct {
	userId  int64
	expires int64
}

var (
	tgBindMu    sync.Mutex
	tgBindCodes = map[string]tgBindEntry{}
)

// GenerateTelegramBindCode issues a short-lived code the user sends to the bot
// via /bind to link their chat to their account.
func GenerateTelegramBindCode(userId int64) string {
	code := strings.ToUpper(pkg.GenerateSecureSecret()[:8])
	now := time.Now().UnixMilli()
	expires := time.Now().Add(telegramBindTTL).UnixMilli()

	tgBindMu.Lock()
	tgBindCodes[code] = tgBindEntry{userId: userId, expires: expires}
	for k, v := range tgBindCodes {
		if now > v.expires {
			delete(tgBindCodes, k)
		}
	}
	tgBindMu.Unlock()
	return code
}

func consumeTelegramBindCode(code string) (int64, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	tgBindMu.Lock()
	defer tgBindMu.Unlock()

	entry, ok := tgBindCodes[code]
	if !ok {
		return 0, false
	}
	delete(tgBindCodes, code)
	if time.Now().UnixMilli() > entry.expires {
		return 0, false
	}
	return entry.userId, true
}

// UnbindTelegram clears the Telegram link for a user.
func UnbindTelegram(userId int64) dto.R {
	if err := DB.Model(&model.User{}).Where("id = ?", userId).Update("telegram_chat_id", 0).Error; err != nil {
		return dto.Err("解绑失败")
	}
	return dto.OkMsg()
}

// GetTelegramStatus reports whether a user has linked their Telegram chat.
func GetTelegramStatus(userId int64) dto.R {
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		return dto.Err("用户不存在")
	}
	return dto.Ok(map[string]interface{}{
		"bound":   user.TelegramChatId != 0,
		"enabled": tgReadConfig("telegram_enabled") == "true",
	})
}

func tgReadConfig(name string) string {
	var cfg model.ViteConfig
	if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
		return ""
	}
	return cfg.Value
}

// StartTelegramBot (re)starts the long-poll loop from current config.
// Safe to call repeatedly; each call cancels any previous loop first.
func StartTelegramBot() {
	tgManager.mu.Lock()
	defer tgManager.mu.Unlock()

	if tgManager.cancel != nil {
		tgManager.cancel()
		tgManager.cancel = nil
	}

	if tgReadConfig("telegram_enabled") != "true" {
		return
	}
	token := strings.TrimSpace(tgReadConfig("telegram_bot_token"))
	if token == "" {
		log.Printf("[Telegram] 已开启但未配置 bot token，跳过启动")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	tgManager.cancel = cancel
	go telegramRegisterCommands(token)
	go telegramLoop(ctx, token)
	log.Printf("[Telegram] 机器人已启动")
}

func telegramRegisterCommands(token string) {
	commands := append([]tgBotCommand(nil), telegramBotCommands...)
	telegramPost(token, "setMyCommands", map[string]interface{}{"commands": commands})
	telegramPost(token, "setChatMenuButton", map[string]interface{}{
		"menu_button": map[string]string{"type": "commands"},
	})
}

// RestartTelegramBotAsync reloads the bot after a config change without blocking
// the caller.
func RestartTelegramBotAsync() {
	go StartTelegramBot()
}

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	Chat      *tgChat `json:"chat"`
	Text      string  `json:"text"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	Message *tgMessage `json:"message"`
	Data    string     `json:"data"`
}

type tgUpdatesResponse struct {
	Ok          bool       `json:"ok"`
	Result      []tgUpdate `json:"result"`
	Description string     `json:"description"`
}

type telegramSendMessageResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

type telegramCallbackJob struct {
	Token    string
	Callback *tgCallbackQuery
}

var telegramCallbackJobs = make(chan telegramCallbackJob, 64)

func init() {
	for i := 0; i < 16; i++ {
		go func() {
			for job := range telegramCallbackJobs {
				handleTelegramCallback(job.Token, job.Callback)
			}
		}()
	}
}

func telegramLoop(ctx context.Context, token string) {
	client := &http.Client{Timeout: (telegramPollSecs + 10) * time.Second}
	var offset int64

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := telegramGetUpdates(ctx, client, token, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[Telegram] getUpdates 失败: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.CallbackQuery != nil {
				telegramDispatchCallback(token, u.CallbackQuery)
				continue
			}
			if u.Message == nil || u.Message.Chat == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			handleTelegramCommand(token, u.Message.Chat.ID, strings.TrimSpace(u.Message.Text))
		}
	}
}

func telegramDispatchCallback(token string, callback *tgCallbackQuery) {
	if callback == nil {
		return
	}
	select {
	case telegramCallbackJobs <- telegramCallbackJob{Token: token, Callback: callback}:
	default:
		go telegramAnswerCallback(token, callback.ID, "")
		if callback.Message != nil && callback.Message.Chat != nil {
			go telegramSend(token, callback.Message.Chat.ID, "机器人正在处理其他请求，请稍后重试。")
		}
	}
}

func telegramGetUpdates(ctx context.Context, client *http.Client, token string, offset int64) ([]tgUpdate, error) {
	url := fmt.Sprintf("%s%s/getUpdates?timeout=%d&offset=%d", telegramAPIBase, token, telegramPollSecs, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var r tgUpdatesResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if !r.Ok {
		return nil, fmt.Errorf("telegram api: %s", r.Description)
	}
	return r.Result, nil
}

func telegramSend(token string, chatID int64, text string) {
	telegramSendWithKeyboard(token, chatID, text, nil)
}

func telegramSendWithKeyboard(token string, chatID int64, text string, keyboard *tgInlineKeyboardMarkup) {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	telegramPost(token, "sendMessage", payload)
}

func telegramSendMessage(token string, chatID int64, text string, keyboard *tgInlineKeyboardMarkup) (int64, bool) {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	body, err := telegramPostResult(token, "sendMessage", payload)
	if err != nil {
		return 0, false
	}
	var response telegramSendMessageResponse
	if err := json.Unmarshal(body, &response); err != nil || !response.Ok || response.Result.MessageID <= 0 {
		return 0, false
	}
	return response.Result.MessageID, true
}

func telegramEditWithKeyboard(token string, chatID, messageID int64, text string, keyboard *tgInlineKeyboardMarkup) {
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	} else {
		payload["reply_markup"] = &tgInlineKeyboardMarkup{InlineKeyboard: [][]tgInlineKeyboardButton{}}
	}
	telegramPost(token, "editMessageText", payload)
}

func telegramAnswerCallback(token, callbackID, text string) {
	payload := map[string]interface{}{"callback_query_id": callbackID}
	if text != "" {
		payload["text"] = text
	}
	telegramPost(token, "answerCallbackQuery", payload)
}

func telegramPost(token, method string, payload map[string]interface{}) {
	_, _ = telegramPostResult(token, method, payload)
}

func telegramPostResult(token, method string, payload map[string]interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Telegram] %s 请求编码失败: %v", method, err)
		return nil, err
	}
	url := fmt.Sprintf("%s%s/%s", telegramAPIBase, token, method)
	resp, err := telegramSendClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[Telegram] %s 失败: %v", method, err)
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Printf("[Telegram] %s 响应读取失败: %v", method, readErr)
		return responseBody, readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("[Telegram] %s 返回 HTTP %d: %s", method, resp.StatusCode, responseBody)
	}
	return responseBody, nil
}

// telegramConsumeResponse reads the response through EOF so net/http can put
// the connection back into the idle pool for the next Bot API request.
func telegramConsumeResponse(resp *http.Response) (string, error) {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		_, err := io.Copy(io.Discard, resp.Body)
		return "", err
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2048))
	_, drainErr := io.Copy(io.Discard, resp.Body)
	if readErr != nil {
		return strings.TrimSpace(string(body)), readErr
	}
	return strings.TrimSpace(string(body)), drainErr
}

func telegramUserByChat(chatID int64) (*model.User, bool) {
	var user model.User
	if err := DB.Where("telegram_chat_id = ?", chatID).First(&user).Error; err != nil {
		return nil, false
	}
	return &user, true
}

func handleTelegramCommand(token string, chatID int64, text string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	arg := ""
	if len(fields) > 1 {
		arg = fields[1]
	}

	switch cmd {
	case "/start":
		if user, ok := telegramUserByChat(chatID); ok {
			telegramShowMainMenu(token, chatID, 0, user)
		} else {
			telegramSend(token, chatID, telegramWelcomeText())
		}
		return
	case "/help":
		telegramSend(token, chatID, telegramHelpText())
		return
	case "/bind":
		telegramHandleBind(token, chatID, arg)
		return
	}

	user, ok := telegramUserByChat(chatID)
	if !ok {
		telegramSend(token, chatID, "尚未绑定账号。请在面板「个人中心」获取绑定码，然后发送：/bind <绑定码>")
		return
	}
	if cmd == "/cancel" {
		telegramCancelWizard(token, chatID, user)
		return
	}
	if !strings.HasPrefix(cmd, "/") && telegramHandleWizardText(token, chatID, user, text) {
		return
	}

	switch cmd {
	case "/unbind":
		UnbindTelegram(user.ID)
		telegramSend(token, chatID, "已解绑。")
	case "/menu":
		telegramShowMainMenu(token, chatID, 0, user)
	case "/login":
		telegramHandleLogin(token, chatID, user)
	case "/usage":
		telegramSend(token, chatID, telegramUsageText(user))
	case "/rules", "/forwards":
		telegramShowForwards(token, chatID, 0, user, 0)
	case "/new_forward":
		telegramStartForwardWizard(token, chatID, 0, user)
	case "/forward":
		telegramShowForwardFromCommand(token, chatID, user, arg)
	case "/tunnels":
		telegramShowTunnels(token, chatID, 0, user, 0)
	case "/new_tunnel":
		telegramStartTunnelWizard(token, chatID, 0, user)
	case "/tunnel":
		telegramShowTunnelFromCommand(token, chatID, user, arg)
	case "/nodes":
		telegramShowNodes(token, chatID, 0, user, 0)
	case "/node":
		telegramShowNodeFromCommand(token, chatID, user, arg)
	case "/audit":
		telegramShowAudits(token, chatID, 0, user, telegramAuditFilter{}, 0)
	case "/enable":
		telegramHandleForwardToggle(token, chatID, user, arg, true)
	case "/disable":
		telegramHandleForwardToggle(token, chatID, user, arg, false)
	case "/users":
		telegramHandleUsers(token, chatID, user)
	case "/reset":
		telegramHandleReset(token, chatID, user, arg)
	default:
		telegramSend(token, chatID, "未知命令。发送 /help 查看可用命令。")
	}
}

func telegramHandleBind(token string, chatID int64, code string) {
	if code == "" {
		telegramSend(token, chatID, "用法：/bind <绑定码>")
		return
	}
	userId, ok := consumeTelegramBindCode(code)
	if !ok {
		telegramSend(token, chatID, "绑定码无效或已过期，请在面板重新获取。")
		return
	}
	DB.Model(&model.User{}).Where("telegram_chat_id = ?", chatID).Update("telegram_chat_id", 0)
	if err := DB.Model(&model.User{}).Where("id = ?", userId).Update("telegram_chat_id", chatID).Error; err != nil {
		telegramSend(token, chatID, "绑定失败，请稍后重试。")
		return
	}
	var user model.User
	if err := DB.First(&user, userId).Error; err != nil {
		telegramSend(token, chatID, "绑定成功！发送 /menu 打开控制面板。")
		return
	}
	telegramShowMainMenu(token, chatID, 0, &user)
}

func telegramHandleLogin(token string, chatID int64, user *model.User) {
	jwt, err := pkg.GenerateToken(user)
	if err != nil {
		telegramSend(token, chatID, "生成登录链接失败。")
		return
	}
	base := strings.TrimRight(GetPanelAddress(""), "/")
	link := fmt.Sprintf("%s/login?tk=%s", base, jwt)
	telegramSend(token, chatID, "一次性登录链接（有效期 7 天，请勿转发）：\n"+link)
}

func telegramUsageText(user *model.User) string {
	used := user.InFlow + user.OutFlow
	var b strings.Builder
	fmt.Fprintf(&b, "账号：%s\n", user.User)
	if user.Flow == 0 {
		fmt.Fprintf(&b, "流量：%s / 不限\n", telegramFormatBytes(used))
	} else {
		limit := user.Flow * bytesToGB
		remain := limit - used
		if remain < 0 {
			remain = 0
		}
		fmt.Fprintf(&b, "流量：%s / %d GB\n", telegramFormatBytes(used), user.Flow)
		fmt.Fprintf(&b, "剩余：%s\n", telegramFormatBytes(remain))
	}
	if user.ExpTime == 0 {
		fmt.Fprintf(&b, "到期：永久\n")
	} else {
		fmt.Fprintf(&b, "到期：%s\n", time.UnixMilli(user.ExpTime).Format("2006-01-02 15:04"))
	}
	var ruleCount int64
	DB.Model(&model.Forward{}).Where("user_id = ?", user.ID).Count(&ruleCount)
	if user.Num == 0 {
		fmt.Fprintf(&b, "规则：%d / 不限", ruleCount)
	} else {
		fmt.Fprintf(&b, "规则：%d / %d", ruleCount, user.Num)
	}
	return b.String()
}

func telegramRulesText(user *model.User) string {
	var forwards []model.Forward
	DB.Where("user_id = ?", user.ID).Order("inx ASC, created_time DESC").Find(&forwards)
	if len(forwards) == 0 {
		return "暂无转发规则。"
	}
	var b strings.Builder
	b.WriteString("转发规则：\n")
	for _, f := range forwards {
		state := "运行中"
		if f.Status != 1 {
			state = "已暂停"
		}
		fmt.Fprintf(&b, "#%d %s [%s] 入%d → %s\n", f.ID, f.Name, state, f.InPort, f.RemoteAddr)
	}
	b.WriteString("\n用 /disable <ID> 暂停，/enable <ID> 恢复。")
	return b.String()
}

func telegramHandleForwardToggle(token string, chatID int64, user *model.User, arg string, enable bool) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		if enable {
			telegramSend(token, chatID, "用法：/enable <规则ID>")
		} else {
			telegramSend(token, chatID, "用法：/disable <规则ID>")
		}
		return
	}
	var res dto.R
	if enable {
		res = ResumeForward(id, user.ID, user.RoleId)
	} else {
		res = PauseForward(id, user.ID, user.RoleId)
	}
	telegramSend(token, chatID, res.Msg)
}

func telegramHandleUsers(token string, chatID int64, admin *model.User) {
	if admin.RoleId != adminRoleID {
		telegramSend(token, chatID, "无权限。")
		return
	}
	var users []model.User
	DB.Where("role_id != ?", adminRoleID).Order("id ASC").Limit(50).Find(&users)
	if len(users) == 0 {
		telegramSend(token, chatID, "暂无用户。")
		return
	}
	var b strings.Builder
	b.WriteString("用户概览（最多 50 条）：\n")
	for _, u := range users {
		used := u.InFlow + u.OutFlow
		quota := "不限"
		if u.Flow != 0 {
			quota = fmt.Sprintf("%d GB", u.Flow)
		}
		fmt.Fprintf(&b, "#%d %s %s / %s\n", u.ID, u.User, telegramFormatBytes(used), quota)
	}
	b.WriteString("\n用 /reset <用户ID> 重置其流量。")
	telegramSend(token, chatID, b.String())
}

func telegramHandleReset(token string, chatID int64, admin *model.User, arg string) {
	if admin.RoleId != adminRoleID {
		telegramSend(token, chatID, "无权限。")
		return
	}
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		telegramSend(token, chatID, "用法：/reset <用户ID>")
		return
	}
	res := ResetFlow(dto.ResetFlowDto{ID: id}, 1)
	telegramSend(token, chatID, res.Msg)
}

func telegramHelpText() string {
	return strings.Join([]string{
		"可用命令：",
		"/bind <绑定码> — 绑定面板账号（绑定码在面板「个人中心」获取）",
		"/unbind — 解绑当前账号",
		"/menu — 打开手机控制面板",
		"/login — 获取一次性登录链接",
		"/usage — 查看流量 / 到期 / 规则用量",
		"/tunnels — 隧道列表",
		"/new_tunnel — 新增隧道（管理员）",
		"/tunnel <ID> — 隧道详情",
		"/forwards — 转发列表（/rules 仍可用）",
		"/new_forward — 新增转发",
		"/forward <ID> — 转发详情",
		"/enable <规则ID> — 恢复指定转发",
		"/disable <规则ID> — 暂停指定转发",
		"/nodes — 节点监控",
		"/node <ID> — 节点详情",
		"/audit — 最近流量审计",
		"/cancel — 取消当前新增向导",
		"",
		"管理员命令：",
		"/users — 用户流量概览",
		"/reset <用户ID> — 重置该用户流量",
		"",
		"创建或编辑复杂配置：使用 /login 进入面板。",
	}, "\n")
}

func telegramWelcomeText() string {
	return strings.Join([]string{
		"Flux Panel 手机控制机器人",
		"",
		"尚未绑定账号。请在面板「个人中心」获取绑定码，然后发送：",
		"/bind <绑定码>",
		"",
		"发送 /help 查看全部命令。",
	}, "\n")
}

func telegramFormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
