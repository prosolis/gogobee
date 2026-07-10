package bot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := loadRecoveryKey(dir); got != "" {
		t.Fatalf("fresh install returned %q, want empty", got)
	}

	const key = "EsTd o49d mMLt 3Uf9 Gjn9 x5fv YE9H wF6n aadC q2D8 Fv7j rQ4c"
	saveRecoveryKey(dir, key)
	if got := loadRecoveryKey(dir); got != key {
		t.Fatalf("loadRecoveryKey() = %q, want %q", got, key)
	}
}

// The key is crypto material sitting next to crypto.db; it must not be readable
// by other users on the host.
func TestRecoveryKeyFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	saveRecoveryKey(dir, "some-key")
	info, err := os.Stat(filepath.Join(dir, crossSigningFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("recovery key file mode = %o, want 600", perm)
	}
}

// A corrupt store must degrade to "no key" rather than panicking or returning
// garbage that would be fed to VerifyWithRecoveryKey.
func TestRecoveryKeyCorruptStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, crossSigningFile), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := loadRecoveryKey(dir); got != "" {
		t.Fatalf("corrupt store returned %q, want empty", got)
	}
}

// Overwriting an adopted key must not leave trailing bytes from the longer value.
func TestRecoveryKeyOverwrite(t *testing.T) {
	dir := t.TempDir()
	saveRecoveryKey(dir, "a-much-longer-original-recovery-key-value")
	saveRecoveryKey(dir, "short")
	if got := loadRecoveryKey(dir); got != "short" {
		t.Fatalf("loadRecoveryKey() = %q, want %q", got, "short")
	}
}
