package plugin

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

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

// dmCache maps user IDs to their DM room IDs to avoid creating duplicate rooms.
var (
	dmCache   = make(map[id.UserID]id.RoomID)
	dmCacheMu sync.Mutex
)

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

// DisplayName returns the Matrix display name for a user, falling back
// to the localpart extracted from the user ID (e.g., "@alice:server" -> "alice").
func (b *Base) DisplayName(userID id.UserID) string {
	resp, err := b.Client.GetDisplayName(context.Background(), userID)
	if err != nil || resp.DisplayName == "" {
		s := string(userID)
		if idx := strings.Index(s, ":"); idx > 0 {
			s = s[1:idx]
		}
		return s
	}
	return resp.DisplayName
}

// RoomMembers returns the set of user IDs visible from a room. If space groups
// are enabled, this returns the union of all members across rooms in the same
// space group. Otherwise falls back to the single room's membership.
func (b *Base) RoomMembers(roomID id.RoomID) map[id.UserID]bool {
	if spaceGroupMgr != nil {
		if members := spaceGroupMgr.GetGroupMembers(roomID); members != nil {
			return members
		}
	}
	// Fallback: direct API call
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

// ---- Space Group Manager ----

var spaceGroupMgr *SpaceGroupManager

// SpaceGroupManager automatically groups rooms with overlapping membership
// so that leaderboards and other scoped queries show the full community.
type SpaceGroupManager struct {
	mu           sync.RWMutex
	client       *mautrix.Client
	threshold    int // overlap percentage (0-100)
	roomToGroup  map[id.RoomID]int
	groupMembers map[int]map[id.UserID]bool
}

// InitSpaceGroups creates and initializes the space group manager.
func InitSpaceGroups(client *mautrix.Client) {
	threshold := 50
	if v := os.Getenv("SPACE_GROUP_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			threshold = n
		}
	}

	sg := &SpaceGroupManager{
		client:       client,
		threshold:    threshold,
		roomToGroup:  make(map[id.RoomID]int),
		groupMembers: make(map[int]map[id.UserID]bool),
	}

	// Always compute fresh groups on startup to pick up threshold changes
	sg.Refresh()
	spaceGroupMgr = sg
	slog.Info("space_groups: initialized", "threshold", threshold)
}

// RefreshSpaceGroups triggers a refresh of space group mappings.
func RefreshSpaceGroups() {
	if spaceGroupMgr != nil {
		spaceGroupMgr.Refresh()
	}
}

// GetGroupMembers returns the union of all members across rooms in the same
// space group as roomID. Returns nil if the room is not tracked.
func (sg *SpaceGroupManager) GetGroupMembers(roomID id.RoomID) map[id.UserID]bool {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	gid, ok := sg.roomToGroup[roomID]
	if !ok {
		return nil
	}
	return sg.groupMembers[gid]
}

// Refresh recomputes space groups from live room membership data.
func (sg *SpaceGroupManager) Refresh() {
	ctx := context.Background()

	// Get all rooms the bot is in
	joinedResp, err := sg.client.JoinedRooms(ctx)
	if err != nil {
		slog.Error("space_groups: failed to get joined rooms", "err", err)
		return
	}
	rooms := joinedResp.JoinedRooms
	if len(rooms) == 0 {
		slog.Warn("space_groups: bot is not in any rooms")
		return
	}

	// Fetch members for each room
	roomMembers := make(map[id.RoomID]map[id.UserID]bool, len(rooms))
	for _, roomID := range rooms {
		resp, err := sg.client.JoinedMembers(ctx, roomID)
		if err != nil {
			slog.Warn("space_groups: failed to get members", "room", roomID, "err", err)
			continue
		}
		members := make(map[id.UserID]bool, len(resp.Joined))
		for uid := range resp.Joined {
			members[uid] = true
		}
		roomMembers[roomID] = members
	}

	// Strict grouping: a room only joins a group if it meets the overlap
	// threshold with EVERY room already in that group (no transitive chaining).
	roomList := make([]id.RoomID, 0, len(roomMembers))
	for r := range roomMembers {
		roomList = append(roomList, r)
	}

	// Precompute pairwise overlap pass/fail
	meetsThreshold := func(a, b id.RoomID) bool {
		membersA, membersB := roomMembers[a], roomMembers[b]
		smallerSize := len(membersA)
		if len(membersB) < smallerSize {
			smallerSize = len(membersB)
		}
		if smallerSize == 0 {
			return false
		}
		overlap := 0
		if len(membersA) <= len(membersB) {
			for uid := range membersA {
				if membersB[uid] {
					overlap++
				}
			}
		} else {
			for uid := range membersB {
				if membersA[uid] {
					overlap++
				}
			}
		}
		return overlap*100 >= smallerSize*sg.threshold
	}

	// Build groups: try to add each room to an existing group where it
	// meets the threshold with every member. Otherwise start a new group.
	var groups [][]id.RoomID
	for _, r := range roomList {
		placed := false
		for gi, group := range groups {
			fitsAll := true
			for _, member := range group {
				if !meetsThreshold(r, member) {
					fitsAll = false
					break
				}
			}
			if fitsAll {
				groups[gi] = append(groups[gi], r)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []id.RoomID{r})
		}
	}

	// Assign group IDs
	newRoomToGroup := make(map[id.RoomID]int, len(roomList))
	for gid, group := range groups {
		for _, r := range group {
			newRoomToGroup[r] = gid + 1
		}
	}

	// Build group member unions
	newGroupMembers := make(map[int]map[id.UserID]bool)
	for r, gid := range newRoomToGroup {
		if newGroupMembers[gid] == nil {
			newGroupMembers[gid] = make(map[id.UserID]bool)
		}
		for uid := range roomMembers[r] {
			newGroupMembers[gid][uid] = true
		}
	}

	// Persist to DB — only replace rows for rooms we successfully fetched
	d := db.Get()
	tx, err := d.Begin()
	if err != nil {
		slog.Error("space_groups: begin tx", "err", err)
		return
	}
	defer tx.Rollback()
	// Delete only rooms we have fresh data for (preserve entries for rooms that failed to fetch)
	for r := range newRoomToGroup {
		if _, err := tx.Exec(`DELETE FROM space_groups WHERE room_id = ?`, string(r)); err != nil {
			slog.Error("space_groups: delete row", "room", r, "err", err)
			return
		}
	}
	for r, gid := range newRoomToGroup {
		if _, err := tx.Exec(`INSERT INTO space_groups (room_id, group_id, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			string(r), gid); err != nil {
			slog.Error("space_groups: insert row", "room", r, "err", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("space_groups: commit", "err", err)
		return
	}

	// Swap cache under write lock
	sg.mu.Lock()
	sg.roomToGroup = newRoomToGroup
	sg.groupMembers = newGroupMembers
	sg.mu.Unlock()

	// Log summary
	groupCounts := make(map[int]int)
	for _, gid := range newRoomToGroup {
		groupCounts[gid]++
	}
	multiRoom := 0
	for _, count := range groupCounts {
		if count > 1 {
			multiRoom++
		}
	}
	slog.Info("space_groups: refresh complete",
		"rooms", len(newRoomToGroup),
		"groups", len(groupCounts),
		"multi_room_groups", multiRoom,
		"threshold", sg.threshold)
}

// rebuildMemberCache fetches live membership for rooms in stored groups.
func (sg *SpaceGroupManager) rebuildMemberCache() {
	ctx := context.Background()
	newGroupMembers := make(map[int]map[id.UserID]bool)

	for roomID, gid := range sg.roomToGroup {
		resp, err := sg.client.JoinedMembers(ctx, roomID)
		if err != nil {
			slog.Warn("space_groups: rebuild cache failed for room", "room", roomID, "err", err)
			continue
		}
		if newGroupMembers[gid] == nil {
			newGroupMembers[gid] = make(map[id.UserID]bool)
		}
		for uid := range resp.Joined {
			newGroupMembers[gid][uid] = true
		}
	}

	sg.mu.Lock()
	sg.groupMembers = newGroupMembers
	sg.mu.Unlock()
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

// simpleMarkdownToHTML converts the limited Markdown subset used in bot messages
// (**bold**, _italic_, `code`, newlines) to Matrix-compatible HTML.
var (
	mdBoldRe   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdItalicRe = regexp.MustCompile(`(?:^|[ (])_([^_]+?)_(?:$|[ ).,!?])`)
	mdCodeRe   = regexp.MustCompile("`([^`]+)`")
	mdHasFmt   = regexp.MustCompile(`\*\*|(?:^|[ (])_[^_]+_(?:$|[ ).,!?])|` + "`")
)

func simpleMarkdownToHTML(text string) string {
	h := html.EscapeString(text)
	h = mdBoldRe.ReplaceAllString(h, "<strong>$1</strong>")
	// Italic needs careful handling to not match snake_case
	h = mdItalicRe.ReplaceAllStringFunc(h, func(m string) string {
		// Preserve leading/trailing non-underscore chars
		start := 0
		for start < len(m) && m[start] != '_' {
			start++
		}
		end := len(m) - 1
		for end > start && m[end] != '_' {
			end--
		}
		return m[:start] + "<em>" + m[start+1:end] + "</em>" + m[end+1:]
	})
	h = mdCodeRe.ReplaceAllString(h, "<code>$1</code>")
	h = strings.ReplaceAll(h, "\n", "<br>\n")
	return h
}

var mdStripItalicRe = regexp.MustCompile(`_([^_]+?)_`)

func stripMarkdown(text string) string {
	s := mdBoldRe.ReplaceAllString(text, "$1")
	s = mdStripItalicRe.ReplaceAllString(s, "$1")
	s = mdCodeRe.ReplaceAllString(s, "$1")
	return s
}

// textContent builds a MessageEventContent, adding HTML formatting when Markdown is detected.
func textContent(text string) *event.MessageEventContent {
	if mdHasFmt.MatchString(text) {
		return &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          stripMarkdown(text),
			Format:        event.FormatHTML,
			FormattedBody: simpleMarkdownToHTML(text),
		}
	}
	return &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	}
}

// SendMessage sends a message to a room, auto-formatting Markdown as HTML.
func (b *Base) SendMessage(roomID id.RoomID, text string) error {
	content := textContent(text)
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send message", "room", roomID, "err", err)
	}
	return err
}

// SendMessageID sends a message and returns the event ID.
func (b *Base) SendMessageID(roomID id.RoomID, text string) (id.EventID, error) {
	content := textContent(text)
	resp, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send message", "room", roomID, "err", err)
		return "", err
	}
	return resp.EventID, nil
}

// SendThread sends a message in a thread rooted at threadID.
func (b *Base) SendThread(roomID id.RoomID, threadID id.EventID, text string) error {
	content := textContent(text)
	content.RelatesTo = &event.RelatesTo{
		Type:    event.RelThread,
		EventID: threadID,
		InReplyTo: &event.InReplyTo{
			EventID: threadID,
		},
		IsFallingBack: true,
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send thread message", "room", roomID, "err", err)
	}
	return err
}

// SendNotice sends an m.notice message to a room.
func (b *Base) SendNotice(roomID id.RoomID, text string) error {
	content := textContent(text)
	content.MsgType = event.MsgNotice
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	return err
}

// SendReply sends a reply to a specific event.
func (b *Base) SendReply(roomID id.RoomID, eventID id.EventID, text string) error {
	content := textContent(text)
	content.RelatesTo = &event.RelatesTo{
		InReplyTo: &event.InReplyTo{
			EventID: eventID,
		},
	}
	_, err := b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	if err != nil {
		slog.Error("failed to send reply", "room", roomID, "err", err)
	}
	return err
}

// SendHTML sends an HTML-formatted message.
func (b *Base) SendHTML(roomID id.RoomID, plain, htmlBody string) error {
	content := &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          plain,
		Format:        event.FormatHTML,
		FormattedBody: htmlBody,
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

// GetDMRoom returns the DM room for a user, creating one if needed.
func (b *Base) GetDMRoom(userID id.UserID) (id.RoomID, error) {
	dmCacheMu.Lock()
	if roomID, ok := dmCache[userID]; ok {
		dmCacheMu.Unlock()
		return roomID, nil
	}
	dmCacheMu.Unlock()

	// Check account data for existing DM rooms
	var dmRooms map[id.UserID][]id.RoomID
	err := b.Client.GetAccountData(context.Background(), "m.direct", &dmRooms)
	if err == nil {
		if rooms, ok := dmRooms[userID]; ok && len(rooms) > 0 {
			roomID := rooms[len(rooms)-1] // use most recent
			dmCacheMu.Lock()
			dmCache[userID] = roomID
			dmCacheMu.Unlock()
			return roomID, nil
		}
	}

	// No existing DM room — create one
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
		return "", fmt.Errorf("create DM room: %w", err)
	}

	dmCacheMu.Lock()
	dmCache[userID] = resp.RoomID
	dmCacheMu.Unlock()
	return resp.RoomID, nil
}

// IsDMRoom checks if the given room is a known DM room for the given user.
func IsDMRoom(roomID id.RoomID, userID id.UserID) bool {
	dmCacheMu.Lock()
	defer dmCacheMu.Unlock()
	cached, ok := dmCache[userID]
	return ok && cached == roomID
}

// SendDM sends a direct message to a user. Reuses existing DM room if available.
func (b *Base) SendDM(userID id.UserID, text string) error {
	roomID, err := b.GetDMRoom(userID)
	if err != nil {
		return err
	}
	return b.SendMessage(roomID, text)
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
