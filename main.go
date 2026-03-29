package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "yourbot/bot"
    "yourbot/config"
    "yourbot/database"
    "yourbot/matrix"
    "yourbot/messages"
)

func main() {
    log.Println("=" + strings.Repeat("=", 49))
    log.Println("🤖 Запуск Matrix Registration Bot (Go)")
    log.Println("=" + strings.Repeat("=", 49))

    // Загрузка конфигурации
    cfg := config.Load()

    // Загрузка сообщений
    if err := messages.Load("messages/messages.yaml"); err != nil {
        log.Fatalf("❌ Ошибка загрузки сообщений: %v", err)
    }
    log.Println("✅ Сообщения загружены")

    // База данных
    db, err := database.New(cfg.Database.Path)
    if err != nil {
        log.Fatalf("❌ Ошибка БД: %v", err)
    }
    defer db.Close()
    log.Println("✅ База данных подключена")

    // Matrix клиент
    mc, err := matrix.NewClient(matrix.Config{
        ServerURL:   cfg.Matrix.ServerURL,
        BotUser:     cfg.Matrix.BotUser,
        BotPassword: cfg.Matrix.BotPassword,
        AdminRoomID: cfg.Matrix.AdminRoomID,
        StorePath:   cfg.Matrix.StorePath,
    })
    if err != nil {
        log.Fatalf("❌ Ошибка Matrix: %v", err)
    }
    log.Println("✅ Matrix клиент подключен")

    // Telegram бот
    tgbot, err := tgbotapi.NewBotAPI(cfg.Telegram.BotToken)
    if err != nil {
        log.Fatalf("❌ Ошибка Telegram: %v", err)
    }
    log.Println("✅ Telegram бот подключен")

    // Создание бота
    b := bot.New(tgbot, db, mc, &bot.Config{
        AdminChatID:  cfg.Telegram.AdminChatID,
        MaxAccounts:  cfg.Limits.MaxAccountsPerUser,
        TelegraphURL: cfg.Guide.TelegraphURL,
        MatrixDomain: cfg.Matrix.Domain,
    })

    // Запуск
    b.Start()

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    log.Println("🛑 Shutdown...")
    b.Stop()
    db.Close()
}