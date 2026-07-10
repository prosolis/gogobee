package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// registeredSecretNodes collects every NodeKindSecret node across all
// registered zone graphs, keyed by node ID.
func registeredSecretNodes(t *testing.T) map[string]ZoneNode {
	t.Helper()
	out := map[string]ZoneNode{}
	for _, z := range allZones() {
		g, ok := loadZoneGraph(z.ID)
		if !ok {
			continue
		}
		for id, n := range g.Nodes {
			if n.Kind == NodeKindSecret {
				out[id] = n
			}
		}
	}
	return out
}

// TestSecretRoomDiscovery_EverySecretHasBespokeFlavor guards that no secret
// node ships on the generic fallback — every one gets an authored line. Catches
// a future secret room added without a discovery entry.
func TestSecretRoomDiscovery_EverySecretHasBespokeFlavor(t *testing.T) {
	for id, n := range registeredSecretNodes(t) {
		if _, ok := secretRoomDiscovery[id]; !ok {
			t.Errorf("secret node %q (%q) has no bespoke discovery line", id, n.Label)
		}
		if line := secretRoomDiscoveryLine(n); strings.TrimSpace(line) == "" {
			t.Errorf("secret node %q produced an empty discovery line", id)
		}
	}
}

func TestSecretRoomDiscoveryLine_FallsBackOnUnknownNode(t *testing.T) {
	n := ZoneNode{NodeID: "made_up.node", Kind: NodeKindSecret, Label: "Nowhere Nook"}
	line := secretRoomDiscoveryLine(n)
	if !strings.Contains(line, "Nowhere Nook") {
		t.Errorf("fallback should name the node label, got: %s", line)
	}
}

// TestSecretRoomKeys_UnlockRealDestinationEdges is the cross-zone-key
// consistency check: every key's item Name must match a LockKey edge's key_id
// (lower-cased) in its destination zone, and every source must be a real secret
// room. A typo between the granted item name and the authored key_id would
// silently make a vault permanently unreachable; this catches it.
func TestSecretRoomKeys_UnlockRealDestinationEdges(t *testing.T) {
	secrets := registeredSecretNodes(t)
	for srcNode, key := range secretRoomKeys {
		if _, ok := secrets[srcNode]; !ok {
			t.Errorf("key source %q is not a registered secret room", srcNode)
		}
		g, ok := loadZoneGraph(key.unlocksIn)
		if !ok {
			t.Errorf("%s: destination zone %q has no graph", srcNode, key.unlocksIn)
			continue
		}
		wantKeyID := strings.ToLower(key.item.Name)
		found := false
		for _, outs := range g.Edges {
			for _, e := range outs {
				if e.Lock == LockKey && strings.ToLower(lockDataString(e.LockData, "key_id")) == wantKeyID {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("key %q (from %s) has no LockKey edge with key_id=%q in zone %q — vault unreachable",
				key.item.Name, srcNode, wantKeyID, key.unlocksIn)
		}
	}
}

// TestCrossZoneKey_UnlocksWithItemNotWithout exercises the runtime lock
// evaluation end-to-end on a real authored edge.
func TestCrossZoneKey_UnlocksWithItemNotWithout(t *testing.T) {
	g, ok := loadZoneGraph(ZoneManorBlackspire)
	if !ok {
		t.Fatal("manor graph missing")
	}
	var keyEdge ZoneEdge
	for _, e := range g.outgoingEdges("manor_blackspire.upper_hall") {
		if e.Lock == LockKey {
			keyEdge = e
		}
	}
	if keyEdge.To == "" {
		t.Fatal("no LockKey edge off manor upper_hall")
	}

	base := edgeUnlockCtx{RunID: "r", FromNode: "manor_blackspire.upper_hall"}
	if ok, _ := evaluateEdgeLock(keyEdge, base); ok {
		t.Error("edge should be locked without the key in inventory")
	}

	withKey := base
	withKey.InventoryNames = map[string]bool{"sunken sigil": true}
	if ok, reason := evaluateEdgeLock(keyEdge, withKey); !ok {
		t.Errorf("edge should unlock with the sigil in inventory, got locked: %s", reason)
	}
}

// TestGrantSecretRoomKey_GrantsOnceIdempotent verifies a key-bearing secret
// hands its key over exactly once.
func TestGrantSecretRoomKey_GrantsOnceIdempotent(t *testing.T) {
	townTestDB(t)
	p := &AdventurePlugin{}
	u := id.UserID("@keyholder:test.invalid")

	node := ZoneNode{NodeID: "sunken_temple.coral_reliquary", Kind: NodeKindSecret}
	line := p.grantSecretRoomKey(u, node)
	if !strings.Contains(line, "Sunken Sigil") {
		t.Fatalf("first grant should name the key, got: %q", line)
	}

	items, err := loadAdvInventory(u)
	if err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	sigils := 0
	for _, it := range items {
		if strings.EqualFold(it.Name, "Sunken Sigil") {
			sigils++
		}
	}
	if sigils != 1 {
		t.Fatalf("want exactly 1 Sunken Sigil, got %d", sigils)
	}

	// Second find on a later run must not stack a duplicate.
	if again := p.grantSecretRoomKey(u, node); again != "" {
		t.Errorf("re-finding the room should not re-grant the key, got: %q", again)
	}
	items, _ = loadAdvInventory(u)
	sigils = 0
	for _, it := range items {
		if strings.EqualFold(it.Name, "Sunken Sigil") {
			sigils++
		}
	}
	if sigils != 1 {
		t.Fatalf("still want exactly 1 Sunken Sigil after re-find, got %d", sigils)
	}
}

// TestGrantSecretRoomKey_NoKeyForPlainSecret confirms a secret room that carries
// no cross-zone key stays silent.
func TestGrantSecretRoomKey_NoKeyForPlainSecret(t *testing.T) {
	townTestDB(t)
	p := &AdventurePlugin{}
	u := id.UserID("@plain:test.invalid")
	node := ZoneNode{NodeID: "forest_shadows.sapling_shrine", Kind: NodeKindSecret}
	if line := p.grantSecretRoomKey(u, node); line != "" {
		t.Errorf("plain secret should grant no key, got: %q", line)
	}
}

// TestResolveSecretRoom_GrantsPageCacheAndKey checks the full treasure-cache
// payout: a journal page, the consumable cache, and (for a key-bearing secret)
// the cross-zone key — with no combat touching HP.
func TestResolveSecretRoom_GrantsPageCacheAndKey(t *testing.T) {
	townTestDB(t)
	p := &AdventurePlugin{}
	u := id.UserID("@secret:test.invalid")

	zone, ok := getZone(ZoneSunkenTemple)
	if !ok {
		t.Fatal("sunken temple zone missing")
	}
	node := ZoneNode{
		NodeID: "sunken_temple.coral_reliquary", Kind: NodeKindSecret,
		Label:   "Coral Reliquary",
		Content: ZoneNodeContent{LootBias: 1.8},
	}
	run := &DungeonRun{UserID: string(u), ZoneID: ZoneSunkenTemple, CurrentNode: node.NodeID}

	out := p.resolveSecretRoom(u, run, zone, node)
	if !strings.Contains(out, "Coral Reliquary") {
		t.Errorf("outcome should carry the discovery line:\n%s", out)
	}

	// A journal page was granted (fresh player, nothing found yet).
	if mask, err := loadJournalPages(u); err != nil || journalPageCount(mask) != 1 {
		t.Fatalf("want exactly 1 journal page granted, got count=%d err=%v", journalPageCount(mask), err)
	}

	// The guaranteed cache + the cross-zone key are in inventory.
	items, err := loadAdvInventory(u)
	if err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	var caches, keys int
	for _, it := range items {
		switch {
		case it.Type == "key":
			keys++
		case it.Type == "consumable":
			caches++
		}
	}
	if caches != secretRoomCacheCount {
		t.Errorf("want %d cache consumables, got %d", secretRoomCacheCount, caches)
	}
	if keys != 1 {
		t.Errorf("want the Sunken Sigil key granted, got %d keys", keys)
	}
}
