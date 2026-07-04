package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
)

// Session is the bot's live Matrix connection plus its event source. It hides
// the difference between the two transports the bot can run under:
//
//   - masdevice: a normal Matrix user authenticated via the MAS OAuth device
//     grant, receiving events over /sync (mautrix DefaultSyncer).
//   - appservice: a Synapse appservice authenticated by its as_token, receiving
//     events pushed over the appservice transaction API (mautrix EventProcessor).
//
// Callers register the same three handlers (message/member/reaction) via
// OnEventType and call Run regardless of mode; the plugin layer never knows which
// transport is in use.
type Session struct {
	Client *mautrix.Client
	mode   string

	// masdevice
	syncer *mautrix.DefaultSyncer

	// appservice
	as           *appservice.AppService
	ep           *appservice.EventProcessor
	presenceOnce sync.Once
}

// EventHandler matches both DefaultSyncer.OnEventType and EventProcessor.On.
type EventHandler = func(ctx context.Context, evt *event.Event)

// NewSession builds a Session for the configured auth mode. Default and
// "masdevice" use the MAS device grant; "appservice" uses the transaction model.
func NewSession(cfg Config) (*Session, error) {
	switch cfg.AuthMode {
	case "appservice":
		return newAppserviceSession(cfg)
	case "", "masdevice":
		return newMASDeviceSession(cfg)
	default:
		return nil, fmt.Errorf("unknown AUTH_MODE %q (want appservice or masdevice)", cfg.AuthMode)
	}
}

// newMASDeviceSession wraps the existing device-grant client (NewClient) and its
// DefaultSyncer. This is the retained rollback path — unchanged behaviour.
func newMASDeviceSession(cfg Config) (*Session, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Session{
		Client: client,
		mode:   "masdevice",
		syncer: client.Syncer.(*mautrix.DefaultSyncer),
	}, nil
}

// OnEventType registers a handler for the given event type against whichever
// event source backs this session.
func (s *Session) OnEventType(evtType event.Type, fn EventHandler) {
	switch s.mode {
	case "appservice":
		s.ep.On(evtType, fn)
	default:
		s.syncer.OnEventType(evtType, fn)
	}
}

// StartPresence begins actively maintaining the bot's Matrix presence, tied to
// ctx. In appservice mode it launches the online heartbeat (runPresenceHeartbeat);
// masdevice mode relies on the /sync long-poll's implicit presence refresh, so it
// is a no-op there. Call this early — before the slow plugin init — so the bot
// shows online from boot rather than only once the transaction listener starts
// (~2min later). The presence PUT is outbound-only and needs no event handlers, so
// starting it ahead of the listener is safe. Idempotent: extra calls do nothing.
func (s *Session) StartPresence(ctx context.Context) {
	if s.mode != "appservice" {
		return
	}
	s.presenceOnce.Do(func() {
		go s.runPresenceHeartbeat(ctx)
	})
}

// Run blocks, delivering events to registered handlers, until ctx is cancelled.
func (s *Session) Run(ctx context.Context) error {
	switch s.mode {
	case "appservice":
		return s.runAppservice(ctx)
	default:
		return s.runSync(ctx)
	}
}

// Stop halts the event source. Safe to call once.
func (s *Session) Stop() {
	switch s.mode {
	case "appservice":
		if s.ep != nil {
			s.ep.Stop()
		}
		if s.as != nil {
			s.as.Stop()
		}
	default:
		s.Client.StopSync()
	}
}

// runSync is the device-grant /sync loop (moved verbatim from main.go): restart
// on transient failure, exit on context cancel.
func (s *Session) runSync(ctx context.Context) error {
	for {
		err := s.Client.SyncWithContext(ctx)
		if ctx.Err() != nil {
			return nil // shutdown requested
		}
		if err != nil {
			slog.Error("sync stopped, restarting in 5s", "err", err)
		} else {
			slog.Warn("sync returned without error, restarting in 5s")
		}
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return nil
		}
	}
}
