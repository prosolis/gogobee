# Spec: Space Inviter — `space_inviter.go`

## Overview

When a local homeserver user is detected as not being a member of **any** Matrix Space
(i.e. they aren't in the target Space *and* aren't in any other Space the bot can see them in),
the bot should **DM the admin user** and ask whether to invite them to the target Space,
rather than auto-inviting.

The intent: only prompt for users who are completely Space-less. If they're already in some
other Space, leave them alone — they've made a choice.

---

## Config values required

All values sourced from the existing GogoBee config struct. No hardcoding.

| Key | Type | Description |
|---|---|---|
| `target_space_id` | `id.RoomID` | The Space to potentially invite users into |
| `home_domain` | `string` | e.g. `parodia.dev` — used to filter out federated users |
| `synapse_base_url` | `string` | e.g. `https://parodia.dev` — for admin API calls |
| `admin_access_token` | `string` | Synapse admin token — store in secrets |
| `admin_user_id` | `id.UserID` | The user to DM with invite prompts (i.e. the operator) |
| `invite_delay` | `time.Duration` | Delay between prompts during startup sweep (default: 2s) |
| `reprompt_cooldown` | `time.Duration` | Minimum time between prompts for the same user (default: 30 days). Prevents restart/upgrade ping-spam for users who already said `no`. |

---

## Trigger conditions

### 1. Startup sweep
- On bot startup, after permission checks pass, fetch all local non-guest non-deactivated users via the Synapse Admin API (`/_synapse/admin/v2/users`)
- Pass `?guests=false&deactivated=false` and paginate fully
- Apply the user filter (see "User filter" below) before any Space-membership work
- For each remaining user, determine whether they belong to **any** Space (see "Space membership check" below)
- For each user who is in zero Spaces and not on the blocklist and has no pending invite to `target_space_id`: queue a DM prompt to the admin user
- Stagger prompts using `invite_delay` to avoid flooding the admin DM

### 2. Reactive handler
- Listen for `m.room.member` events with `membership: join`
- Apply the user filter (see "User filter" below)
- If the joining user is in zero Spaces, not on the blocklist, has no pending invite to `target_space_id`, and a prompt hasn't already been sent for them this session: send a DM prompt to the admin user

---

## User filter

A user is eligible for a Space-less prompt only if **all** of the following hold. Apply this
filter in both the startup sweep and the reactive handler — same rules, one place in code.

1. Local to this homeserver (user ID suffix matches `:home_domain`). Federated users are out of scope.
2. Not the bot itself.
3. Not the configured `admin_user_id`.
4. Not an appservice / bridge puppet. Synapse `/admin/v2/users` returns a `user_type` field — exclude entries where `user_type` is non-empty (typical values: `bot`, `support`, and appservice puppets often appear with non-empty types or with names matching the appservice's `sender_localpart`). Also exclude users whose localpart matches any configured appservice `sender_localpart` if available.
5. Not a guest, not deactivated (already filtered at the API level, but re-check defensively for the reactive path).

---

## Space membership check

**Operational invariant: the bot is joined to every Space on the homeserver.** The spec
relies on this — if a Space exists that the bot isn't in, users in only that Space will be
incorrectly flagged as Space-less.

A user counts as "in a Space" if they are joined or invited to **any** room of type `m.space`
(i.e. `m.room.create` content has `"type": "m.space"`).

- On startup (and refreshed as the bot joins/leaves Spaces), enumerate the bot's joined rooms via the mautrix client and filter to those with `m.room.create.type == m.space` — this yields the set of Spaces to check against
- For each Space, fetch its members via `JoinedMembers` (joined) and the room state for `m.room.member` events with `membership: invite` (pending invites)
- Build an in-memory union set: `users_in_any_space = ⋃ members(space)` for all known Spaces
- A local user is "Space-less" iff they are absent from this union set
- Refresh the union set at the start of each sweep; for the reactive handler, update incrementally on `m.room.member` events in Space rooms

No admin API call is needed for the Space-membership check itself. The admin API is still
required to enumerate local users (to discover users who are in zero rooms the bot can see).

---

## DM room bootstrap

Before sending any prompt, the bot must have a 1:1 DM room with `admin_user_id`.

- On startup, look up the bot's `m.direct` account data
- If an existing DM room with `admin_user_id` is listed and the bot is still joined to it, reuse it
- Otherwise create a new room: invite-only, `is_direct: true`, invite `admin_user_id`, and update `m.direct` account data to record the mapping
- Cache the resolved DM room ID in memory for the session
- If the admin leaves the DM room, treat the next prompt as a trigger to recreate it

---

## DM prompt format

Sent as a plain text message to the admin user's DM room:

```
🐝 @someuser:parodia.dev just joined but isn't in the Space.
Want me to invite them?

Reply with:
  yes    — invite them
  no     — skip for now (I'll ask again later)
  ignore — never ask about this user again
```

Use the existing GogoBee pattern for interactive admin responses (reply matching or
reaction handling — match whatever pattern is already established in the codebase).

---

## Response handling

| Reply | Action |
|---|---|
| `yes` (case-insensitive) | Invite the user to the Space identified by `target_space_id` (always — never any other Space), log success or failure |
| `no` (case-insensitive) | Skip. Will prompt again on next sweep or next join event |
| `ignore` (case-insensitive) | Add user to persistent blocklist, never prompt again |

- Only one pending prompt per user at a time — deduplication required
- If the admin doesn't respond, do nothing; prompt again on next trigger
- **Re-validate at reply time, not prompt time.** When the admin replies `yes`, re-check that the user is still Space-less and still has no pending invite to `target_space_id`. If they joined a Space (or were already invited) between prompt and reply, skip the invite and DM the admin a one-liner: `ℹ️ @user:domain is already in a Space — skipped.`
- **Unknown replies are silently ignored.** If the admin types anything other than `yes`/`no`/`ignore` (case-insensitive, leading/trailing whitespace stripped) in the DM room while a prompt is pending, the bot does not respond, does not error, does not re-send help. The pending prompt remains pending until a valid reply arrives or the bot restarts.
- If the user already has a pending invite to `target_space_id` at reply time (e.g. someone else invited them out-of-band), treat `yes` as a no-op and log it.
- If an invite attempt fails with `M_FORBIDDEN`, log an error loudly and DM the admin:
  > 🚨 Tried to invite @someuser:parodia.dev but got M_FORBIDDEN.
  > My power level may have dropped. Check the Space permissions.
- For any other invite failure (rate-limit, network, etc.), log the full error and DM the admin a short notice; do not auto-retry — the next sweep will catch it.

---

## Permission check (on startup, before sweep)

Abort with a fatal error and a clear message if any of the following are true:

1. Bot is not joined to `target_space_id` or cannot read its membership
2. Bot cannot read the target Space's power levels
3. Bot's power level in the target Space is below the `invite` threshold
4. `admin_access_token` is empty
5. Admin API returns non-200 (log status code and response body)
6. **Sanity check**: log a warning (not fatal) listing every Space the bot is currently joined to, so the operator can eyeball whether any Space is missing. If the bot is in zero Spaces other than the target, log a louder warning — the "in any Space" check will degenerate to "in target Space".

Error messages should be specific about what's wrong and what needs to be fixed.
Do not silently swallow these — they should halt startup.

---

## Persistence — `space_inviter_prompts` table

All prompt activity is persisted in a single SQLite table. This replaces both the
flat-file blocklist and the in-memory dedup set with one durable source of truth.

```sql
CREATE TABLE IF NOT EXISTS space_inviter_prompts (
    user_id           TEXT NOT NULL,           -- @user:domain
    prompt_sent_at    INTEGER NOT NULL,        -- unix seconds
    trigger_kind      TEXT NOT NULL,           -- 'sweep' | 'reactive'
    dm_room_id        TEXT NOT NULL,           -- room the prompt was sent in
    dm_event_id       TEXT NOT NULL,           -- event ID of the prompt message
    reply             TEXT,                    -- 'yes' | 'no' | 'ignore' | NULL (pending)
    replied_at        INTEGER,                 -- unix seconds, NULL if pending
    invite_attempted  INTEGER NOT NULL DEFAULT 0,  -- 0/1
    invite_outcome    TEXT,                    -- 'ok' | 'forbidden' | 'error' | 'skipped_race' | NULL
    invite_error      TEXT,                    -- raw error string for diagnostics
    PRIMARY KEY (user_id, prompt_sent_at)
);
CREATE INDEX IF NOT EXISTS idx_space_inviter_prompts_user ON space_inviter_prompts(user_id);
CREATE INDEX IF NOT EXISTS idx_space_inviter_prompts_pending ON space_inviter_prompts(user_id) WHERE reply IS NULL;
```

### Lifecycle

- **On prompt sent**: insert a new row with `reply = NULL`.
- **On reply received**: update the most-recent pending row for that user — set `reply`, `replied_at`. Match the reply to the pending prompt using the `m.relates_to` reply reference to `dm_event_id` if the admin uses Matrix replies; otherwise just match the most-recent pending row in the DM.
- **On invite attempt**: update the same row — set `invite_attempted = 1`, `invite_outcome`, and `invite_error` if applicable.

### Querying

- **Is user blocked (`ignore`)?** `SELECT 1 FROM space_inviter_prompts WHERE user_id = ? AND reply = 'ignore' LIMIT 1`
- **Has a pending prompt?** `SELECT 1 FROM space_inviter_prompts WHERE user_id = ? AND reply IS NULL LIMIT 1`
- **Has been prompted this session?** Replaced by "has any row at all" — but the eligibility check is "no pending prompt AND not on ignore list AND (not yet ever prompted OR last reply was `no` and we want to re-prompt on next trigger)". `no` replies don't block re-prompting.

### Dedup rules (in DB terms)

A user is **not eligible** for a new prompt if any of these are true:

1. A pending row exists (`reply IS NULL`) — already awaiting an answer.
2. Any row exists with `reply = 'ignore'` — permanently silenced.
3. **Cooldown**: any row exists with `prompt_sent_at > (now - reprompt_cooldown)` — already prompted recently, regardless of reply. This is the rule that prevents restart/upgrade ping-spam: a `no` from yesterday does not get re-asked tomorrow just because the bot restarted.

A `no` reply does eventually allow re-prompting, but only after `reprompt_cooldown` has elapsed since the last prompt. Default 30 days — long enough that the admin won't feel nagged across normal ops, short enough that genuinely new context (user has been Space-less for a month) gets surfaced.

### Eligibility query

```sql
-- Returns 1 row if user IS eligible for a new prompt
SELECT 1
WHERE NOT EXISTS (
    SELECT 1 FROM space_inviter_prompts
    WHERE user_id = ?
      AND (
          reply IS NULL                              -- pending
          OR reply = 'ignore'                        -- blocked forever
          OR prompt_sent_at > ?                      -- within cooldown window (now - reprompt_cooldown)
      )
);
```

### Un-ignore

- `UPDATE space_inviter_prompts SET reply = 'no' WHERE user_id = ? AND reply = 'ignore'` via a sqlite shell, or a small admin command. v1 documents the SQL one-liner; in-band command can come later.

### Schema migration

- Apply via the existing GogoBee migration mechanism. New table, no data migration from any prior store.

---

## Logging

At INFO level (one structured log line per event):

- Startup sweep begin/end with counts: total local users, eligible after filter, Space-less, prompts queued
- Each prompt sent: `event=prompt_sent user=@x:domain trigger=sweep|reactive`
- Each reply received: `event=prompt_reply user=@x:domain reply=yes|no|ignore|unknown`
- Each invite attempt: `event=invite_attempt user=@x:domain space=<target> outcome=ok|forbidden|error err=...`
- DM room bootstrap: `event=dm_room_resolved room=<id> created=true|false`

At WARN level: skipped invites due to race (user joined a Space between prompt and reply), invite failures other than M_FORBIDDEN.

At ERROR level: M_FORBIDDEN on invite, admin API non-200, fatal startup permission failures.

---

## Out of scope

- Auto-accepting join requests
- Inviting federated users from other homeservers
- Any UI beyond DM text messages
