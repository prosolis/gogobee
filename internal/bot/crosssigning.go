package bot

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
)

// crossSigningFile is the on-disk store for the bot's cross-signing recovery key
// (data/cross_signing.json), alongside mas_auth.json and crypto.db. The key
// decrypts the private cross-signing keys the bot parks in SSSS, so it is the
// only thing that lets a rebuilt crypto.db re-sign the bot's device instead of
// minting a whole new identity and forcing everyone to re-verify.
//
// It is deliberately NOT logged: the screen wrapper pipes the bot's output
// through tee, which truncates the log on every restart, so a key that only ever
// exists in a log line is a key you lose on the next boot.
const crossSigningFile = "cross_signing.json"

type crossSigningStore struct {
	RecoveryKey string `json:"recovery_key"`
}

func crossSigningPath(dataDir string) string {
	return filepath.Join(dataDir, crossSigningFile)
}

// loadRecoveryKey returns the stored key, or "" when there is none.
func loadRecoveryKey(dataDir string) string {
	data, err := os.ReadFile(crossSigningPath(dataDir))
	if err != nil {
		return "" // fresh install
	}
	var s crossSigningStore
	if err := json.Unmarshal(data, &s); err != nil {
		slog.Warn("cross-signing: corrupt recovery-key store, ignoring", "path", crossSigningPath(dataDir), "err", err)
		return ""
	}
	return s.RecoveryKey
}

func saveRecoveryKey(dataDir, key string) {
	data, err := json.MarshalIndent(crossSigningStore{RecoveryKey: key}, "", "  ")
	if err != nil {
		slog.Error("cross-signing: marshal recovery key failed", "err", err)
		return
	}
	path := crossSigningPath(dataDir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		slog.Error("cross-signing: could not persist recovery key; a crypto.db rebuild will need a re-verify", "path", path, "err", err)
		return
	}
	slog.Info("cross-signing: recovery key persisted", "path", path)
}

// uiaSession answers a user-interactive-auth challenge with just the session ID.
// Under MAS the key upload is either allowed outright or refused; there is no
// password to offer.
func uiaSession(ui *mautrix.RespUserInteractive) interface{} {
	return map[string]interface{}{"session": ui.Session}
}

// bootstrapCrossSigning establishes the bot's cross-signing identity, which is
// what lets clients show it as verified rather than as an unknown device. E2EE
// works without it; only the verification badge depends on it.
//
// It must never mint a second identity by accident. mautrix's
// GenerateAndUploadCrossSigningKeys is unconditional: every call generates a fresh
// master/self/user-signing trio and overwrites the published one. Calling it on
// each start reminted the bot's identity every boot, which is why clients kept
// asking users to re-verify. So generate only when there is no identity to keep,
// or when the operator explicitly asks for a reset.
//
// The private keys live server-side in SSSS, never in crypto.db, and mach.Load
// does not restore them. A bot that keeps its crypto.db stays signed from its
// first signing and needs nothing here. A bot whose crypto.db was rebuilt has a
// brand-new device that only the recovery key can re-sign.
//
// Set CROSS_SIGNING_REGENERATE=1 to deliberately reset the identity (costs one
// re-verify per user, and stores the fresh key). CROSS_SIGNING_RECOVERY_KEY
// imports an existing key into the store, for adopting an identity created before
// the bot persisted its own.
func bootstrapCrossSigning(ctx context.Context, mach *crypto.OlmMachine, dataDir string) {
	stored := loadRecoveryKey(dataDir)
	if envKey := os.Getenv("CROSS_SIGNING_RECOVERY_KEY"); envKey != "" && envKey != stored {
		saveRecoveryKey(dataDir, envKey)
		stored = envKey
	}

	hasKeys, isVerified, err := mach.GetOwnVerificationStatus(ctx)
	if err != nil {
		slog.Warn("cross-signing: could not determine verification status, leaving identity alone", "err", err)
		return
	}
	regenerate := os.Getenv("CROSS_SIGNING_REGENERATE") != ""

	switch {
	case regenerate || !hasKeys:
		if regenerate && hasKeys {
			slog.Warn("cross-signing: CROSS_SIGNING_REGENERATE set — replacing the published identity; every user must verify the bot once more. Unset it after this start.")
		}
		key, _, err := mach.GenerateAndUploadCrossSigningKeys(ctx, uiaSession, "")
		if err != nil {
			slog.Warn("cross-signing: key upload failed, bot will show unverified", "err", err)
			return
		}
		saveRecoveryKey(dataDir, key)
		if err := mach.SignOwnDevice(ctx, mach.OwnIdentity()); err != nil {
			slog.Warn("cross-signing: sign own device failed", "err", err)
		}
		if err := mach.SignOwnMasterKey(ctx); err != nil {
			slog.Warn("cross-signing: sign master key failed", "err", err)
		}
		slog.Info("cross-signing: identity created and device signed")

	case isVerified:
		slog.Info("cross-signing: identity already published and this device is signed")

	case stored != "":
		// Pulls the private keys back out of SSSS, then signs this device and the
		// master key with them.
		if err := mach.VerifyWithRecoveryKey(ctx, stored); err != nil {
			slog.Warn("cross-signing: recovery-key restore failed, bot will show unverified", "err", err)
			return
		}
		slog.Info("cross-signing: device re-signed from the stored recovery key")

	default:
		slog.Warn("cross-signing: this device is unsigned and no recovery key is stored, so the bot will show unverified (E2EE still works). Set CROSS_SIGNING_REGENERATE=1 once to mint a fresh identity.",
			"store", crossSigningPath(dataDir))
	}
}
