package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

// Config holds the bot's startup configuration.
type Config struct {
	Homeserver  string
	UserID      string // Bot's full Matrix user ID, e.g. @gogobee:example.com
	ASToken     string // Appservice as_token from registration.yaml
	DataDir     string
	DisplayName string
}

// NewClient creates and configures a mautrix client authenticated as an
// application service, with E2EE via the cryptohelper.
//
// Why appservice instead of password login:
//
// Under Matrix Authentication Service (MAS), the legacy password login
// (m.login.password) and User-Interactive Auth (UIA) flows are being removed —
// auth moves to OAuth2/OIDC. A bot built on password login + password-UIA
// cross-signing (the old code) breaks in a MAS world: short-lived tokens with
// no refresh, and UIA-gated key uploads that simply fail.
//
// The appservice model sidesteps MAS entirely. The as_token is a Synapse-level
// trust relationship declared in registration.yaml — Synapse validates it
// directly, not via MAS — so it never expires and needs no login flow. Device
// creation uses MSC4190 (PUT /devices), which replaces the UIA-gated device
// dance for appservice users. This is the MAS-durable path.
//
// The cryptohelper still handles everything E2EE:
//   - Persistent crypto store in SQLite (device keys, sessions, cross-signing)
//   - Megolm session sharing / key exchange, Olm device-to-device sessions
//   - The bot trusts all users' devices by default (appropriate for a bot)
//   - No interactive emoji/SAS verification needed
func NewClient(cfg Config) (*mautrix.Client, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if cfg.ASToken == "" {
		return nil, fmt.Errorf("AS_TOKEN is required (appservice registration as_token)")
	}
	if cfg.UserID == "" {
		return nil, fmt.Errorf("BOT_USER_ID is required")
	}

	ctx := context.Background()
	userID := id.UserID(cfg.UserID)

	// The as_token IS the access token. No login flow, no refresh — Synapse
	// trusts it for any user in the registration's namespace.
	client, err := mautrix.NewClient(cfg.Homeserver, userID, cfg.ASToken)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Assert our own user ID on every request (appservice identity assertion,
	// ?user_id=). Required so Synapse knows which namespaced user we're acting
	// as — including on /sync and the MSC4190 device creation below.
	client.SetAppServiceUserID = true

	// Validate the token + homeserver reachability up front with a clear error.
	whoami, err := client.Whoami(ctx)
	if err != nil {
		return nil, fmt.Errorf("appservice token validation failed (whoami): %w", err)
	}
	if whoami.UserID != userID {
		return nil, fmt.Errorf("appservice identity mismatch: token resolves to %s but BOT_USER_ID is %s", whoami.UserID, userID)
	}
	slog.Info("appservice token valid", "user_id", whoami.UserID)

	// Set up E2EE. The cryptohelper persists all crypto state in its own SQLite
	// DB (device keys, olm/megolm sessions, cross-signing keys, device trust),
	// separate from the main app database. The device ID is persisted there too,
	// so restarts reuse the same device — no device.json needed.
	cryptoDBPath := filepath.Join(cfg.DataDir, "crypto.db")
	ch, err := cryptohelper.NewCryptoHelper(client, []byte("gogobee_pickle_key"), cryptoDBPath)
	if err != nil {
		return nil, fmt.Errorf("init crypto helper: %w", err)
	}

	// MSC4190: create/refresh the bot's device via PUT /devices instead of the
	// UIA-gated login dance. On first run the crypto store has no device ID, so
	// a fresh one is generated and persisted; on restart the stored ID is
	// reused (the PUT is idempotent). LoginAs carries only the display name —
	// in MSC4190 mode the cryptohelper never calls /login.
	ch.MSC4190 = true
	ch.LoginAs = &mautrix.ReqLogin{
		InitialDeviceDisplayName: cfg.DisplayName,
	}

	if err := ch.Init(ctx); err != nil {
		return nil, fmt.Errorf("crypto helper init (MSC4190 device create): %w", err)
	}
	client.Crypto = ch

	// Best-effort cross-signing bootstrap. This makes the bot's device show as
	// "verified" to users who have verified the bot's master key. It is NOT
	// required for E2EE to function — the bot already trusts all devices and
	// shares keys — so any failure is logged and ignored.
	//
	// Under MSC3202 (appservice device assertion) Synapse may accept the key
	// upload without UIA, in which case the callback below is never invoked. If
	// the homeserver still demands UIA, we have no password credential to
	// satisfy it (that's the whole point of going MAS-native), so it fails soft.
	mach := ch.Machine()
	recoveryKey, _, err := mach.GenerateAndUploadCrossSigningKeys(ctx, func(ui *mautrix.RespUserInteractive) interface{} {
		slog.Warn("cross-signing: homeserver requested UIA, but appservice auth has no password credential to satisfy it — skipping verified-shield bootstrap")
		return map[string]interface{}{"session": ui.Session}
	}, "")
	if err != nil {
		slog.Warn("cross-signing: key upload skipped (may already exist or UIA required)", "err", err)
	} else {
		slog.Info("cross-signing: keys uploaded", "recovery_key", recoveryKey)
	}

	if err := mach.SignOwnDevice(ctx, mach.OwnIdentity()); err != nil {
		slog.Warn("cross-signing: sign own device failed", "err", err)
	} else {
		slog.Info("cross-signing: own device signed")
	}

	if err := mach.SignOwnMasterKey(ctx); err != nil {
		slog.Warn("cross-signing: sign master key failed", "err", err)
	} else {
		slog.Info("cross-signing: master key signed")
	}

	slog.Info("E2EE initialized",
		"user_id", client.UserID,
		"device_id", client.DeviceID,
		"crypto_store", "sqlite-persistent",
		"auth", "appservice+msc4190",
	)

	return client, nil
}
