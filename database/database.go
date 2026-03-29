package database

import (
    "database/sql"
    "sync"
    "time"

    _ "modernc.org/sqlite"
)

type DB struct {
    *sql.DB
    writeMu sync.Mutex
}

type User struct {
    TGChatID         int64
    TGUsername       string
    FirstName        string
    RegisteredAt     time.Time
    LastActivity     time.Time
    Status           string
    RegistrationCount int
}

type MatrixAccount struct {
    ID            int
    TGChatID      int64
    MatrixUsername string
    MatrixFullID   string
    CreatedAt     time.Time
    Status        string
}

type ActionLog struct {
    ID        int
    TGChatID  int64
    Action    string
    Details   string
    Timestamp time.Time
}

func New(path string) (*DB, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, err
    }

    // Настройки пула
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    db.SetConnMaxLifetime(0)

    // WAL режим и настройки
    _, err = db.Exec(`
        PRAGMA journal_mode=WAL;
        PRAGMA busy_timeout=5000;
        PRAGMA synchronous=NORMAL;
    `)
    if err != nil {
        return nil, err
    }

    // Создание таблиц
    if err := initSchema(db); err != nil {
        return nil, err
    }

    return &DB{DB: db}, nil
}

func initSchema(db *sql.DB) error {
    queries := []string{
        `CREATE TABLE IF NOT EXISTS users (
            tg_chat_id INTEGER PRIMARY KEY,
            tg_username TEXT,
            first_name TEXT,
            registered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            last_activity DATETIME DEFAULT CURRENT_TIMESTAMP,
            status TEXT DEFAULT 'active',
            registration_count INTEGER DEFAULT 0
        )`,
        `CREATE TABLE IF NOT EXISTS matrix_accounts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            tg_chat_id INTEGER,
            matrix_username TEXT,
            matrix_full_id TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            status TEXT DEFAULT 'active',
            FOREIGN KEY (tg_chat_id) REFERENCES users(tg_chat_id) ON DELETE CASCADE
        )`,
        `CREATE TABLE IF NOT EXISTS action_logs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            tg_chat_id INTEGER,
            action TEXT,
            details TEXT,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
        )`,
        `CREATE INDEX IF NOT EXISTS idx_users_status ON users(status)`,
        `CREATE INDEX IF NOT EXISTS idx_accounts_tg ON matrix_accounts(tg_chat_id)`,
        `CREATE INDEX IF NOT EXISTS idx_accounts_matrix ON matrix_accounts(matrix_username)`,
        `CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON action_logs(timestamp)`,
    }

    for _, q := range queries {
        if _, err := db.Exec(q); err != nil {
            return err
        }
    }
    return nil
}

// Чтение (без блокировки)
func (db *DB) GetUser(chatID int64) (*User, error) {
    row := db.QueryRow(`
        SELECT tg_chat_id, tg_username, first_name, registered_at, 
               last_activity, status, registration_count 
        FROM users WHERE tg_chat_id = ?
    `, chatID)

    var u User
    err := row.Scan(&u.TGChatID, &u.TGUsername, &u.FirstName, 
                    &u.RegisteredAt, &u.LastActivity, &u.Status, &u.RegistrationCount)
    if err != nil {
        return nil, err
    }
    return &u, nil
}

// Запись (с блокировкой)
func (db *DB) AddUser(chatID int64, username, firstName string) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()

    _, err := db.Exec(`
        INSERT INTO users (tg_chat_id, tg_username, first_name, last_activity)
        VALUES (?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT (tg_chat_id) DO UPDATE SET
            tg_username = excluded.tg_username,
            first_name = excluded.first_name,
            last_activity = CURRENT_TIMESTAMP
    `, chatID, username, firstName)
    return err
}

func (db *DB) IsUserBanned(chatID int64) (bool, error) {
    user, err := db.GetUser(chatID)
    if err != nil {
        return false, err
    }
    return user.Status == "banned", nil
}

func (db *DB) BanUser(chatID int64, reason string) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()

    _, err := db.Exec("UPDATE users SET status = 'banned' WHERE tg_chat_id = ?", chatID)
    if err != nil {
        return err
    }
    return db.logActionInternal(chatID, "ban", reason)
}

func (db *DB) UnbanUser(chatID int64) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()

    _, err := db.Exec("UPDATE users SET status = 'active' WHERE tg_chat_id = ?", chatID)
    if err != nil {
        return err
    }
    return db.logActionInternal(chatID, "unban", "")
}

func (db *DB) GetUserAccounts(chatID int64) ([]MatrixAccount, error) {
    rows, err := db.Query(`
        SELECT id, tg_chat_id, matrix_username, matrix_full_id, created_at, status 
        FROM matrix_accounts 
        WHERE tg_chat_id = ? AND status = 'active'
    `, chatID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var accounts []MatrixAccount
    for rows.Next() {
        var a MatrixAccount
        if err := rows.Scan(&a.ID, &a.TGChatID, &a.MatrixUsername, 
                          &a.MatrixFullID, &a.CreatedAt, &a.Status); err != nil {
            return nil, err
        }
        accounts = append(accounts, a)
    }
    return accounts, rows.Err()
}

func (db *DB) AddMatrixAccount(chatID int64, username, fullID string) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()

    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    _, err = tx.Exec(`
        INSERT INTO matrix_accounts (tg_chat_id, matrix_username, matrix_full_id)
        VALUES (?, ?, ?)
    `, chatID, username, fullID)
    if err != nil {
        return err
    }

    _, err = tx.Exec(`
        UPDATE users SET registration_count = registration_count + 1
        WHERE tg_chat_id = ?
    `, chatID)
    if err != nil {
        return err
    }

    if err := db.logActionInternalTx(tx, chatID, "register", "Created "+fullID); err != nil {
        return err
    }

    return tx.Commit()
}

func (db *DB) HardDeleteMatrixAccount(chatID int64, username string) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()

    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    _, err = tx.Exec(`
        DELETE FROM matrix_accounts
        WHERE tg_chat_id = ? AND matrix_username = ?
    `, chatID, username)
    if err != nil {
        return err
    }

    _, err = tx.Exec(`
        UPDATE users SET registration_count = registration_count - 1
        WHERE tg_chat_id = ? AND registration_count > 0
    `, chatID)
    if err != nil {
        return err
    }

    if err := db.logActionInternalTx(tx, chatID, "hard_delete", "Removed "+username); err != nil {
        return err
    }

    return tx.Commit()
}

func (db *DB) GetAllUsers(limit, offset int) ([]User, error) {
    rows, err := db.Query(`
        SELECT tg_chat_id, tg_username, first_name, registered_at, 
               last_activity, status, registration_count 
        FROM users ORDER BY registered_at DESC LIMIT ? OFFSET ?
    `, limit, offset)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var users []User
    for rows.Next() {
        var u User
        if err := rows.Scan(&u.TGChatID, &u.TGUsername, &u.FirstName, 
                          &u.RegisteredAt, &u.LastActivity, &u.Status, &u.RegistrationCount); err != nil {
            return nil, err
        }
        users = append(users, u)
    }
    return users, rows.Err()
}

type Stats struct {
    Total    int
    Active   int
    Banned   int
    Accounts int
}

func (db *DB) GetStats() (*Stats, error) {
    stats := &Stats{}

    row := db.QueryRow(`
        SELECT 
            COUNT(*) as total,
            COUNT(*) FILTER (WHERE status = 'active') as active,
            COUNT(*) FILTER (WHERE status = 'banned') as banned
        FROM users
    `)
    if err := row.Scan(&stats.Total, &stats.Active, &stats.Banned); err != nil {
        return nil, err
    }

    row = db.QueryRow("SELECT COUNT(*) FROM matrix_accounts WHERE status = 'active'")
    if err := row.Scan(&stats.Accounts); err != nil {
        return nil, err
    }

    return stats, nil
}

func (db *DB) logActionInternal(chatID int64, action, details string) error {
    _, err := db.Exec(`
        INSERT INTO action_logs (tg_chat_id, action, details)
        VALUES (?, ?, ?)
    `, chatID, action, details)
    return err
}

func (db *DB) logActionInternalTx(tx *sql.Tx, chatID int64, action, details string) error {
    _, err := tx.Exec(`
        INSERT INTO action_logs (tg_chat_id, action, details)
        VALUES (?, ?, ?)
    `, chatID, action, details)
    return err
}

func (db *DB) LogAction(chatID int64, action, details string) error {
    db.writeMu.Lock()
    defer db.writeMu.Unlock()
    return db.logActionInternal(chatID, action, details)
}

func (db *DB) GetLogs(days, limit int) ([]ActionLog, error) {
    rows, err := db.Query(`
        SELECT id, tg_chat_id, action, details, timestamp 
        FROM action_logs 
        WHERE timestamp >= datetime('now', '-' || ? || ' days')
        ORDER BY timestamp DESC
        LIMIT ?
    `, days, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var logs []ActionLog
    for rows.Next() {
        var l ActionLog
        if err := rows.Scan(&l.ID, &l.TGChatID, &l.Action, &l.Details, &l.Timestamp); err != nil {
            return nil, err
        }
        logs = append(logs, l)
    }
    return logs, rows.Err()
}

func (db *DB) SearchUsers(query string) ([]User, error) {
    searchPattern := "%" + query + "%"
    rows, err := db.Query(`
        SELECT tg_chat_id, tg_username, first_name, registered_at, 
               last_activity, status, registration_count 
        FROM users 
        WHERE tg_username LIKE ? OR first_name LIKE ?
        LIMIT 20
    `, searchPattern, searchPattern)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var users []User
    for rows.Next() {
        var u User
        if err := rows.Scan(&u.TGChatID, &u.TGUsername, &u.FirstName, 
                          &u.RegisteredAt, &u.LastActivity, &u.Status, &u.RegistrationCount); err != nil {
            return nil, err
        }
        users = append(users, u)
    }
    return users, rows.Err()
}

func (db *DB) Close() error {
    return db.DB.Close()
}