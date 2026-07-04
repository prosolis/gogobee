package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// cryptoToDeviceTypes are the to-device event types the crypto machine must see
// to establish Olm/Megolm sessions and share/receive room keys. In /sync mode
// the cryptohelper gets these automatically; under the appservice transaction
// model we route each one to mach.HandleToDeviceEvent ourselves.
var cryptoToDeviceTypes = []event.Type{
	event.ToDeviceEncrypted,
	event.ToDeviceRoomKey,
	event.ToDeviceForwardedRoomKey,
	event.ToDeviceRoomKeyRequest,
	event.ToDeviceRoomKeyWithheld,
	event.ToDeviceOrgMatrixRoomKeyWithheld,
	event.ToDeviceSecretRequest,
	event.ToDeviceSecretSend,
	event.ToDeviceDummy,
}

// newAppserviceSession builds the appservice-mode Session: as_token auth, a
// cryptohelper that mints the bot's device via MSC4190, and an EventProcessor
// fed by Synapse's transaction pushes (in place of /sync). The bot is an
// appservice user — Synapse forbids AS users from /sync, so all events, plus the
// E2EE extensions (to-device / device lists / OTK counts), arrive over the
// transaction API instead.
func newAppserviceSession(cfg Config) (*Session, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if cfg.UserID == "" {
		return nil, fmt.Errorf("BOT_USER_ID is required")
	}
	if cfg.RegistrationPath == "" {
		return nil, fmt.Errorf("AS_REGISTRATION is required in appservice mode")
	}
	if cfg.HomeserverDomain == "" {
		return nil, fmt.Errorf("HOMESERVER_DOMAIN is required in appservice mode (server_name, e.g. parodia.dev)")
	}
	if !((&appservice.HostConfig{Hostname: cfg.ListenHost, Port: cfg.ListenPort}).IsConfigured()) {
		return nil, fmt.Errorf("AS_LISTEN_HOST/AS_LISTEN_PORT must be set in appservice mode")
	}

	ctx := context.Background()

	reg, err := appservice.LoadRegistration(cfg.RegistrationPath)
	if err != nil {
		return nil, fmt.Errorf("load appservice registration %q: %w", cfg.RegistrationPath, err)
	}
	// Gate for to-device delivery: handleTransaction only pumps to-device events
	// when this is set (appservice/http.go). Force it on regardless of the yaml so
	// E2EE key exchange can't silently break on a missing field.
	reg.EphemeralEvents = true

	as, err := appservice.CreateFull(appservice.CreateOpts{
		Registration:     reg,
		HomeserverDomain: cfg.HomeserverDomain,
		HomeserverURL:    cfg.Homeserver,
		HostConfig:       appservice.HostConfig{Hostname: cfg.ListenHost, Port: cfg.ListenPort},
	})
	if err != nil {
		return nil, fmt.Errorf("create appservice: %w", err)
	}
	// Surface the HTTP listener + transaction logs (default is a silent Nop).
	as.Log = zerolog.New(os.Stderr).With().Timestamp().Str("component", "appservice").Logger().
		Level(zerolog.InfoLevel)

	userID := id.UserID(cfg.UserID)
	if as.BotMXID() != userID {
		return nil, fmt.Errorf("registration sender_localpart resolves to %s but BOT_USER_ID is %s", as.BotMXID(), userID)
	}

	client := as.BotClient() // as_token auth + SetAppServiceUserID (?user_id=) assertion

	// Validate the token + identity before we start listening.
	whoami, err := client.Whoami(ctx)
	if err != nil {
		return nil, fmt.Errorf("appservice token validation failed (whoami): %w", err)
	}
	if whoami.UserID != userID {
		return nil, fmt.Errorf("appservice identity mismatch: token resolves to %s but BOT_USER_ID is %s", whoami.UserID, userID)
	}
	slog.Info("appservice token valid", "user_id", whoami.UserID)

	// ---- E2EE via cryptohelper (MSC4190 device creation, no /login) ----
	cryptoDBPath := filepath.Join(cfg.DataDir, "crypto.db")
	ch, err := cryptohelper.NewCryptoHelper(client, []byte("gogobee_pickle_key"), cryptoDBPath)
	if err != nil {
		return nil, fmt.Errorf("init crypto helper: %w", err)
	}
	// MSC4190: the appservice creates/refreshes its own device via PUT /devices
	// instead of the UIA-gated login. The crypto store persists the device ID, so
	// restarts reuse it. LoginAs carries only the display name (never calls /login
	// in MSC4190 mode). client.Syncer is nil here, so Init does NOT wire /sync
	// handlers — we drive the crypto machine from transactions below instead.
	ch.MSC4190 = true
	ch.LoginAs = &mautrix.ReqLogin{InitialDeviceDisplayName: cfg.DisplayName}
	if err := ch.Init(ctx); err != nil {
		return nil, fmt.Errorf("crypto helper init (MSC4190 device create): %w", err)
	}
	client.Crypto = ch

	// ---- Event processor: replicate the cryptohelper's /sync wiring against
	// the appservice transaction channels ----
	ep := appservice.NewEventProcessor(as)
	mach := ch.Machine()

	// Crypto plumbing that /sync would otherwise carry:
	ep.OnOTK(mach.HandleOTKCounts)
	ep.OnDeviceList(mach.HandleDeviceLists)
	for _, t := range cryptoToDeviceTypes {
		ep.On(t, mach.HandleToDeviceEvent)
	}

	// Keep the client's StateStore current so outgoing sends know which rooms are
	// encrypted and who to share keys with (no /sync backfill here), and let the
	// crypto machine track membership for key sharing. PrependHandler so state is
	// updated before app handlers (auto-join/moderation) run for the same event.
	ep.PrependHandler(event.StateEncryption, func(ctx context.Context, evt *event.Event) {
		mautrix.UpdateStateStore(ctx, as.StateStore, evt)
	})
	ep.PrependHandler(event.StateMember, func(ctx context.Context, evt *event.Event) {
		mautrix.UpdateStateStore(ctx, as.StateStore, evt)
		mach.HandleMemberEvent(ctx, evt)
	})

	// Without /sync there is no state backfill, so the client's StateStore starts
	// empty. Before we hand off a decrypted event (whose reply may need to be
	// encrypted), resolve the room once: mark it encrypted and populate its member
	// list. Otherwise outbound sends go plaintext (IsEncrypted=false) and, worse,
	// the group session is shared to nobody (GetRoomJoinedOrInvitedMembers empty)
	// so recipients can't decrypt the bot's replies.
	var resolved sync.Map // roomID -> struct{}, resolved once
	resolveRoom := func(ctx context.Context, roomID id.RoomID) {
		if _, done := resolved.LoadOrStore(roomID, struct{}{}); done {
			return
		}
		var enc event.EncryptionEventContent
		if err := client.StateEvent(ctx, roomID, event.StateEncryption, "", &enc); err == nil && enc.Algorithm != "" {
			_ = as.StateStore.SetEncryptionEvent(ctx, roomID, &enc)
		}
		if members, err := client.Members(ctx, roomID); err == nil {
			_ = as.StateStore.ReplaceCachedMembers(ctx, roomID, members.Chunk)
		} else {
			slog.Warn("appservice: failed to fetch room members; will retry", "room", roomID, "err", err)
			resolved.Delete(roomID) // allow a later event to retry
		}
	}

	// Decrypt inbound encrypted room events and re-dispatch the plaintext so the
	// normal message/reaction handlers fire (mirrors cryptohelper.HandleEncrypted).
	ep.On(event.EventEncrypted, func(ctx context.Context, evt *event.Event) {
		resolveRoom(ctx, evt.RoomID)
		decrypted, err := ch.Decrypt(ctx, evt)
		if err != nil {
			slog.Warn("appservice: failed to decrypt event", "room", evt.RoomID, "event", evt.ID, "err", err)
			return
		}
		ep.Dispatch(ctx, decrypted)
	})

	return &Session{
		Client: client,
		mode:   "appservice",
		as:     as,
		ep:     ep,
	}, nil
}

// runAppservice starts the transaction dispatchers and the HTTP listener, then
// blocks until ctx is cancelled.
func (s *Session) runAppservice(ctx context.Context) error {
	s.ep.Start(ctx)
	s.as.Ready = true

	errCh := make(chan struct{})
	go func() {
		s.as.Start() // blocks in ListenAndServe until Stop()
		close(errCh)
	}()

	slog.Info("appservice listener started", "address", s.as.Host.Address())

	select {
	case <-ctx.Done():
		return nil
	case <-errCh:
		return fmt.Errorf("appservice HTTP listener exited unexpectedly")
	}
}
