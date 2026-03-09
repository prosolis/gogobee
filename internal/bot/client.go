package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gogobee/internal/util"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"
)

// DeviceInfo holds persisted device credentials.
type DeviceInfo struct {
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
	UserID      string `json:"user_id"`
}

// Config holds the bot's startup configuration.
type Config struct {
	Homeserver  string
	UserID      string
	Password    string
	DataDir     string
	DisplayName string
}

// NewClient creates and configures a mautrix client with E2EE support.
// The cryptohelper handles:
//   - Persistent crypto store in SQLite (device keys, sessions, cross-signing keys)
//   - Automatic cross-signing bootstrap (self-signs the device on first run)
//   - Automatic device trust via cross-signing (no manual verification needed)
//   - Megolm session sharing and key exchange
//   - Olm session management for device-to-device encryption
//
// This solves the TS version's device verification issues because:
//   1. Crypto state persists across restarts (not in-memory like fake-indexeddb)
//   2. Cross-signing makes other devices trust this bot automatically
//   3. The bot trusts all users' devices by default (appropriate for a bot)
//   4. No manual emoji/SAS verification needed
func NewClient(cfg Config) (*mautrix.Client, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	devicePath := filepath.Join(cfg.DataDir, "device.json")

	// Try to load existing device credentials
	device, err := loadDevice(devicePath)
	if err != nil {
		slog.Info("no existing device found, will login fresh")
	}

	var client *mautrix.Client

	if device != nil {
		// Validate existing token
		valid, _ := util.IsTokenValid(cfg.Homeserver, device.AccessToken)
		if valid {
			slog.Info("existing device credentials valid", "device_id", device.DeviceID)
			userID := id.UserID(device.UserID)
			client, err = mautrix.NewClient(cfg.Homeserver, userID, device.AccessToken)
			if err != nil {
				return nil, fmt.Errorf("create client with existing token: %w", err)
			}
			client.DeviceID = id.DeviceID(device.DeviceID)
		} else {
			slog.Warn("existing device credentials invalid, logging in again")
			device = nil
		}
	}

	if device == nil {
		// Fresh login
		loginResp, err := util.LoginWithPassword(cfg.Homeserver, cfg.UserID, cfg.Password, cfg.DisplayName)
		if err != nil {
			return nil, fmt.Errorf("login: %w", err)
		}

		userID := id.UserID(loginResp.UserID)
		client, err = mautrix.NewClient(cfg.Homeserver, userID, loginResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("create client: %w", err)
		}
		client.DeviceID = id.DeviceID(loginResp.DeviceID)

		// Save device info
		device = &DeviceInfo{
			AccessToken: loginResp.AccessToken,
			DeviceID:    loginResp.DeviceID,
			UserID:      loginResp.UserID,
		}
		if err := saveDevice(devicePath, device); err != nil {
			slog.Warn("failed to save device info", "err", err)
		}

		slog.Info("logged in successfully",
			"user_id", loginResp.UserID,
			"device_id", loginResp.DeviceID,
		)
	}

	// Set up E2EE via cryptohelper — stores crypto state in its own SQLite DB,
	// separate from the main app database. Unlike the TS version which used an
	// in-memory fake-indexeddb store that was lost on restart (causing constant
	// re-verification), mautrix-go's cryptohelper persists everything in SQLite:
	// device keys, olm/megolm sessions, cross-signing keys, and device trust state.
	//
	// We pass just the raw file path — the cryptohelper wraps it in a file: URI
	// with _txlock=immediate internally (see cryptohelper.go line 82).
	cryptoDBPath := filepath.Join(cfg.DataDir, "crypto.db")
	ch, err := cryptohelper.NewCryptoHelper(client, []byte("gogobee_pickle_key"), cryptoDBPath)
	if err != nil {
		return nil, fmt.Errorf("init crypto helper: %w", err)
	}

	// LoginAs enables the cryptohelper to re-login if the token expires,
	// and to bootstrap cross-signing on first run. Cross-signing means:
	//   - The bot's master key signs its own device key
	//   - Other users/devices that have verified the bot's master key
	//     will automatically trust this device
	//   - No interactive emoji/SAS verification needed
	ch.LoginAs = &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: cfg.UserID,
		},
		Password:                 cfg.Password,
		InitialDeviceDisplayName: cfg.DisplayName,
	}

	if err := ch.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("crypto helper init: %w", err)
	}

	// Attach crypto helper to client
	client.Crypto = ch

	// Bootstrap cross-signing: generate keys, sign own device, sign master key.
	// This makes the bot's device show as "verified" to other users.
	mach := ch.Machine()
	recoveryKey, _, err := mach.GenerateAndUploadCrossSigningKeys(context.Background(), func(ui *mautrix.RespUserInteractive) interface{} {
		return map[string]interface{}{
			"type": mautrix.AuthTypePassword,
			"identifier": map[string]interface{}{
				"type": mautrix.IdentifierTypeUser,
				"user": cfg.UserID,
			},
			"password": cfg.Password,
			"session":  ui.Session,
		}
	}, "")
	if err != nil {
		slog.Warn("cross-signing: key upload failed (may already exist)", "err", err)
	} else {
		slog.Info("cross-signing: keys uploaded", "recovery_key", recoveryKey)
	}

	if err := mach.SignOwnDevice(context.Background(), mach.OwnIdentity()); err != nil {
		slog.Warn("cross-signing: sign own device failed", "err", err)
	} else {
		slog.Info("cross-signing: own device signed")
	}

	if err := mach.SignOwnMasterKey(context.Background()); err != nil {
		slog.Warn("cross-signing: sign master key failed", "err", err)
	} else {
		slog.Info("cross-signing: master key signed")
	}

	slog.Info("E2EE initialized",
		"device_id", client.DeviceID,
		"crypto_store", "sqlite-persistent",
	)

	return client, nil
}

func loadDevice(path string) (*DeviceInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info DeviceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func saveDevice(path string, info *DeviceInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
