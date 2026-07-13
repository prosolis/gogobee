package bot

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// lazyStateStore resolves room state from the homeserver on first use instead of
// waiting for it to arrive over a sync.
//
// The appservice has no /sync, so the state store only ever learns a room is
// encrypted from an m.room.encryption event pushed in a transaction — which for
// an already-configured room never happens again. Anything the bot says on its
// own initiative (ambient DMs, expedition beats, briefings) therefore reached
// SendMessageEvent with IsEncrypted=false and went out as plaintext into an
// encrypted room. The state store being in-memory made every restart a fresh
// chance to leak.
//
// Both reads the send path depends on are resolved here on the first miss and
// cached: whether the room is encrypted, and who to share the group session with
// (an empty member list encrypts to nobody, which is its own outage). Neither
// read is allowed to fail open — a lookup that cannot be resolved returns an
// error, which aborts the send. A failed message is recoverable; a plaintext one
// that has already landed in an encrypted room is not.
type lazyStateStore struct {
	// Embeds both interfaces the store is consumed through: the appservice
	// dispatches state updates through the first, and the crypto machine type-
	// asserts the client's store to the second (cryptohelper.NewCryptoHelper
	// rejects a store that fails the assertion). Embedding only the appservice
	// interface silently drops crypto's extra methods from the concrete type.
	innerStateStore

	client *mautrix.Client

	mu    sync.Mutex
	locks map[id.RoomID]*sync.Mutex
	// Rooms whose m.room.encryption state we have asked the server about. The
	// underlying store cannot distinguish "not encrypted" from "never heard of
	// it", so the distinction is tracked here.
	encryptionResolved map[id.RoomID]bool
}

// innerStateStore is the full method set the wrapped store must provide.
type innerStateStore interface {
	appservice.StateStore
	crypto.StateStore
}

// The wrapper is consumed through both, and losing either one is a startup
// failure rather than a build failure without these.
var (
	_ appservice.StateStore = (*lazyStateStore)(nil)
	_ crypto.StateStore     = (*lazyStateStore)(nil)
)

func newLazyStateStore(inner innerStateStore) *lazyStateStore {
	return &lazyStateStore{
		innerStateStore:    inner,
		locks:              make(map[id.RoomID]*sync.Mutex),
		encryptionResolved: make(map[id.RoomID]bool),
	}
}

// lockRoom serialises resolution per room so a burst of sends into a cold room
// makes one request rather than one per message.
func (s *lazyStateStore) lockRoom(roomID id.RoomID) func() {
	s.mu.Lock()
	lock, ok := s.locks[roomID]
	if !ok {
		lock = &sync.Mutex{}
		s.locks[roomID] = lock
	}
	s.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func (s *lazyStateStore) isEncryptionResolved(roomID id.RoomID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encryptionResolved[roomID]
}

func (s *lazyStateStore) markEncryptionResolved(roomID id.RoomID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encryptionResolved[roomID] = true
}

// IsEncrypted answers from the underlying store once the room's encryption state
// has been resolved, fetching it from the server the first time.
func (s *lazyStateStore) IsEncrypted(ctx context.Context, roomID id.RoomID) (bool, error) {
	if !s.isEncryptionResolved(roomID) {
		unlock := s.lockRoom(roomID)
		defer unlock()

		if !s.isEncryptionResolved(roomID) {
			var enc event.EncryptionEventContent
			err := s.client.StateEvent(ctx, roomID, event.StateEncryption, "", &enc)
			switch {
			case err == nil:
				if enc.Algorithm != "" {
					if err := s.innerStateStore.SetEncryptionEvent(ctx, roomID, &enc); err != nil {
						return false, fmt.Errorf("cache encryption state for %s: %w", roomID, err)
					}
				}
			case errors.Is(err, mautrix.MNotFound):
				// No m.room.encryption at all: the room really is unencrypted.
			default:
				// Unresolved. Refuse rather than guess "unencrypted" and leak.
				return false, fmt.Errorf("resolve encryption state for %s: %w", roomID, err)
			}
			s.markEncryptionResolved(roomID)
		}
	}
	return s.innerStateStore.IsEncrypted(ctx, roomID)
}

// GetEncryptionEvent is how the crypto machine reads a room's megolm settings
// (session rotation period, etc). It needs the same resolution as IsEncrypted,
// since an unresolved room would otherwise report no encryption event.
func (s *lazyStateStore) GetEncryptionEvent(ctx context.Context, roomID id.RoomID) (*event.EncryptionEventContent, error) {
	if _, err := s.IsEncrypted(ctx, roomID); err != nil {
		return nil, err
	}
	return s.innerStateStore.GetEncryptionEvent(ctx, roomID)
}

// GetRoomJoinedOrInvitedMembers backs megolm session sharing. Without a sync
// there is no member backfill, so an unfetched room would report zero members
// and the bot would encrypt to an audience of nobody.
func (s *lazyStateStore) GetRoomJoinedOrInvitedMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	fetched, err := s.innerStateStore.HasFetchedMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("check member cache for %s: %w", roomID, err)
	}
	if !fetched {
		unlock := s.lockRoom(roomID)
		defer unlock()

		if fetched, err := s.innerStateStore.HasFetchedMembers(ctx, roomID); err != nil {
			return nil, fmt.Errorf("check member cache for %s: %w", roomID, err)
		} else if !fetched {
			members, err := s.client.Members(ctx, roomID)
			if err != nil {
				return nil, fmt.Errorf("fetch members of %s: %w", roomID, err)
			}
			// No membership filter, so this also marks the room as fetched.
			if err := s.innerStateStore.ReplaceCachedMembers(ctx, roomID, members.Chunk); err != nil {
				return nil, fmt.Errorf("cache members of %s: %w", roomID, err)
			}
		}
	}
	return s.innerStateStore.GetRoomJoinedOrInvitedMembers(ctx, roomID)
}
