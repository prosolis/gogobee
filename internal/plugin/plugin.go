package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gogobee/internal/db"
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

// RoomMembers returns the set of user IDs in a room. Used to scope leaderboards
// to only show users present in the room where the command was issued.
func (b *Base) RoomMembers(roomID id.RoomID) map[id.UserID]bool {
	resp, err := b.Client.JoinedMembers(context.Background(), roomID)
	if err != nil {
		slog.Error("failed to get room members", "room", roomID, "err", err)
		return nil
	}
	members := make(map[id.UserID]bool, len(resp.Joined))
	for uid := range resp.Joined {
		members[uid] = true
	}
	return members
}

// ResolveUser resolves a partial username or display name to a full Matrix user ID.
// Accepts any of: "@user:server", "user:server", "user", display name, or partial match.
// Uses room membership to match against display names when a localpart match isn't found.
// Returns the matched user ID and true, or empty and false if no unique match.
func (b *Base) ResolveUser(input string, roomIDs ...id.RoomID) (id.UserID, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}

	// If it already looks like a full Matrix ID, use it directly
	if strings.Contains(input, ":") {
		if !strings.HasPrefix(input, "@") {
			input = "@" + input
		}
		return id.UserID(input), true
	}

	// Strip leading @ for the search
	query := strings.TrimPrefix(input, "@")
	queryLower := strings.ToLower(query)

	d := db.Get()
	rows, err := d.Query(`SELECT user_id FROM user_stats`)
	if err != nil {
		return "", false
	}
	defer rows.Close()

	var exact []string
	var partial []string

	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}

		// Extract localpart: @localpart:server -> localpart
		localpart := uid
		if strings.HasPrefix(localpart, "@") {
			localpart = localpart[1:]
		}
		if idx := strings.Index(localpart, ":"); idx > 0 {
			localpart = localpart[:idx]
		}

		lower := strings.ToLower(localpart)

		if lower == queryLower {
			exact = append(exact, uid)
		} else if strings.Contains(lower, queryLower) {
			partial = append(partial, uid)
		}
	}

	// Exact localpart match — return it (even if multiple servers, prefer first)
	if len(exact) >= 1 {
		return id.UserID(exact[0]), true
	}

	// Single partial localpart match
	if len(partial) == 1 {
		return id.UserID(partial[0]), true
	}

	// Fall back to display name matching via room membership
	if len(roomIDs) > 0 {
		resp, err := b.Client.JoinedMembers(context.Background(), roomIDs[0])
		if err == nil {
			var dnExact []id.UserID
			var dnPartial []id.UserID
			for uid, member := range resp.Joined {
				if member.DisplayName == "" {
					continue
				}
				dn := strings.ToLower(member.DisplayName)
				if dn == queryLower {
					dnExact = append(dnExact, uid)
				} else if strings.Contains(dn, queryLower) {
					dnPartial = append(dnPartial, uid)
				}
			}
			if len(dnExact) >= 1 {
				return dnExact[0], true
			}
			if len(dnPartial) == 1 {
				return dnPartial[0], true
			}
		}
	}

	return "", false
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
		MsgType:  event.MsgImage,
		Body:     caption,
		FileName: filename,
		URL:      uri.CUString(),
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
