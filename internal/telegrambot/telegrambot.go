package telegrambot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	tgapp "github.com/go-sphere/telegram-bot/telegram"
	telebot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tbxark/air780e-sms-forwarder/internal/config"
	"github.com/tbxark/air780e-sms-forwarder/internal/sms"
)

const (
	routeMenu = "menu"
	routeAct  = "act"
	maxText   = 3900
)

type Executor interface {
	ExecuteAT(context.Context, string) ([]string, error)
}

type Service struct {
	app      *tgapp.Bot
	chatID   int64
	executor Executor
}

type callbackData struct {
	ID string `json:"id"`
}

type action struct {
	title    string
	commands []string
	parent   string
}

func New(cfg config.Config, executor Executor) (*Service, error) {
	if strings.TrimSpace(cfg.TelegramToken) == "" {
		return nil, errors.New("telegram_token is required")
	}
	chatID, err := ParseChatID(cfg.TelegramChat)
	if err != nil {
		return nil, err
	}
	if executor == nil {
		return nil, errors.New("telegram AT executor is required")
	}

	s := &Service{chatID: chatID, executor: executor}
	app, err := tgapp.NewApp(tgapp.Config{Token: cfg.TelegramToken}, tgapp.WithErrorHandler(s.handleError))
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	s.app = app
	s.bindRoutes()
	return s, nil
}

func ParseChatID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("telegram_chat is required")
	}
	chatID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telegram_chat must be an int64: %w", err)
	}
	return chatID, nil
}

func (s *Service) Start(ctx context.Context) error {
	return s.app.Start(ctx)
}

func (s *Service) Initialize(ctx context.Context) error {
	if err := s.registerCommands(ctx); err != nil {
		return err
	}
	return s.SendDefaultMenu(ctx)
}

func (s *Service) Close(ctx context.Context) error {
	return s.app.Close(ctx)
}

func (s *Service) SendSMS(ctx context.Context, event sms.Event) error {
	return s.sendMessage(ctx, s.chatID, formatSMSMessage(event))
}

func (s *Service) SendRaw(ctx context.Context, line string) error {
	return s.pushText(ctx, "Air780E raw: "+line)
}

func (s *Service) SendDefaultMenu(ctx context.Context) error {
	return s.sendMessage(ctx, s.chatID, mainMenuMessage())
}

func (s *Service) pushText(ctx context.Context, text string) error {
	return s.sendMessage(ctx, s.chatID, &tgapp.Message{Text: text})
}

func (s *Service) sendMessage(ctx context.Context, chatID int64, msg *tgapp.Message) error {
	_, err := s.app.API().SendMessage(ctx, sendMessageParams(chatID, msg))
	return err
}

func (s *Service) registerCommands(ctx context.Context) error {
	_, err := s.app.API().SetMyCommands(ctx, &telebot.SetMyCommandsParams{Commands: defaultBotCommands()})
	return err
}

func (s *Service) bindRoutes() {
	s.app.BindCommand("start", s.showMainMenu)
	s.app.BindCommand("menu", s.showMainMenu)
	s.app.BindNoRoute(s.showMainMenu)
	s.app.BindCallback(routeMenu, s.handleMenu)
	s.app.BindCallback(routeAct, s.handleAction)
}

func (s *Service) showMainMenu(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	return s.app.SendMessage(ctx, update, mainMenuMessage())
}

func (s *Service) handleMenu(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	data, err := callbackPayload(update)
	if err != nil {
		return err
	}
	var msg *tgapp.Message
	switch data.ID {
	case "main":
		msg = mainMenuMessage()
	case "status":
		msg = statusMenuMessage()
	case "sms":
		msg = smsMenuMessage()
	case "device":
		msg = deviceMenuMessage()
	case "help":
		msg = helpMessage()
	case "reset_confirm":
		msg = resetConfirmMessage()
	default:
		msg = mainMenuMessage()
	}
	return s.app.SendMessage(ctx, update, msg)
}

func (s *Service) handleAction(ctx context.Context, update *tgapp.Update) error {
	if !s.authorized(update) {
		return s.rejectUnauthorized(ctx, update)
	}
	data, err := callbackPayload(update)
	if err != nil {
		return err
	}
	act, ok := actionForID(data.ID)
	if !ok {
		return s.app.SendMessage(ctx, update, &tgapp.Message{Text: "Unknown action", Button: backToMainKeyboard()})
	}
	results := s.runCommands(ctx, act.commands)
	return s.app.SendMessage(ctx, update, &tgapp.Message{
		Text:      formatCommandResult(act.title, results),
		ParseMode: models.ParseModeHTML,
		Button:    actionResultKeyboard(act.parent),
	})
}

func (s *Service) runCommands(ctx context.Context, commands []string) []commandResult {
	results := make([]commandResult, 0, len(commands))
	for _, cmd := range commands {
		lines, err := s.executor.ExecuteAT(ctx, cmd)
		results = append(results, commandResult{Command: cmd, Lines: lines, Err: err})
		if err != nil || ctx.Err() != nil {
			break
		}
	}
	return results
}

func (s *Service) authorized(update *tgapp.Update) bool {
	return AuthorizedChat(update, s.chatID)
}

func AuthorizedChat(update *tgapp.Update, chatID int64) bool {
	if update == nil {
		return false
	}
	if update.Message != nil {
		return update.Message.Chat.ID == chatID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		return update.CallbackQuery.Message.Message.Chat.ID == chatID
	}
	return false
}

func (s *Service) rejectUnauthorized(ctx context.Context, update *tgapp.Update) error {
	if update == nil {
		return nil
	}
	msg := &tgapp.Message{Text: "unauthorized chat"}
	if update.Message != nil || (update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil) {
		return s.app.SendMessage(ctx, update, msg)
	}
	return nil
}

func (s *Service) handleError(ctx context.Context, b *telebot.Bot, update *tgapp.Update, err error) {
	if err == nil {
		return
	}
	slog.Error("telegram handler failed", "err", err)
	if update == nil {
		return
	}
	tgapp.SendErrorMessage(ctx, b, update, err)
}

func callbackPayload(update *tgapp.Update) (*callbackData, error) {
	if update == nil || update.CallbackQuery == nil {
		return nil, errors.New("missing callback query")
	}
	_, data, err := tgapp.UnmarshalData[callbackData](update.CallbackQuery.Data)
	return data, err
}

func mainMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Air780E Console\nChoose an action.",
		Button: oneColumnKeyboard(
			menuButton("Status", "status"),
			menuButton("SMS History", "sms"),
			menuButton("Device Control", "device"),
			menuButton("Help", "help"),
		),
	}
}

func defaultBotCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: "Open the default control menu"},
		{Command: "menu", Description: "Open the control menu"},
	}
}

func sendMessageParams(chatID int64, msg *tgapp.Message) *telebot.SendMessageParams {
	if msg == nil {
		msg = &tgapp.Message{}
	}
	params := &telebot.SendMessageParams{
		ChatID:    chatID,
		Text:      truncateText(msg.Text),
		ParseMode: msg.ParseMode,
	}
	if len(msg.Button) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: msg.Button}
	}
	return params
}

func statusMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Status",
		Button: oneColumnKeyboard(
			actionButton("Status Summary", "status_summary"),
			actionButton("Signal Quality", "signal"),
			actionButton("Network Registration", "registration"),
			actionButton("Operator", "operator"),
			actionButton("SIM Status", "sim"),
			actionButton("Module Info", "module"),
			menuButton("Back", "main"),
		),
	}
}

func smsMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "SMS History",
		Button: oneColumnKeyboard(
			actionButton("Unread SMS", "sms_unread"),
			actionButton("All SMS", "sms_all"),
			actionButton("SMS Storage", "sms_storage"),
			menuButton("Back", "main"),
		),
	}
}

func deviceMenuMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Device Control",
		Button: oneColumnKeyboard(
			actionButton("Current Function Mode", "function_mode"),
			actionButton("Re-enable SMS Push", "enable_sms_push"),
			menuButton("Restart Module", "reset_confirm"),
			menuButton("Back", "main"),
		),
	}
}

func resetConfirmMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Restart the Air780E module? The serial connection may drop during reboot.",
		Button: oneColumnKeyboard(
			actionButton("Confirm Restart", "reset"),
			menuButton("Cancel / Back", "device"),
		),
	}
}

func helpMessage() *tgapp.Message {
	return &tgapp.Message{
		Text: "Send /menu to open the control menu. New SMS messages are pushed to the authorized chat automatically. When telegram_raw=true, raw serial lines are pushed too.",
		Button: oneColumnKeyboard(
			menuButton("Back", "main"),
		),
	}
}

func backToMainKeyboard() [][]models.InlineKeyboardButton {
	return oneColumnKeyboard(menuButton("Back to Main Menu", "main"))
}

func actionResultKeyboard(parent string) [][]models.InlineKeyboardButton {
	if parent == "" || parent == "main" {
		return backToMainKeyboard()
	}
	return oneColumnKeyboard(
		menuButton("Back", parent),
		menuButton("Main Menu", "main"),
	)
}

func oneColumnKeyboard(buttons ...models.InlineKeyboardButton) [][]models.InlineKeyboardButton {
	rows := make([][]models.InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		rows = append(rows, []models.InlineKeyboardButton{button})
	}
	return rows
}

func menuButton(text, id string) models.InlineKeyboardButton {
	return tgapp.NewButton(text, routeMenu, callbackData{ID: id})
}

func actionButton(text, id string) models.InlineKeyboardButton {
	return tgapp.NewButton(text, routeAct, callbackData{ID: id})
}

func actionForID(id string) (action, bool) {
	actions := map[string]action{
		"status_summary":  {title: "Status Summary", parent: "status", commands: []string{"+CPIN?", "+CSQ", "+CREG?", "+CEREG?", "+COPS?", "+CFUN?"}},
		"signal":          {title: "Signal Quality", parent: "status", commands: []string{"+CSQ"}},
		"registration":    {title: "Network Registration", parent: "status", commands: []string{"+CREG?", "+CEREG?"}},
		"operator":        {title: "Operator", parent: "status", commands: []string{"+COPS?"}},
		"sim":             {title: "SIM Status", parent: "status", commands: []string{"+CPIN?", "+CCID"}},
		"module":          {title: "Module Info", parent: "status", commands: []string{"+CGMI", "+CGMM", "+CGMR", "+CGSN"}},
		"sms_unread":      {title: "Unread SMS", parent: "sms", commands: []string{"+CMGF=1", "+CMGL=\"REC UNREAD\""}},
		"sms_all":         {title: "All SMS", parent: "sms", commands: []string{"+CMGF=1", "+CMGL=\"ALL\""}},
		"sms_storage":     {title: "SMS Storage", parent: "sms", commands: []string{"+CPMS?"}},
		"function_mode":   {title: "Current Function Mode", parent: "device", commands: []string{"+CFUN?"}},
		"enable_sms_push": {title: "Re-enable SMS Push", parent: "device", commands: []string{"+CMGF=1", "+CNMI=2,2,0,0,0"}},
		"reset":           {title: "Restart Module", parent: "device", commands: []string{"+RESET"}},
	}
	act, ok := actions[id]
	return act, ok
}

func truncateText(text string) string {
	runes := []rune(text)
	if len(runes) <= maxText {
		return text
	}
	suffix := "\n[truncated]"
	limit := maxText - len([]rune(suffix))
	if limit < 0 {
		limit = 0
	}
	return string(runes[:limit]) + suffix
}

type MutexExecutor struct {
	mu sync.Mutex
	fn func(context.Context, string) ([]string, error)
}

func NewMutexExecutor(fn func(context.Context, string) ([]string, error)) *MutexExecutor {
	return &MutexExecutor{fn: fn}
}

func (e *MutexExecutor) ExecuteAT(ctx context.Context, cmd string) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e.fn == nil {
		return nil, errors.New("telegram AT executor function is required")
	}
	return e.fn(ctx, cmd)
}
