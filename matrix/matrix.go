package matrix

import (
    "sync"
    "time"

    "maunium.net/go/mautrix"
    "maunium.net/go/mautrix/event"
    "maunium.net/go/mautrix/id"
)

type Client struct {
    *mautrix.Client
    AdminRoomID id.RoomID
    ready       chan struct{}
    once        sync.Once
}

type Config struct {
    ServerURL   string
    BotUser     string
    BotPassword string
    AdminRoomID string
    StorePath   string
}

func NewClient(cfg Config) (*Client, error) {
    client, err := mautrix.NewClient(cfg.ServerURL, "", "")
    if err != nil {
        return nil, err
    }

    mc := &Client{
        Client:      client,
        AdminRoomID: id.RoomID(cfg.AdminRoomID),
        ready:       make(chan struct{}),
    }

    // Логин
    resp, err := client.Login(&mautrix.ReqLogin{
        Type: mautrix.AuthTypePassword,
        Identifier: mautrix.UserIdentifier{
            Type: mautrix.IdentifierTypeUser,
            User: id.UserID(cfg.BotUser),
        },
        Password: cfg.BotPassword,
    })
    if err != nil {
        return nil, err
    }

    client.UserID = resp.UserID
    client.AccessToken = resp.AccessToken
    client.DeviceID = resp.DeviceID

    // Sync в фоне
    go mc.syncLoop()

    return mc, nil
}

func (mc *Client) syncLoop() {
    syncer := mc.Client.Syncer.(*mautrix.DefaultSyncer)
    
    syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
        // Игнорируем входящие сообщения
    })

    for {
        err := mc.Sync()
        if err != nil {
            time.Sleep(5 * time.Second)
        }
    }
}

func (mc *Client) WaitForReady() {
    <-mc.ready
}

func (mc *Client) MarkReady() {
    mc.once.Do(func() {
        close(mc.ready)
    })
}

func (mc *Client) SendAdminCommand(command string) error {
    _, err := mc.SendMessageEvent(
        mc.AdminRoomID,
        event.EventMessage,
        &event.MessageEventContent{
            MsgType: event.MsgText,
            Body:    command,
        },
    )
    return err
}

func (mc *Client) CheckUsernameAvailable(username string) (bool, error) {
    _, err := mc.RegisterAvailable(username)
    if err != nil {
        return false, err
    }
    return true, nil
}