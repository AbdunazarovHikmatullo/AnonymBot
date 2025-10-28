package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserGender represents the gender of the user
type UserGender string

const (
	Male   UserGender = "male"
	Female UserGender = "female"
)

// UserState holds the state for each user
type UserState struct {
	Gender   UserGender
	Partner  int64 // Chat partner ID, 0 if none
	Waiting  bool  // If waiting for a match
}

// Bot struct to hold bot state
type Bot struct {
	api           *tgbotapi.BotAPI
	users         map[int64]*UserState
	maleQueue     []int64
	femaleQueue   []int64
	mu            sync.Mutex
}

func NewBot(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:   api,
		users: make(map[int64]*UserState),
	}, nil
}

// Run starts the bot
func (b *Bot) Run() {
	// Set bot commands for menu
	b.setBotCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		userID := update.Message.From.ID
		text := update.Message.Text

		b.mu.Lock()
		state, exists := b.users[userID]
		if !exists {
			state = &UserState{}
			b.users[userID] = state
		}
		b.mu.Unlock()

		switch text {
		case "/start":
			b.handleStart(userID)
		case "/stop":
			b.stopChat(userID)
		case "/next":
			b.nextPartner(userID)
		default:
			b.forwardMessage(userID, text)
		}
	}
}

// setBotCommands registers commands in BotFather menu
func (b *Bot) setBotCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "/start", Description: "🔥 Начать анонимный чат"},
		{Command: "/stop", Description: "🛑 Завершить чат"},
		{Command: "/next", Description: "➡️ Найти нового собеседника"},
	}
	config := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(config)
	if err != nil {
		log.Printf("Failed to set commands: %v", err)
	}
}

// handleStart shows welcome message with inline buttons
func (b *Bot) handleStart(userID int64) {
	msg := tgbotapi.NewMessage(userID, "🌟 Добро пожаловать в *Таинственный чат*! Найди свою искру анонимно! 😎\nВыбери пол:")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👨 Мужской", "gender_male"),
			tgbotapi.NewInlineKeyboardButtonData("👩 Женский", "gender_female"),
		),
	)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// handleCallback processes inline button clicks
func (b *Bot) handleCallback(query *tgbotapi.CallbackQuery) {
	userID := query.From.ID
	data := query.Data

	b.mu.Lock()
	state, exists := b.users[userID]
	if !exists {
		state = &UserState{}
		b.users[userID] = state
	}
	b.mu.Unlock()

	switch data {
	case "gender_male":
		b.setGender(userID, Male)
	case "gender_female":
		b.setGender(userID, Female)
	case "start_chat":
		b.startChat(userID)
	}

	// Remove inline keyboard after click
	b.api.Request(tgbotapi.NewCallback(query.ID, ""))
	b.api.Request(tgbotapi.NewDeleteMessage(userID, query.Message.MessageID))
}

// setGender sets gender and shows start chat button
func (b *Bot) setGender(userID int64, gender UserGender) {
	b.mu.Lock()
	state := b.users[userID]
	state.Gender = gender
	b.mu.Unlock()

	msg := tgbotapi.NewMessage(userID, fmt.Sprintf("🎉 Пол выбран: *%s*! Готов начать анонимную магию? 💬", gender))
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔥 Начать чат", "start_chat"),
		),
	)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// startChat adds user to queue
func (b *Bot) startChat(userID int64) {
	b.mu.Lock()
	state := b.users[userID]
	if state.Gender == "" {
		b.mu.Unlock()
		b.sendMessage(userID, "Сначала выбери пол через /start.")
		return
	}
	if state.Partner != 0 {
		b.mu.Unlock()
		b.sendMessage(userID, "Ты уже в чате! Используй /stop или /next.")
		return
	}
	state.Waiting = true

	if state.Gender == Male {
		b.maleQueue = append(b.maleQueue, userID)
	} else {
		b.femaleQueue = append(b.femaleQueue, userID)
	}
	b.mu.Unlock()

	b.sendMessage(userID, "🔎 Ищем твою искру... Останься на связи! 😎")
	b.matchUsers()
}

// matchUsers pairs users
func (b *Bot) matchUsers() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for len(b.maleQueue) > 0 && len(b.femaleQueue) > 0 {
		maleID := b.maleQueue[0]
		femaleID := b.femaleQueue[0]

		b.maleQueue = b.maleQueue[1:]
		b.femaleQueue = b.femaleQueue[1:]

		maleState := b.users[maleID]
		femaleState := b.users[femaleID]

		maleState.Partner = femaleID
		femaleState.Partner = maleID

		maleState.Waiting = false
		femaleState.Waiting = false

		b.sendMessage(maleID, "✨ Партнёр найден! Пиши и наслаждайся анонимной магией! 💬\n(/stop — выйти, /next — новый чат)")
		b.sendMessage(femaleID, "✨ Партнёр найден! Пиши и наслаждайся анонимной магией! 💬\n(/stop — выйти, /next — новый чат)")
	}
}

// stopChat ends chat
func (b *Bot) stopChat(userID int64) {
	b.mu.Lock()
	state := b.users[userID]
	if state.Partner == 0 {
		b.mu.Unlock()
		b.sendMessage(userID, "Ты не в чате. Начни с /start!")
		return
	}

	partnerID := state.Partner
	partnerState := b.users[partnerID]

	state.Partner = 0
	partnerState.Partner = 0

	b.removeFromQueue(userID)
	b.removeFromQueue(partnerID)

	b.mu.Unlock()

	b.sendMessage(userID, "🛑 Чат завершён. Хочешь новую искру? Жми /start!")
	b.sendMessage(partnerID, "🛑 Партнёр завершил чат. Хочешь новый? Жми /start!")
}

// nextPartner stops and starts new chat
func (b *Bot) nextPartner(userID int64) {
	b.stopChat(userID)
	b.startChat(userID)
}

// forwardMessage sends message to partner
func (b *Bot) forwardMessage(userID int64, text string) {
	b.mu.Lock()
	state := b.users[userID]
	partnerID := state.Partner
	b.mu.Unlock()

	if partnerID != 0 {
		b.sendMessage(partnerID, text)
	} else {
		b.sendMessage(userID, "Ты не в чате. Жми /start или 'Начать чат'!")
	}
}

// removeFromQueue removes user from queues
func (b *Bot) removeFromQueue(userID int64) {
	for i, id := range b.maleQueue {
		if id == userID {
			b.maleQueue = append(b.maleQueue[:i], b.maleQueue[i+1:]...)
			return
		}
	}
	for i, id := range b.femaleQueue {
		if id == userID {
			b.femaleQueue = append(b.femaleQueue[:i], b.femaleQueue[i+1:]...)
			return
		}
	}
}

func (b *Bot) sendMessage(userID int64, text string) {
	msg := tgbotapi.NewMessage(userID, text)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable not set")
	}

	bot, err := NewBot(token)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Bot started")
	bot.Run()
}