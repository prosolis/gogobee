import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { DateTime, IANAZone } from "luxon";
import logger from "../utils/logger";

const STATUS_EMOJI: Record<string, string> = {
  online: "\u{1F7E2}",
  away: "\u{1F7E1}",
  afk: "\u{1F534}",
};

export class PresencePlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "presence";
  }

  get commands(): CommandDef[] {
    return [
      { name: "away", description: "Set status to away", usage: "!away [message]" },
      { name: "afk", description: "Set status to AFK", usage: "!afk [message]" },
      { name: "back", description: "Set status back to online", usage: "!back" },
      { name: "whois", description: "Show user profile card", usage: "!whois @user" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    const db = getDb();

    // Passive: auto-clear away/afk on any message
    db.prepare(
      `UPDATE presence SET status = 'online', status_message = NULL, updated_at = unixepoch()
       WHERE user_id = ? AND room_id = ? AND status IN ('away', 'afk')`
    ).run(ctx.sender, ctx.roomId);

    // Passive: always upsert updated_at for last seen
    db.prepare(
      `INSERT INTO presence (user_id, room_id, status, updated_at)
       VALUES (?, ?, 'online', unixepoch())
       ON CONFLICT(user_id, room_id) DO UPDATE SET updated_at = unixepoch()`
    ).run(ctx.sender, ctx.roomId);

    if (this.isCommand(ctx.body, "away")) {
      await this.handleAway(ctx);
    } else if (this.isCommand(ctx.body, "afk")) {
      await this.handleAfk(ctx);
    } else if (this.isCommand(ctx.body, "back")) {
      await this.handleBack(ctx);
    } else if (this.isCommand(ctx.body, "whois")) {
      await this.handleWhois(ctx);
    }
  }

  private async handleAway(ctx: MessageContext): Promise<void> {
    const message = this.getArgs(ctx.body, "away") || null;
    const db = getDb();

    db.prepare(
      `INSERT INTO presence (user_id, room_id, status, status_message, updated_at)
       VALUES (?, ?, 'away', ?, unixepoch())
       ON CONFLICT(user_id, room_id) DO UPDATE SET status = 'away', status_message = excluded.status_message, updated_at = unixepoch()`
    ).run(ctx.sender, ctx.roomId, message);

    const reply = message
      ? `${ctx.sender} is now away: ${message}`
      : `${ctx.sender} is now away.`;
    await this.sendMessage(ctx.roomId, reply);
  }

  private async handleAfk(ctx: MessageContext): Promise<void> {
    const message = this.getArgs(ctx.body, "afk") || null;
    const db = getDb();

    db.prepare(
      `INSERT INTO presence (user_id, room_id, status, status_message, updated_at)
       VALUES (?, ?, 'afk', ?, unixepoch())
       ON CONFLICT(user_id, room_id) DO UPDATE SET status = 'afk', status_message = excluded.status_message, updated_at = unixepoch()`
    ).run(ctx.sender, ctx.roomId, message);

    const reply = message
      ? `${ctx.sender} is now AFK: ${message}`
      : `${ctx.sender} is now AFK.`;
    await this.sendMessage(ctx.roomId, reply);
  }

  private async handleBack(ctx: MessageContext): Promise<void> {
    const db = getDb();

    db.prepare(
      `INSERT INTO presence (user_id, room_id, status, status_message, updated_at)
       VALUES (?, ?, 'online', NULL, unixepoch())
       ON CONFLICT(user_id, room_id) DO UPDATE SET status = 'online', status_message = NULL, updated_at = unixepoch()`
    ).run(ctx.sender, ctx.roomId);

    await this.sendMessage(ctx.roomId, `${ctx.sender} is back online.`);
  }

  private async handleWhois(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "whois").trim();
    const targetUser = args.split(/\s/)[0];

    if (!targetUser || !targetUser.startsWith("@")) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !whois @user:server");
      return;
    }

    const db = getDb();

    // Presence data
    const presence = db
      .prepare(`SELECT status, status_message, updated_at FROM presence WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { status: string; status_message: string | null; updated_at: number } | undefined;

    const status = presence?.status ?? "online";
    const statusMessage = presence?.status_message;
    const updatedAt = presence?.updated_at;

    // User data
    const user = db
      .prepare(`SELECT xp, level, rep, timezone FROM users WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { xp: number; level: number; rep: number; timezone: string | null } | undefined;

    // Streak data
    const stats = db
      .prepare(`SELECT current_streak FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { current_streak: number } | undefined;

    // Now playing
    const playing = db
      .prepare(`SELECT game FROM now_playing WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { game: string } | undefined;

    // Build profile card
    const lines: string[] = [targetUser];

    // Status line
    const emoji = STATUS_EMOJI[status] ?? STATUS_EMOJI.online;
    const statusLabel = status.charAt(0).toUpperCase() + status.slice(1);
    let statusLine = `Status: ${emoji} ${statusLabel}`;
    if (statusMessage) statusLine += ` (${statusMessage})`;
    lines.push(statusLine);

    // Last seen
    if (updatedAt) {
      lines.push(`Last seen: ${this.formatLastSeen(updatedAt)}`);
    }

    // Timezone
    if (user?.timezone) {
      const zone = IANAZone.create(user.timezone);
      if (zone.isValid) {
        const localTime = DateTime.now().setZone(zone).toFormat("HH:mm");
        lines.push(`Timezone: ${user.timezone} (${localTime} local)`);
      }
    }

    // Now playing
    if (playing?.game) {
      lines.push(`Now playing: ${playing.game}`);
    }

    // Rep & Level
    const rep = user?.rep ?? 0;
    const level = user?.level ?? 0;
    lines.push(`Reputation: ${rep} | Level: ${level}`);

    // Streak
    const streak = stats?.current_streak ?? 0;
    lines.push(`Streak: ${streak} days`);

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private formatLastSeen(updatedAt: number): string {
    const nowSeconds = Math.floor(Date.now() / 1000);
    const diffSeconds = nowSeconds - updatedAt;

    if (diffSeconds < 60) return "just now";

    const diffMinutes = Math.floor(diffSeconds / 60);
    if (diffMinutes < 60) return `${diffMinutes} minute${diffMinutes !== 1 ? "s" : ""} ago`;

    const diffHours = Math.floor(diffMinutes / 60);
    if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? "s" : ""} ago`;

    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays} day${diffDays !== 1 ? "s" : ""} ago`;
  }
}
