package plugin

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// TestResolvePetName_ThroughDispatcher is a regression test for the bug where
// resolvePendingInteraction deleted the pending entry before dispatch, while
// resolvePetName then tried to LoadAndDelete it again — always missing, so the
// adoption silently no-op'd and no pet was ever persisted. The fix passes the
// already-loaded interaction into resolvePetName. This test drives the real
// dispatcher path (resolvePendingInteraction) to lock that in.
func TestResolvePetName_ThroughDispatcher(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@pet-name-dispatch:example")
	if err := createAdvCharacter(uid, "petnamer"); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	interaction := &advPendingInteraction{
		Type:      "pet_name",
		Data:      &advPendingPetName{PetType: "dog"},
		ExpiresAt: time.Now().Add(time.Hour),
	}
	p.pending.Store(string(uid), interaction)

	if err := p.resolvePendingInteraction(MessageContext{Sender: uid, Body: "Pepper"}, interaction); err != nil {
		t.Fatal(err)
	}

	pet, err := loadPetState(uid)
	if err != nil {
		t.Fatal(err)
	}
	if !pet.HasPet() {
		t.Fatalf("expected pet to be adopted; got %+v", pet)
	}
	if pet.Name != "Pepper" {
		t.Errorf("pet name = %q, want Pepper", pet.Name)
	}
	if pet.Type != "dog" {
		t.Errorf("pet type = %q, want dog", pet.Type)
	}
	if !pet.Arrived || pet.Level != 1 {
		t.Errorf("pet arrived=%v level=%d, want true/1", pet.Arrived, pet.Level)
	}
}
