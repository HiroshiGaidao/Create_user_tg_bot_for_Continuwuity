package config

import (
    "os"
    "strconv"
)

type Config struct {
    Telegram struct {
        BotToken   string
        AdminChatID int64
    }
    Matrix struct {
        ServerURL     string
        Domain        string
        BotUser       string
        BotPassword   string
        AdminRoomID   string
        StorePath     string
    }
    Database struct {
        Path string
    }
    Logging struct {
        Path         string
        Level        string
        MaxBytes     int
        BackupCount  int
    }
    Limits struct {
        MaxAccountsPerUser int
    }
    Guide struct {
        TelegraphURL string
    }
}

func Load() *Config {
    cfg := &Config{}

    // Telegram
    cfg.Telegram.BotToken = getEnv("TG_BOT_TOKEN", "")
    cfg.Telegram.AdminChatID = getEnvInt64("ADMIN_CHAT_ID", 0)

    // Matrix
    cfg.Matrix.ServerURL = getEnv("MATRIX_SERVER_URL", "https://matrix.yourdomain.com")
    cfg.Matrix.Domain = getEnv("MATRIX_DOMAIN", "yourdomain.com")
    cfg.Matrix.BotUser = getEnv("MATRIX_BOT_USER", "regbot")
    cfg.Matrix.BotPassword = getEnv("MATRIX_BOT_PASSWORD", "")
    cfg.Matrix.AdminRoomID = getEnv("MATRIX_ADMIN_ROOM_ID", "")
    cfg.Matrix.StorePath = getEnv("MATRIX_STORE_PATH", "/app/matrix_store")

    // Database
    cfg.Database.Path = getEnv("DB_PATH", "/app/data/bot_database.db")

    // Logging
    cfg.Logging.Path = getEnv("LOG_PATH", "/app/logs")
    cfg.Logging.Level = getEnv("LOG_LEVEL", "INFO")
    cfg.Logging.MaxBytes = getEnvInt("LOG_MAX_BYTES", 10485760)
    cfg.Logging.BackupCount = getEnvInt("LOG_BACKUP_COUNT", 7)

    // Limits
    cfg.Limits.MaxAccountsPerUser = getEnvInt("MAX_ACCOUNTS_PER_USER", 3)

    // Guide
    cfg.Guide.TelegraphURL = getEnv("TELEGRAPH_GUIDE", "https://telegra.ph/Matrix-Guide-01-01")

    return cfg
}

func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
    if val := os.Getenv(key); val != "" {
        if i, err := strconv.Atoi(val); err == nil {
            return i
        }
    }
    return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
    if val := os.Getenv(key); val != "" {
        if i, err := strconv.ParseInt(val, 10, 64); err == nil {
            return i
        }
    }
    return defaultVal
}