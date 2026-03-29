package bot

import (
    "log"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "yourbot/database"
    "yourbot/matrix"
    "yourbot/messages"
)

type Bot struct {
    tg     *tgbotapi.BotAPI
    db     *database.DB
    matrix *matrix.Client
    config *Config
    updates tgbotapi.UpdatesChannel
}

type Config struct {
    AdminChatID      int64
    MaxAccounts      int
    TelegraphURL     string
    MatrixDomain     string
}

func New(tg *tgbotapi.BotAPI, db *database.DB, mc *matrix.Client, cfg *Config) *Bot {
    return &Bot{
        tg:     tg,
        db:     db,
        matrix: mc,
        config: cfg,
    }
}

func (b *Bot) Start() {
    u := tgbotapi.NewUpdate(0)
    u.Timeout = 60
    b.updates = b.tg.GetUpdatesChan(u)

    log.Println("📱 Telegram bot polling started...")
    go b.handleUpdates()
}

func (b *Bot) Stop() {
    b.tg.StopReceivingUpdates()
}

func (b *Bot) handleUpdates() {
    for update := range b.updates {
        if update.Message != nil {
            b.handleMessage(update.Message)
        }
        if update.CallbackQuery != nil {
            b.handleCallback(update.CallbackQuery)
        }
    }
}