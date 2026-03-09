package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gogobee/internal/util"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// CommandDef describes a bot command for help/discovery.
type CommandDef struct {
	Name        string
	Description string
	Usage       string
	Category    string
	AdminOnly   bool
}

// MessageContext holds the context for a message event.
type MessageContext struct {
	RoomID    id.RoomID
	EventID   id.EventID
	Sender    id.UserID
	Body      string
	IsCommand bool // true if the message starts with the command prefix
	Event     *event.Event
}

// ReactionContext holds the context for a reaction event.
type ReactionContext struct {
	RoomID      id.RoomID
	EventID     id.EventID
	Sender      id.UserID
	TargetEvent id.EventID
	Emoji       string
	Event       *event.Event
}

// Plugin is the interface all plugins must implement.
type Plugin interface {
	Name() string
	Commands() []CommandDef
	OnMessage(ctx MessageContext) error
	OnReaction(ctx ReactionContext) error
	Init() error
}

// Base provides common helpers for plugin implementations.
type Base struct {
	Client *mautrix.Client
	Prefix string
}

// NewBase creates a Base with default prefix "!".
func NewBase(client *mautrix.Client) Base {
	return Base{Client: client, Prefix: "!"}
}

// IsCommand checks if body matches prefix+command.
func (b *Base) IsCommand(body, command string) bool {
	return util.IsCommand(body, b.Prefix, command)
}

// GetArgs returns the argument string after the command.
func (b *Base) GetArgs(body, command string) string {
	return util.GetArgs(body, b.Prefix, command)
}

// IsAdmin checks if a user ID is in the admin list.
func (b *Base) IsAdmin(userID id.UserID) bool {
	admins := os.Getenv("ADMIN_USERS")
	if admins == "" {
		return false
	}
	for _, a := range strings.Split(admins, ",") {
		if strings.TrimSpace(a) == string(userID) {
			return true
		}
	}
	return false
}

// SendMessage sends a plain text message to a room.
func (b *Base) SendMessage(roomID id.RoomID, text string) error {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send message", "room", roomID, "err", err)
	}
	return err
}

// SendMessageID sends a plain text message and returns the event ID.
func (b *Base) SendMessageID(roomID id.RoomID, text string) (id.EventID, error) {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	}
	resp, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send message", "room", roomID, "err", err)
		return "", err
	}
	return resp.EventID, nil
}

// SendThread sends a message in a thread rooted at threadID.
func (b *Base) SendThread(roomID id.RoomID, threadID id.EventID, text string) error {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
		RelatesTo: &event.RelatesTo{
			Type:    event.RelThread,
			EventID: threadID,
			InReplyTo: &event.InReplyTo{
				EventID: threadID,
			},
			IsFallingBack: true,
		},
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send thread message", "room", roomID, "err", err)
	}
	return err
}

// SendNotice sends an m.notice message to a room.
func (b *Base) SendNotice(roomID id.RoomID, text string) error {
	content := &event.MessageEventContent{
		MsgType: event.MsgNotice,
		Body:    text,
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	return err
}

// SendReply sends a reply to a specific event.
func (b *Base) SendReply(roomID id.RoomID, eventID id.EventID, text string) error {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
		RelatesTo: &event.RelatesTo{
			InReplyTo: &event.InReplyTo{
				EventID: eventID,
			},
		},
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send reply", "room", roomID, "err", err)
	}
	return err
}

// SendHTML sends an HTML-formatted message.
func (b *Base) SendHTML(roomID id.RoomID, plain, html string) error {
	content := &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          plain,
		Format:        event.FormatHTML,
		FormattedBody: html,
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	return err
}

// SendReact sends a reaction emoji to an event.
func (b *Base) SendReact(roomID id.RoomID, eventID id.EventID, emoji string) error {
	content := &event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			Type:    event.RelAnnotation,
			EventID: eventID,
			Key:     emoji,
		},
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventReaction, content)
	return err
}

// SendDM sends a direct message to a user. Creates a DM room if needed.
func (b *Base) SendDM(userID id.UserID, text string) error {
	resp, err := b.Client.CreateRoom(context.Background(), &mautrix.ReqCreateRoom{
		Preset:   "trusted_private_chat",
		Invite:   []id.UserID{userID},
		IsDirect: true,
		InitialState: []*event.Event{
			{
				Type: event.StateEncryption,
				Content: event.Content{
					Parsed: &event.EncryptionEventContent{
						Algorithm: id.AlgorithmMegolmV1,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create DM room: %w", err)
	}
	return b.SendMessage(resp.RoomID, text)
}

// UploadContent uploads data to the Matrix content repository and returns the MXC URI.
func (b *Base) UploadContent(data []byte, contentType, filename string) (id.ContentURI, error) {
	resp, err := b.Client.UploadBytesWithName(context.Background(), data, contentType, filename)
	if err != nil {
		return id.ContentURI{}, err
	}
	return resp.ContentURI, nil
}

// SendImage uploads image data and sends it as an m.image message with a caption.
func (b *Base) SendImage(roomID id.RoomID, imgData []byte, filename, caption string, width, height int) error {
	uri, err := b.UploadContent(imgData, "image/png", filename)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}
	content := &event.MessageEventContent{
		MsgType: event.MsgImage,
		Body:    caption,
		URL:     uri.CUString(),
		Info: &event.FileInfo{
			MimeType: "image/png",
			Size:     len(imgData),
			Width:    width,
			Height:   height,
		},
	}
	_, err = b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	return err
}
