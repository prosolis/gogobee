package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

// Config holds the bot's startup configuration.
type Config struct {
	Homeserver  string
	UserID      string // Bot's full Matrix user ID, e.g. @twinbee:parodia.dev
	DataDir     string
	DisplayName string

	// AuthMode selects how the bot connects to Matrix:
	//   "masdevice" (default) — MAS OAuth 2.0 device grant + /sync (masauth.go).
	//   "appservice"          — Matrix appservice: as_token auth + Synapse→bot
	//                           transaction push (appservice.go). MAS-durable:
	//                           no login, no MFA, no token expiry ever.
	// The device-grant path is retained as an instant rollback: flip AUTH_MODE
	// back to masdevice and restart, no rebuild.
	AuthMode string

	// ---- appservice mode only ----
	// RegistrationPath points at the appservice registration YAML (id, as_token,
	// hs_token, sender_localpart, namespaces). Synapse and the bot share it.
	RegistrationPath string
	// ListenHost/ListenPort is where the bot's HTTP listener binds to receive
	// Synapse's transaction pushes (Synapse reaches it via the registration url).
	ListenHost string
	ListenPort uint16
	// HomeserverDomain is the server_name (e.g. parodia.dev), needed to derive
	// the bot's MXID from sender_localpart.
	HomeserverDomain string
}

// NewClient creates and configures a mautrix client authenticated against
// Matrix Authentication Service (MAS) via the OAuth 2.0 device grant, with
// E2EE via the cryptohelper.
//
// Auth flow (see masauth.go for detail):
//   - Discover MAS OAuth endpoints from the homeserver's well-known + OIDC docs.
//   - If we have a persisted refresh token, refresh it for a fresh access token.
//   - Otherwise run the device-authorization grant: print a URL + code for a
//     human to approve ONCE in a browser (as the bot's user), then store the
//     resulting access + refresh tokens.
//   - A background goroutine refreshes the access token before it expires and
//     updates the live client, so the bot runs indefinitely without re-auth.
//
// The bot stays a normal Matrix user (not an appservice), so /sync works — an
// appservice user is forbidden from /sync by Synapse. E2EE is unchanged: the
// cryptohelper persists device keys, olm/megolm sessions and cross-signing in
// its own SQLite store, and the bot trusts all users' devices by default.
func NewClient(cfg Config) (*mautrix.Client, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if cfg.UserID == "" {
		return nil, fmt.Errorf("BOT_USER_ID is required")
	}

	ctx := context.Background()

	// ---- MAS OAuth device-grant authentication ----
	auth := newMASAuth(cfg.DataDir)
	auth.load()
	if err := auth.discover(cfg.Homeserver); err != nil {
		return nil, fmt.Errorf("MAS discovery: %w", err)
	}

	if auth.refreshToken != "" {
		// Returning start: refresh the stored token.
		if err := auth.refresh(); err != nil {
			return nil, fmt.Errorf("MAS token refresh failed (delete %s to re-authorize): %w",
				filepath.Join(cfg.DataDir, "mas_auth.json"), err)
		}
		slog.Info("MAS auth: refreshed access token from stored session", "device_id", auth.deviceID)
	} else {
		// First run: register a client and run the interactive device grant.
		if err := auth.ensureClient(cfg.DisplayName); err != nil {
			return nil, fmt.Errorf("MAS client registration: %w", err)
		}
		if err := auth.deviceFlow(ctx); err != nil {
			return nil, fmt.Errorf("MAS device authorization: %w", err)
		}
	}

	userID := id.UserID(cfg.UserID)
	client, err := mautrix.NewClient(cfg.Homeserver, userID, auth.token())
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	client.DeviceID = id.DeviceID(auth.deviceID)

	// Validate the token + identity before proceeding.
	whoami, err := client.Whoami(ctx)
	if err != nil {
		return nil, fmt.Errorf("token validation failed (whoami): %w", err)
	}
	if whoami.UserID != userID {
		return nil, fmt.Errorf("identity mismatch: token resolves to %s but BOT_USER_ID is %s", whoami.UserID, userID)
	}
	slog.Info("MAS auth: token valid", "user_id", whoami.UserID, "device_id", client.DeviceID)

	// ---- E2EE via cryptohelper ----
	// The crypto store persists device keys, olm/megolm sessions, cross-signing
	// and device trust in its own SQLite DB. The stored device ID must match the
	// one bound to our OAuth token (device: scope); a fresh mas_auth.json is
	// paired with a fresh crypto.db.
	cryptoDBPath := filepath.Join(cfg.DataDir, "crypto.db")
	ch, err := cryptohelper.NewCryptoHelper(client, []byte("gogobee_pickle_key"), cryptoDBPath)
	if err != nil {
		return nil, fmt.Errorf("init crypto helper: %w", err)
	}
	// No LoginAs: we already have a token + device ID; the cryptohelper just
	// attaches E2EE to the existing session.
	if err := ch.Init(ctx); err != nil {
		return nil, fmt.Errorf("crypto helper init: %w", err)
	}
	client.Crypto = ch

	// Best-effort cross-signing bootstrap (makes the bot's device show as
	// verified to users who trust its master key). Not required for E2EE to
	// function; under MAS the key upload may be refused, which we ignore.
	mach := ch.Machine()
	bootstrapCrossSigning(ctx, mach, cfg.DataDir)

	// ---- Background token refresher ----
	// Refresh ~60s before expiry and push the new token into the live client so
	// in-flight /sync and API calls keep authenticating.
	go refreshLoop(context.Background(), auth, client)

	slog.Info("E2EE initialized",
		"user_id", client.UserID,
		"device_id", client.DeviceID,
		"crypto_store", "sqlite-persistent",
		"auth", "mas-oauth-device-grant",
	)

	return client, nil
}

// refreshLoop keeps the client's access token fresh for the life of the process.
func refreshLoop(ctx context.Context, auth *masAuth, client *mautrix.Client) {
	for {
		wait := time.Until(auth.expiry()) - 60*time.Second
		if wait < 10*time.Second {
			wait = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := auth.refresh(); err != nil {
			slog.Error("MAS token refresh failed; retrying in 30s", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}
		client.AccessToken = auth.token()
		slog.Debug("MAS token refreshed", "expires_at", auth.expiry().Format(time.RFC3339))
	}
}
