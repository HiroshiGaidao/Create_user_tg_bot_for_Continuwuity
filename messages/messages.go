package messages

import (
    "fmt"
    "os"
    "strings"

    "gopkg.in/yaml.v3"
)

// Messages содержит все текстовые сообщения
type Messages struct {
    MainMenu     MainMenu     `yaml:"main_menu"`
    Help         Help         `yaml:"help"`
    Register     Register     `yaml:"register"`
    Accounts     Accounts     `yaml:"accounts"`
    Password     Password     `yaml:"password"`
    Validation   Validation   `yaml:"validation"`
    Buttons      Buttons      `yaml:"buttons"`
    Admin        Admin        `yaml:"admin"`
    Server       Server       `yaml:"server"`
    Errors       Errors       `yaml:"errors"`
    Notifications Notifications `yaml:"notifications"`
}

type MainMenu struct {
    Title string `yaml:"title"`
}

type Help struct {
    Title string `yaml:"title"`
}

type Register struct {
    Prompt       string `yaml:"prompt"`
    Checking     string `yaml:"checking"`
    Available    string `yaml:"available"`
    Taken        string `yaml:"taken"`
    Sent         string `yaml:"sent"`
    Approved     string `yaml:"approved"`
    Rejected     string `yaml:"rejected"`
    LimitReached string `yaml:"limit_reached"`
}

type Accounts struct {
    Empty       string `yaml:"empty"`
    Title       string `yaml:"title"`
    AccountItem string `yaml:"account_item"`
}

type Password struct {
    NoAccounts string `yaml:"no_accounts"`
    Select     string `yaml:"select"`
    Resetting  string `yaml:"resetting"`
    Success    string `yaml:"success"`
    Error      string `yaml:"error"`
}

type Validation struct {
    Empty        string `yaml:"empty"`
    TooShort     string `yaml:"too_short"`
    TooLong      string `yaml:"too_long"`
    InvalidFormat string `yaml:"invalid_format"`
    Reserved     string `yaml:"reserved"`
    Banned       string `yaml:"banned"`
}

type Buttons struct {
    Registration  string `yaml:"registration"`
    MyAccounts    string `yaml:"my_accounts"`
    ResetPassword string `yaml:"reset_password"`
    Help          string `yaml:"help"`
    Back          string `yaml:"back"`
    Confirm       string `yaml:"confirm"`
    Reject        string `yaml:"reject"`
    Delete        string `yaml:"delete"`
    Ban           string `yaml:"ban"`
    Unban         string `yaml:"unban"`
    Home          string `yaml:"home"`
}

type Admin struct {
    Title        string `yaml:"title"`
    UsersTitle   string `yaml:"users_title"`
    StatsTitle   string `yaml:"stats_title"`
    SearchPrompt string `yaml:"search_prompt"`
    SearchEmpty  string `yaml:"search_empty"`
    SearchTitle  string `yaml:"search_title"`
    LogsTitle    string `yaml:"logs_title"`
    UserTitle    string `yaml:"user_title"`
    UserAccounts string `yaml:"user_accounts"`
    Deleted      string `yaml:"deleted"`
    Banned       string `yaml:"banned"`
    Unbanned     string `yaml:"unbanned"`
    NoRights     string `yaml:"no_rights"`
    Processing   string `yaml:"processing"`
    Deleting     string `yaml:"deleting"`
    Error        string `yaml:"error"`
}

type Server struct {
    Info string `yaml:"server.info"`
}

type Errors struct {
    CallbackExpired string `yaml:"callback_expired"`
    NotYourRequest  string `yaml:"not_your_request"`
    UserNotFound    string `yaml:"user_not_found"`
    DatabaseError   string `yaml:"database_error"`
    MatrixError     string `yaml:"matrix_error"`
    Unknown         string `yaml:"unknown"`
}

type Notifications struct {
    AccountDeleted string `yaml:"account_deleted"`
    AccessBlocked  string `yaml:"access_blocked"`
    AccessRestored string `yaml:"access_restored"`
}

// Global messages instance
var Msg *Messages

// Load загружает сообщения из YAML файла
func Load(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("failed to read messages file: %w", err)
    }

    Msg = &Messages{}
    if err := yaml.Unmarshal(data, Msg); err != nil {
        return fmt.Errorf("failed to parse messages YAML: %w", err)
    }

    return nil
}

// Format заменяет плейсхолдеры в строке
func Format(template string, values map[string]interface{}) string {
    result := template
    for key, value := range values {
        placeholder := "{" + key + "}"
        result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
    }
    return result
}

// Helper функции для удобного доступа

func MainMenuTitle() string {
    return Msg.MainMenu.Title
}

func HelpText(maxAccounts int) string {
    return Format(Msg.Help.Title, map[string]interface{}{
        "max_accounts": maxAccounts,
    })
}

func RegisterPrompt() string {
    return Msg.Register.Prompt
}

func RegisterApproved(login, password, serverInfo, guideURL string) string {
    return Format(Msg.Register.Approved, map[string]interface{}{
        "login":      login,
        "password":   password,
        "server_info": serverInfo,
        "guide_url":  guideURL,
    })
}

func ServerInfo(domain string) string {
    return Format(Msg.Server.Info, map[string]interface{}{
        "domain": domain,
    })
}

func Button(key string) string {
    switch key {
    case "registration":
        return Msg.Buttons.Registration
    case "my_accounts":
        return Msg.Buttons.MyAccounts
    case "reset_password":
        return Msg.Buttons.ResetPassword
    case "help":
        return Msg.Buttons.Help
    case "back":
        return Msg.Buttons.Back
    case "confirm":
        return Msg.Buttons.Confirm
    case "reject":
        return Msg.Buttons.Reject
    case "delete":
        return Msg.Buttons.Delete
    case "ban":
        return Msg.Buttons.Ban
    case "unban":
        return Msg.Buttons.Unban
    case "home":
        return Msg.Buttons.Home
    default:
        return key
    }
}

func ButtonDelete(username string) string {
    return Format(Msg.Buttons.Delete, map[string]interface{}{
        "username": username,
    })
}

func Validation(key string, login string) string {
    switch key {
    case "empty":
        return Msg.Validation.Empty
    case "too_short":
        return Msg.Validation.TooShort
    case "too_long":
        return Msg.Validation.TooLong
    case "invalid_format":
        return Msg.Validation.InvalidFormat
    case "reserved":
        return Format(Msg.Validation.Reserved, map[string]interface{}{
            "login": login,
        })
    case "banned":
        return Msg.Validation.Banned
    default:
        return Msg.Validation.InvalidFormat
    }
}

func Admin(key string, values map[string]interface{}) string {
    var template string
    switch key {
    case "title":
        template = Msg.Admin.Title
    case "stats":
        template = Msg.Admin.StatsTitle
    case "deleted":
        template = Msg.Admin.Deleted
    case "banned":
        template = Msg.Admin.Banned
    case "unbanned":
        template = Msg.Admin.Unbanned
    default:
        return key
    }
    return Format(template, values)
}