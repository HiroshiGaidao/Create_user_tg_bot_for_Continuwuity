package bot

import (
    "crypto/rand"
    "fmt"
    "math/big"
    "os"
    "regexp"
    "strings"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "yourbot/database"
    "yourbot/messages"
)

// User states
const (
    StateNone            = ""
    StateWaitingUsername = "waiting_username"
    StateAdminSearching  = "admin_searching"
)

// Global state storage (in production, use Redis)
var userStates = make(map[int64]string)

// === Message Handlers ===

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
    if msg.IsCommand() {
        switch msg.Command() {
        case "start":
            b.cmdStart(msg)
        case "help":
            b.cmdHelp(msg)
        case "register":
            b.cmdRegister(msg)
        case "myaccounts":
            b.cmdMyAccounts(msg)
        case "resetpassword":
            b.cmdResetPassword(msg)
        case "admin":
            b.cmdAdmin(msg)
        }
        return
    }

    // Обработка текстовых сообщений (состояния)
    b.handleTextMessage(msg)
}

func (b *Bot) handleTextMessage(msg *tgbotapi.Message) {
    state := userStates[msg.From.ID]

    switch state {
    case StateWaitingUsername:
        b.handleWaitingUsername(msg)
    case StateAdminSearching:
        b.handleAdminSearching(msg)
    }
}

func (b *Bot) handleWaitingUsername(msg *tgbotapi.Message) {
    logger := fmt.Sprintf("📱 Логин от %d: %s", msg.From.ID, strings.TrimSpace(msg.Text))
    _ = logger // Используйте реальный logger

    // Проверка бана
    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        delete(userStates, msg.From.ID)
        b.sendMessage(msg.Chat.ID, messages.Validation("banned", ""), b.backKeyboard("menu_main"))
        return
    }

    // Санитизация и валидация
    rawUsername := sanitizeInput(msg.Text, 500)
    sanitizedUsername := sanitizeUsername(rawUsername)

    isValid, errorMsg := validateUsername(sanitizedUsername, sanitizedUsername)
    if !isValid {
        b.sendMessage(msg.Chat.ID, errorMsg, b.backKeyboard("menu_main"))
        return
    }

    // Проверка доступности
    b.sendMessage(msg.Chat.ID, messages.Register.Checking)

    available, err := b.matrix.CheckUsernameAvailable(sanitizedUsername)
    if err != nil || !available {
        b.sendMessage(msg.Chat.ID, messages.Register.Taken, b.backKeyboard("menu_main"))
        return
    }

    // Удаляем состояние
    delete(userStates, msg.From.ID)

    // Отправляем заявку админу
    markup := tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("confirm"), fmt.Sprintf("approve_%d_%s", msg.From.ID, sanitizedUsername)),
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("reject"), fmt.Sprintf("reject_%d_%s", msg.From.ID, sanitizedUsername)),
        ),
    )

    adminText := fmt.Sprintf(
        "🔔 **Новая заявка**\n\n"+
            "👤 Пользователь: `%s`\n"+
            "🆔 Telegram: `%d`\n"+
            "📝 Логин: `@%s:%s`",
        sanitizeInput(msg.From.FirstName, 100),
        msg.From.ID,
        sanitizedUsername,
        b.config.MatrixDomain,
    )

    sentMsg, err := b.tg.Send(tgbotapi.NewMessage(b.config.AdminChatID, adminText))
    if err != nil {
        b.sendMessage(msg.Chat.ID, "⚠️ Ошибка отправки заявки.", b.backKeyboard("menu_main"))
        return
    }

    // Сохраняем заявку (в памяти или БД)
    pendingRequests[sentMsg.MessageID] = PendingRequest{
        UserChatID: msg.From.ID,
        Username:   sanitizedUsername,
    }

    b.sendMessage(msg.Chat.ID, messages.Register.Sent, b.backKeyboard("menu_main"))
}

func (b *Bot) handleAdminSearching(msg *tgbotapi.Message) {
    if msg.From.ID != b.config.AdminChatID {
        return
    }

    delete(userStates, msg.From.ID)
    query := sanitizeInput(msg.Text, 100)

    users, err := b.db.SearchUsers(query)
    if err != nil || len(users) == 0 {
        b.sendMessage(msg.Chat.ID, messages.Admin.SearchEmpty)
        return
    }

    msgText := fmt.Sprintf(messages.Admin.SearchTitle, query) + "\n"
    var rows [][]tgbotapi.InlineKeyboardButton

    for _, user := range users {
        msgText += fmt.Sprintf("• `%d` — %s\n", user.TGChatID, user.FirstName)
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(
                fmt.Sprintf("👤 %d", user.TGChatID),
                fmt.Sprintf("admin_user_%d", user.TGChatID),
            ),
        ))
    }

    rows = append(rows, tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_main"),
    ))

    b.sendMessage(msg.Chat.ID, msgText, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// === Callback Handlers ===

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
    // Сразу отвечаем на callback чтобы не было timeout
    cb := tgbotapi.NewCallback(callback.ID, "")
    b.tg.Request(cb)

    switch {
    case callback.Data == "menu_main":
        b.cbMenuMain(callback)
    case callback.Data == "menu_help":
        b.cbMenuHelp(callback)
    case callback.Data == "menu_register":
        b.cbMenuRegister(callback)
    case callback.Data == "menu_accounts":
        b.cbMenuAccounts(callback)
    case callback.Data == "menu_resetpwd":
        b.cbMenuResetPwd(callback)
    case strings.HasPrefix(callback.Data, "resetpwd_"):
        b.cbResetPassword(callback)
    case strings.HasPrefix(callback.Data, "admin_"):
        b.cbAdmin(callback)
    case strings.HasPrefix(callback.Data, "approve_") || strings.HasPrefix(callback.Data, "reject_"):
        b.cbRegistration(callback)
    }
}

func (b *Bot) cbMenuMain(callback *tgbotapi.CallbackQuery) {
    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        messages.MainMenuTitle(), b.mainMenuKeyboard())
}

func (b *Bot) cbMenuHelp(callback *tgbotapi.CallbackQuery) {
    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        messages.HelpText(b.config.MaxAccounts), b.backKeyboard("menu_main"))
}

func (b *Bot) cbMenuRegister(callback *tgbotapi.CallbackQuery) {
    banned, _ := b.db.IsUserBanned(callback.From.ID)
    if banned {
        b.answerCallback(callback.ID, messages.Validation("banned", ""), true)
        return
    }

    accounts, _ := b.db.GetUserAccounts(callback.From.ID)
    if len(accounts) >= b.config.MaxAccounts {
        b.answerCallback(callback.ID, fmt.Sprintf(messages.Register.LimitReached, b.config.MaxAccounts), true)
        return
    }

    userStates[callback.From.ID] = StateWaitingUsername
    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        messages.RegisterPrompt(), b.backKeyboard("menu_main"))
}

func (b *Bot) cbMenuAccounts(callback *tgbotapi.CallbackQuery) {
    accounts, _ := b.db.GetUserAccounts(callback.From.ID)

    if len(accounts) == 0 {
        b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
            messages.Accounts.Empty, b.backKeyboard("menu_main"))
        return
    }

    msgText := messages.Accounts.Title
    for _, acc := range accounts {
        msgText += fmt.Sprintf(messages.Accounts.AccountItem, acc.MatrixFullID)
    }

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        msgText, b.accountsKeyboard(accounts))
}

func (b *Bot) cbMenuResetPwd(callback *tgbotapi.CallbackQuery) {
    accounts, _ := b.db.GetUserAccounts(callback.From.ID)

    if len(accounts) == 0 {
        b.answerCallback(callback.ID, messages.Password.NoAccounts, true)
        return
    }

    if len(accounts) == 1 {
        b.answerCallback(callback.ID, messages.Password.Resetting, false)
        b.resetPassword(callback.From.ID, accounts[0].MatrixUsername)
        return
    }

    msgText := messages.Password.Select
    for _, acc := range accounts {
        msgText += fmt.Sprintf(messages.Accounts.AccountItem, acc.MatrixFullID)
    }

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        msgText, b.accountsKeyboard(accounts))
}

func (b *Bot) cbResetPassword(callback *tgbotapi.CallbackQuery) {
    if callback.From.ID != callback.Message.Chat.ID {
        b.answerCallback(callback.ID, messages.Errors.NotYourRequest, true)
        return
    }

    parts := strings.Split(callback.Data, "_")
    if len(parts) < 2 {
        return
    }
    username := parts[1]

    b.answerCallback(callback.ID, messages.Password.Resetting, false)
    b.resetPassword(callback.From.ID, username)
}

func (b *Bot) cbAdmin(callback *tgbotapi.CallbackQuery) {
    if callback.Message.Chat.ID != b.config.AdminChatID {
        b.answerCallback(callback.ID, messages.Admin.NoRights, true)
        return
    }

    data := callback.Data
    if !strings.HasPrefix(data, "admin_") {
        return
    }

    action := strings.TrimPrefix(data, "admin_")

    switch {
    case action == "users":
        b.adminUsers(callback)
    case action == "stats":
        b.adminStats(callback)
    case action == "search":
        b.answerCallback(callback.ID, "", false)
        userStates[callback.Message.Chat.ID] = StateAdminSearching
        b.sendMessage(callback.Message.Chat.ID, messages.Admin.SearchPrompt)
    case action == "logs":
        b.adminLogs(callback)
    case action == "download_logs":
        b.answerCallback(callback.ID, "📤 Отправляю логи...", false)
        b.sendLogFile(callback.Message.Chat.ID)
    case action == "main":
        b.adminMain(callback)
    case strings.HasPrefix(action, "user_"):
        b.adminUser(callback)
    case strings.HasPrefix(action, "ban_") || strings.HasPrefix(action, "unban_"):
        b.adminBanUnban(callback)
    case strings.HasPrefix(action, "delete_"):
        b.adminDelete(callback)
    }
}

func (b *Bot) cbRegistration(callback *tgbotapi.CallbackQuery) {
    if callback.Message.Chat.ID != b.config.AdminChatID {
        b.answerCallback(callback.ID, messages.Admin.NoRights, true)
        return
    }

    parts := strings.Split(callback.Data, "_")
    if len(parts) < 3 {
        return
    }

    action := parts[0]
    var userChatID int64
    fmt.Sscanf(parts[1], "%d", &userChatID)
    username := parts[2]

    if action == "approve" {
        b.answerCallback(callback.ID, messages.Admin.Processing, false)
        b.approveRegistration(callback, userChatID, username)
    } else {
        b.answerCallback(callback.ID, messages.Admin.Processing, false)
        b.rejectRegistration(callback, userChatID, username)
    }
}

// === Command Handlers ===

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
    username := sanitizeInput(msg.From.UserName, 100)
    firstName := sanitizeInput(msg.From.FirstName, 100)

    b.db.AddUser(msg.From.ID, username, firstName)

    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        b.sendMessage(msg.Chat.ID, messages.Validation("banned", ""))
        return
    }

    b.sendMessage(msg.Chat.ID, messages.MainMenuTitle(), b.mainMenuKeyboard())
}

func (b *Bot) cmdHelp(msg *tgbotapi.Message) {
    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        return
    }
    b.sendMessage(msg.Chat.ID, messages.HelpText(b.config.MaxAccounts), b.backKeyboard("menu_main"))
}

func (b *Bot) cmdRegister(msg *tgbotapi.Message) {
    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        b.sendMessage(msg.Chat.ID, messages.Validation("banned", ""))
        return
    }

    accounts, _ := b.db.GetUserAccounts(msg.From.ID)
    if len(accounts) >= b.config.MaxAccounts {
        b.sendMessage(msg.Chat.ID, fmt.Sprintf(messages.Register.LimitReached, b.config.MaxAccounts))
        return
    }

    userStates[msg.From.ID] = StateWaitingUsername
    b.sendMessage(msg.Chat.ID, messages.RegisterPrompt(), b.backKeyboard("menu_main"))
}

func (b *Bot) cmdMyAccounts(msg *tgbotapi.Message) {
    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        b.sendMessage(msg.Chat.ID, messages.Validation("banned", ""))
        return
    }

    accounts, _ := b.db.GetUserAccounts(msg.From.ID)
    if len(accounts) == 0 {
        b.sendMessage(msg.Chat.ID, messages.Accounts.Empty, b.backKeyboard("menu_main"))
        return
    }

    msgText := messages.Accounts.Title
    for _, acc := range accounts {
        msgText += fmt.Sprintf(messages.Accounts.AccountItem, acc.MatrixFullID)
    }

    b.sendMessage(msg.Chat.ID, msgText, b.accountsKeyboard(accounts))
}

func (b *Bot) cmdResetPassword(msg *tgbotapi.Message) {
    banned, _ := b.db.IsUserBanned(msg.From.ID)
    if banned {
        b.sendMessage(msg.Chat.ID, messages.Validation("banned", ""))
        return
    }

    accounts, _ := b.db.GetUserAccounts(msg.From.ID)
    if len(accounts) == 0 {
        b.sendMessage(msg.Chat.ID, messages.Password.NoAccounts, b.backKeyboard("menu_main"))
        return
    }

    if len(accounts) == 1 {
        b.resetPassword(msg.From.ID, accounts[0].MatrixUsername)
        return
    }

    msgText := messages.Password.Select
    for _, acc := range accounts {
        msgText += fmt.Sprintf(messages.Accounts.AccountItem, acc.MatrixFullID)
    }

    b.sendMessage(msg.Chat.ID, msgText, b.accountsKeyboard(accounts))
}

func (b *Bot) cmdAdmin(msg *tgbotapi.Message) {
    if msg.From.ID != b.config.AdminChatID {
        return
    }

    stats, _ := b.db.GetStats()
    msgText := messages.Admin("title", map[string]interface{}{
        "total":    stats.Total,
        "active":   stats.Active,
        "banned":   stats.Banned,
        "accounts": stats.Accounts,
    })

    b.sendMessage(msg.Chat.ID, msgText, b.adminKeyboard())
}

// === Admin Handlers ===

func (b *Bot) adminUsers(callback *tgbotapi.CallbackQuery) {
    users, _ := b.db.GetAllUsers(10, 0)

    msgText := messages.Admin.UsersTitle
    var rows [][]tgbotapi.InlineKeyboardButton

    for _, user := range users {
        emoji := "🟢"
        if user.Status == "banned" {
            emoji = "🔴"
        }
        msgText += fmt.Sprintf("%s `%d` — %s (%d аккаунтов)\n", emoji, user.TGChatID, user.FirstName, user.RegistrationCount)
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(
                fmt.Sprintf("⚙️ %d", user.TGChatID),
                fmt.Sprintf("admin_user_%d", user.TGChatID),
            ),
        ))
    }

    rows = append(rows, tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_main"),
    ))

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        msgText, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) adminStats(callback *tgbotapi.CallbackQuery) {
    stats, _ := b.db.GetStats()

    msgText := fmt.Sprintf(
        "📊 **Статистика:**\n\n"+
            "• Всего: %d\n"+
            "• Активных: %d\n"+
            "• Забаненных: %d\n"+
            "• Аккаунтов: %d\n"+
            "• Лимит на пользователя: %d",
        stats.Total, stats.Active, stats.Banned, stats.Accounts, b.config.MaxAccounts,
    )

    markup := tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_main"),
        ),
    )

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID, msgText, markup)
}

func (b *Bot) adminLogs(callback *tgbotapi.CallbackQuery) {
    logs, _ := b.db.GetLogs(7, 50)

    msgText := messages.Admin.LogsTitle
    for _, log := range logs {
        emoji := "🟢"
        if log.Action == "ban" || log.Action == "delete" {
            emoji = "🔴"
        } else if log.Action == "register" || log.Action == "unban" {
            emoji = "🟢"
        } else {
            emoji = "🔵"
        }
        msgText += fmt.Sprintf("%s `%s` — %s: %s\n", emoji, log.Timestamp.Format("2006-01-02 15:04"), log.Action, log.Details)
    }

    markup := tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_main"),
        ),
    )

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID, msgText, markup)
}

func (b *Bot) adminMain(callback *tgbotapi.CallbackQuery) {
    stats, _ := b.db.GetStats()
    msgText := messages.Admin("title", map[string]interface{}{
        "total":    stats.Total,
        "active":   stats.Active,
        "banned":   stats.Banned,
        "accounts": stats.Accounts,
    })

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID, msgText, b.adminKeyboard())
}

func (b *Bot) adminUser(callback *tgbotapi.CallbackQuery) {
    parts := strings.Split(callback.Data, "_")
    if len(parts) < 3 {
        return
    }

    var chatID int64
    fmt.Sscanf(parts[2], "%d", &chatID)

    user, err := b.db.GetUser(chatID)
    if err != nil || user == nil {
        b.answerCallback(callback.ID, messages.Errors.UserNotFound, true)
        return
    }

    accounts, _ := b.db.GetUserAccounts(chatID)

    msgText := fmt.Sprintf(
        "👤 **Информация о пользователе**\n\n"+
            "🆔 Telegram: `%d`\n"+
            "👤 Имя: %s\n"+
            "📝 Username: @%s\n"+
            "📅 Регистрация: %s\n"+
            "🔄 Активность: %s\n"+
            "📊 Аккаунтов: %d\n"+
            "🚫 Статус: `%s`\n\n",
        user.TGChatID, user.FirstName, user.TGUsername,
        user.RegisteredAt.Format("2006-01-02"), user.LastActivity.Format("2006-01-02"),
        len(accounts), user.Status,
    )

    if len(accounts) > 0 {
        msgText += "📋 **Аккаунты Matrix:**\n"
        for _, acc := range accounts {
            msgText += fmt.Sprintf("• `%s`\n", acc.MatrixFullID)
        }
    } else {
        msgText += "📋 **Аккаунты Matrix:** нет\n"
    }

    var rows [][]tgbotapi.InlineKeyboardButton

    // Кнопки удаления аккаунтов
    for _, acc := range accounts {
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(
                messages.ButtonDelete(acc.MatrixUsername),
                fmt.Sprintf("admin_delete_%d_%s", chatID, acc.MatrixUsername),
            ),
        ))
    }

    // Кнопка бана/разбана
    if user.Status == "active" {
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("ban"),
                fmt.Sprintf("admin_ban_%d", chatID)),
        ))
    } else {
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("unban"),
                fmt.Sprintf("admin_unban_%d", chatID)),
        ))
    }

    rows = append(rows, tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_users"),
    ))

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        msgText, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) adminBanUnban(callback *tgbotapi.CallbackQuery) {
    parts := strings.Split(callback.Data, "_")
    action := parts[1]

    var chatID int64
    fmt.Sscanf(parts[2], "%d", &chatID)

    if action == "ban" {
        b.db.BanUser(chatID, "Забанен через админ-панель")
        b.sendMessage(chatID, messages.Notifications.AccessBlocked)
    } else {
        b.db.UnbanUser(chatID)
        b.sendMessage(chatID, messages.Notifications.AccessRestored)
    }

    b.db.LogAction(b.config.AdminChatID, "admin_"+action, fmt.Sprintf("User %d", chatID))

    user, _ := b.db.GetUser(chatID)
    actionText := "Забанен"
    if action == "unban" {
        actionText = "Разбанен"
    }
    msgText := fmt.Sprintf("✅ %s: `%s`", actionText, user.FirstName)

    markup := tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_users"),
        ),
    )

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID, msgText, markup)
}

func (b *Bot) adminDelete(callback *tgbotapi.CallbackQuery) {
    parts := strings.Split(callback.Data, "_")
    if len(parts) < 4 {
        return
    }

    var chatID int64
    fmt.Sscanf(parts[2], "%d", &chatID)
    username := parts[3]
    fullID := fmt.Sprintf("@%s:%s", username, b.config.MatrixDomain)

    b.answerCallback(callback.ID, messages.Admin.Deleting, false)

    command := fmt.Sprintf("!admin users deactivate %s", fullID)
    err := b.matrix.SendAdminCommand(command)

    if err != nil {
        b.answerCallback(callback.ID, messages.Admin.Error, true)
        return
    }

    b.db.HardDeleteMatrixAccount(chatID, username)

    // Уведомить пользователя
    b.sendMessage(chatID, fmt.Sprintf(messages.Notifications.AccountDeleted, fullID))

    // Обновить карточку
    user, _ := b.db.GetUser(chatID)
    accounts, _ := b.db.GetUserAccounts(chatID)

    msgText := fmt.Sprintf("✅ **Удалён:** `%s`\n\n📊 Осталось: %d\n\n", username, len(accounts))

    if len(accounts) > 0 {
        msgText += "📋 **Оставшиеся аккаунты:**\n"
        for _, acc := range accounts {
            msgText += fmt.Sprintf("• `%s`\n", acc.MatrixFullID)
        }
    } else {
        msgText += "📋 **Аккаунты:** нет\n"
    }

    var rows [][]tgbotapi.InlineKeyboardButton
    for _, acc := range accounts {
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(
                messages.ButtonDelete(acc.MatrixUsername),
                fmt.Sprintf("admin_delete_%d_%s", chatID, acc.MatrixUsername),
            ),
        ))
    }
    rows = append(rows, tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "admin_users"),
    ))

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        msgText, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// === Registration Handlers ===

func (b *Bot) approveRegistration(callback *tgbotapi.CallbackQuery, userChatID int64, username string) {
    password := generateSecurePassword(16)
    fullID := fmt.Sprintf("@%s:%s", username, b.config.MatrixDomain)
    command := fmt.Sprintf("!admin users create %s %s", fullID, password)

    err := b.matrix.SendAdminCommand(command)
    if err != nil {
        b.answerCallback(callback.ID, messages.Admin.Error, true)
        return
    }

    b.db.AddMatrixAccount(userChatID, username, fullID)

    serverInfo := messages.ServerInfo(b.config.MatrixDomain)
    msgText := messages.RegisterApproved(username, password, serverInfo, b.config.TelegraphURL)

    markup := tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("home"), "menu_main"),
        ),
    )
    b.sendMessage(userChatID, msgText, markup)

    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        fmt.Sprintf("✅ `%s` создан!", username), tgbotapi.NewInlineKeyboardMarkup())
}

func (b *Bot) rejectRegistration(callback *tgbotapi.CallbackQuery, userChatID int64, username string) {
    b.sendMessage(userChatID, messages.Register.Rejected)
    b.editMessage(callback.Message.Chat.ID, callback.Message.MessageID,
        fmt.Sprintf("❌ `%s` отклонена.", username), tgbotapi.NewInlineKeyboardMarkup())
}

// === Password Reset ===

func (b *Bot) resetPassword(chatID int64, username string) {
    password := generateSecurePassword(16)
    fullID := fmt.Sprintf("@%s:%s", username, b.config.MatrixDomain)
    command := fmt.Sprintf("!admin users password %s %s", fullID, password)

    err := b.matrix.SendAdminCommand(command)
    if err != nil {
        b.sendMessage(chatID, messages.Password.Error)
        return
    }

    b.db.LogAction(chatID, "password_reset", username)

    msgText := messages.Format(messages.Password.Success, map[string]interface{}{
        "login":    fullID,
        "password": password,
    })
    b.sendMessage(chatID, msgText)
}

// === Helper Functions ===

func (b *Bot) sendMessage(chatID int64, text string, markup ...tgbotapi.InlineKeyboardMarkup) {
    msg := tgbotapi.NewMessage(chatID, text)
    msg.ParseMode = "Markdown"
    if len(markup) > 0 {
        msg.ReplyMarkup = markup[0]
    }
    b.tg.Send(msg)
}

func (b *Bot) editMessage(chatID int64, messageID int, text string, markup tgbotapi.InlineKeyboardMarkup) {
    msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
    msg.ParseMode = "Markdown"
    msg.ReplyMarkup = &markup
    b.tg.Send(msg)
}

func (b *Bot) answerCallback(callbackID string, text string, showAlert bool) {
    cb := tgbotapi.NewCallback(callbackID, text)
    cb.ShowAlert = showAlert
    b.tg.Request(cb)
}

func (b *Bot) mainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
    return tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("registration"), "menu_register"),
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("my_accounts"), "menu_accounts"),
        ),
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("reset_password"), "menu_resetpwd"),
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("help"), "menu_help"),
        ),
    )
}

func (b *Bot) backKeyboard(callback string) tgbotapi.InlineKeyboardMarkup {
    return tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), callback),
        ),
    )
}

func (b *Bot) accountsKeyboard(accounts []database.MatrixAccount) tgbotapi.InlineKeyboardMarkup {
    var rows [][]tgbotapi.InlineKeyboardButton
    for _, acc := range accounts {
        rows = append(rows, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData(
                fmt.Sprintf("🔑 %s", acc.MatrixUsername),
                fmt.Sprintf("resetpwd_%s", acc.MatrixUsername),
            ),
        ))
    }
    rows = append(rows, tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(messages.Button("back"), "menu_main"),
    ))
    return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) adminKeyboard() tgbotapi.InlineKeyboardMarkup {
    return tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("👥 Пользователи", "admin_users"),
            tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "admin_stats"),
        ),
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("🔍 Поиск", "admin_search"),
            tgbotapi.NewInlineKeyboardButtonData("📜 Логи", "admin_logs"),
        ),
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("💾 Скачать логи", "admin_download_logs"),
        ),
    )
}

func (b *Bot) sendLogFile(chatID int64) {
    logPath := "/app/logs/bot.log" // Или из config

    file, err := os.Open(logPath)
    if err != nil {
        b.sendMessage(chatID, "❌ Файл логов не найден")
        return
    }
    defer file.Close()

    doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(logPath))
    doc.Caption = "📄 Логи бота"
    b.tg.Send(doc)
}

// === Validation Functions ===

func sanitizeUsername(username string) string {
    username = strings.TrimSpace(strings.ToLower(username))
    re := regexp.MustCompile(`[^a-z0-9]`)
    username = re.ReplaceAllString(username, "")
    if len(username) > 20 {
        username = username[:20]
    }
    return username
}

func validateUsername(username, originalInput string) (bool, string) {
    if username == "" {
        return false, messages.Validation("empty", "")
    }
    if len(username) < 3 {
        return false, messages.Validation("too_short", "")
    }
    if len(username) > 20 {
        return false, messages.Validation("too_long", "")
    }
    if !regexp.MustCompile(`^[a-z][a-z0-9]{2,19}$`).MatchString(username) {
        return false, messages.Validation("invalid_format", "")
    }

    forbidden := []string{"admin", "root", "system", "matrix", "bot", "support", "help", "moderator"}
    for _, f := range forbidden {
        if username == f {
            return false, messages.Validation("reserved", originalInput)
        }
    }

    return true, ""
}

func sanitizeInput(text string, maxLength int) string {
    if text == "" {
        return ""
    }
    // Remove control characters except \n
    re := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
    text = re.ReplaceAllString(text, "")
    text = strings.TrimSpace(text)
    if len(text) > maxLength {
        text = text[:maxLength]
    }
    return text
}

// === Password Generation ===

func generateSecurePassword(length int) string {
    const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
    result := make([]byte, length)
    for i := range result {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
        result[i] = chars[n.Int64()]
    }
    return string(result)
}

// === Pending Requests Storage ===

type PendingRequest struct {
    UserChatID int64
    Username   string
}

var pendingRequests = make(map[int]PendingRequest)