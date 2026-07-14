# GogoBee → Appservice Auth Migration (multi-session)

Goal: move TwinBee's Matrix auth from the **MAS OAuth 2.0 device grant** (working
stopgap) to the **full appservice transaction/push model** — the MAS-durable end
state: no human login, no MFA, no consent, `as_token` never expires, scales at
485+ rooms. The device grant stays behind `AUTH_MODE=masdevice` as instant
rollback.

Why the *previous* appservice attempt failed: it was a hybrid (as_token + `/sync`),
and Synapse forbids appservice-namespace users from `/sync`. This migration
replaces `/sync` with Synapse→bot transaction push + MSC3202/MSC2409 E2EE
extensions, which is the supported path in mautrix v0.28.1.

Keep this doc working-tree only — do NOT commit (see feedback_dont_commit_plan_mds).

## Key facts established (investigation, 2026-07-03)

- **HEAD 20d1f92 is MISLABELED**: commit msg says "device grant" but it only
  deleted registration.yaml.example; committed client.go is still the FAILED
  appservice+/sync hybrid. The real device-grant (masauth.go + client.go rewrite)
  is UNCOMMITTED working tree — that's what runs in prod. Backed up to scratchpad.
- mautrix v0.28.1 fully supports appservice E2EE: transaction carries
  `to_device`/`device_lists`/`device_one_time_keys_count` (stable + msc2409/msc3202
  unstable field names); `CreateDeviceMSC4190`; cryptohelper `MSC4190=true` mints
  the device (no /login). HEAD's old appservice code already had auth+crypto
  correct — only the /sync event loop was wrong.
- To-device processing gates on `Registration.EphemeralEvents` (yaml
  `receive_ephemeral: true`) — `appservice/http.go:133`.
- Encrypted rooms: must feed `m.room.encryption`/`m.room.member` to the client's
  StateStore via `mautrix.UpdateStateStore(...)` + `mach.HandleMemberEvent`, or the
  bot sends plaintext into encrypted rooms / can't share keys.
- Both hosts SSH-reachable: `reala@parodia.dev` (Synapse/MAS/Authentik docker),
  `reala@192.168.1.212` (millenia, bot in screen). Retired registration at
  `/home/reala/matrix/compose/synapse/gogobee-registration.yaml.retired-20260703-223425`.

## Decisions (user, 2026-07-03)

- Cutover: **AUTH_MODE flag** (keep device grant as rollback), not full replace.
- Synapse changes: **I apply + restart, confirm right before restart**.

## Session 1 — bot-side code (local, no prod risk)  ✅ DONE (build+vet green)

- [x] `Config`: AuthMode + appservice fields (RegistrationPath, ListenHost/Port,
      HomeserverDomain).
- [x] `internal/bot/session.go`: `Session` unifying both modes (OnEventType/Run/
      Stop); `NewSession(cfg)` dispatches on AuthMode; masdevice path wraps existing
      device-grant NewClient + DefaultSyncer + sync loop (verbatim).
- [x] `internal/bot/appservice.go`: LoadRegistration → CreateFull → BotClient;
      cryptohelper MSC4190 device create; ep.OnOTK/OnDeviceList + 9 crypto to-device
      types → crypto machine; EventEncrypted decrypt+redispatch; StateStore
      population (encryption + members) via lazy per-room resolveRoom + UpdateStateStore
      on member/encryption state; HTTP listener via as.Start() in goroutine.
- [x] `main.go`: NewSession + sess.OnEventType + sess.Run; env AUTH_MODE,
      AS_REGISTRATION, AS_LISTEN_HOST, AS_LISTEN_PORT, HOMESERVER_DOMAIN; envInt helper.
- [x] `.env.example` documented; go.mod tidied (zerolog now direct dep).
- [x] `go build`/`go vet` green (CGO=1 -tags goolm).

New files: internal/bot/session.go, internal/bot/appservice.go. NOT yet committed.

### Known correctness risks to verify in Session 3
- **resolveRoom is lazy on inbound encrypted events only.** Scheduled/broadcast
  posts into an encrypted room the bot hasn't received from yet would send
  plaintext (StateStore unresolved). Mitigation for S3: prewarm the configured
  rooms (BROADCAST_ROOMS, GAMES_ROOM, ESTEEMED_ROOM, MOD_ADMIN_ROOM, MINIFLUX_*)
  at startup, or resolve all joined rooms once. Interactive `!` commands are
  reply-driven so lazy resolve covers them.
- Whether crypto machine successfully shares keys after ReplaceCachedMembers +
  device fetch (FetchKeys) — the make-or-break, only testable against live Synapse.
- MSC4190 device creation must succeed against MAS-fronted Synapse (untested end
  to end; HEAD's old code used the same path but was never verified past whoami).

## Session 2 — Synapse infra on parodia (touches live homeserver)

- [ ] Restore gogobee-registration.yaml: set `url:` = bot tailnet addr, add
      `receive_ephemeral: true`, `org.matrix.msc3202: true`, keep
      `io.element.msc4190: true`, `rate_limited: false`.
- [ ] homeserver.yaml `experimental_features`: msc2409_to_device_messages_enabled,
      msc3202_device_masquerading, msc3202_transaction_extensions,
      msc4190_device_management. Wire `app_service_config_files`.
- [ ] Determine bot's headscale tailnet address on parodia (Synapse must reach the
      bot's ListenPort). Confirm routing.
- [ ] Backup homeserver.yaml + registration; **confirm before restart**; restart
      matrix-synapse; check it comes back for all users.

## Session 3 — cutover + E2EE verification on millenia

- [ ] Copy registration to bot host; set AUTH_MODE=appservice + AS_* env.
- [ ] rsync source + build in place (CGO=1 -tags goolm), restart screen session.
      Remove empty local data/gogobee.db first (empty_local_db_wipes_prod).
- [ ] Verify: transactions arrive (listener logs), whoami OK, MSC4190 device made,
      bot responds to `!` command in a PLAINTEXT room.
- [ ] Verify E2EE: bot decrypts an incoming encrypted-room message AND its reply
      decrypts for a human client (encrypt-out works). This is the make-or-break.
- [ ] If broken: AUTH_MODE=masdevice, restart → back on device grant. Then triage.

## Cleanup (after proven)

- Prune stale @twinbee devices, orphan MAS client, retire masauth.go or leave
  behind the flag. Fix HEAD mislabel (the eventual commit should actually contain
  device-grant + appservice, not the stale hybrid).
