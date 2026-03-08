import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { xpToLevel, xpForNextLevel, progressBar } from "../utils/parser";
import logger from "../utils/logger";

const XP_PER_MESSAGE = 10;
const XP_COOLDOWN_MS = 30_000;

const LEVEL_UP_MESSAGES = [
  "🔔✨ POWER UP! {user} hit Level {level}! Bell combo complete! ✨🔔",
  "🌟 {user} grabbed the golden bell — Level {level}! Speed up! 🐝💨",
  "⚡ Level {level}! {user} is fully powered up! Options attached! 🛸🛸",
  "🎮 {user} evolved to Level {level}! Twin attack ready! 👊👊",
  "🏆 STAGE CLEAR! {user} advanced to Level {level}! 🎆🎆🎆",
  "🔔🔔🔔 {user} collected enough bells for Level {level}! Shield activated! 🛡️",
  "📢 Level {level} unlocked! {user} picked up the megaphone power-up! 🔊💥",
  "⚡ {user} just warped to Level {level}! Laser mode engaged! 🔴🔴🔴",
  "🎰 Bonus round! {user} reached Level {level}! 3-way shot acquired! 🔱",
  "⚡ {user} powered through to Level {level}! Ripple laser online! 〰️💫",
];

export class XpPlugin extends Plugin {
  private cooldowns = new Map<string, number>();

  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "xp";
  }

  get commands(): CommandDef[] {
    return [
      { name: "rank", description: "Show your XP, level, and progress", usage: "!rank [@user]" },
      { name: "leaderboard", description: "Top users by XP", usage: "!leaderboard [n]" },
    ];
  }

  /**
   * Public method so other plugins (rep, wotd) can grant bonus XP.
   */
  grantXp(userId: string, roomId: string, amount: number, reason: string): void {
    const db = getDb();

    // Grab old level before granting
    const before = db.prepare(`SELECT level FROM users WHERE user_id = ? AND room_id = ?`).get(userId, roomId) as
      | { level: number }
      | undefined;
    const oldLevel = before?.level ?? 0;

    db.prepare(`
      INSERT INTO users (user_id, room_id, xp, level)
      VALUES (?, ?, ?, 1)
      ON CONFLICT(user_id, room_id) DO UPDATE SET
        xp = xp + ?,
        level = ?
    `).run(
      userId,
      roomId,
      amount,
      amount,
      // We recalculate level from the new XP total via a subquery-style approach.
      // Since better-sqlite3 is sync, we do a two-step:
      0 // placeholder — we fix level right after
    );

    // Fix level based on actual XP
    const row = db.prepare(`SELECT xp FROM users WHERE user_id = ? AND room_id = ?`).get(userId, roomId) as
      | { xp: number }
      | undefined;
    let newLevel = oldLevel || 1;
    if (row) {
      newLevel = xpToLevel(row.xp);
      db.prepare(`UPDATE users SET level = ? WHERE user_id = ? AND room_id = ?`).run(newLevel, userId, roomId);
    }

    db.prepare(`INSERT INTO xp_log (user_id, room_id, amount, reason) VALUES (?, ?, ?, ?)`).run(
      userId,
      roomId,
      amount,
      reason
    );

    logger.debug(`Granted ${amount} XP to ${userId} in ${roomId}: ${reason}`);

    // Announce level up
    if (newLevel > oldLevel && oldLevel > 0) {
      const template = LEVEL_UP_MESSAGES[Math.floor(Math.random() * LEVEL_UP_MESSAGES.length)];
      const msg = template.replace("{user}", userId).replace("{level}", String(newLevel));
      this.sendMessage(roomId, msg).catch((err) => {
        logger.error(`Failed to announce level up: ${err}`);
      });
    }
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive XP grant with cooldown
    this.handlePassiveXp(ctx.sender, ctx.roomId);

    // Commands
    if (this.isCommand(ctx.body, "rank")) {
      await this.handleRank(ctx);
    } else if (this.isCommand(ctx.body, "leaderboard")) {
      await this.handleLeaderboard(ctx);
    }
  }

  private handlePassiveXp(userId: string, roomId: string): void {
    const key = `${userId}:${roomId}`;
    const now = Date.now();
    const lastGrant = this.cooldowns.get(key) ?? 0;

    if (now - lastGrant < XP_COOLDOWN_MS) return;

    this.cooldowns.set(key, now);
    this.grantXp(userId, roomId, XP_PER_MESSAGE, "message");
  }

  private async handleRank(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "rank");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT xp, level, rep FROM users WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { xp: number; level: number; rep: number } | undefined;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No data found for ${targetUser}.`);
      return;
    }

    const progress = xpForNextLevel(row.xp);
    const bar = progressBar(progress.current, progress.needed);

    await this.sendMessage(
      ctx.roomId,
      `${targetUser}\nLevel ${row.level} | ${row.xp} XP | ${row.rep} rep\n${bar} ${progress.current}/${progress.needed} to next level`
    );
  }

  private async handleLeaderboard(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "leaderboard");
    const limit = Math.min(Math.max(parseInt(args) || 10, 1), 25);

    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, xp, level FROM users WHERE room_id = ? ORDER BY xp DESC LIMIT ?`)
      .all(ctx.roomId, limit) as { user_id: string; xp: number; level: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No leaderboard data yet.");
      return;
    }

    const lines = rows.map((r, i) => `${i + 1}. ${r.user_id} — Lv${r.level} (${r.xp} XP)`);
    await this.sendMessage(ctx.roomId, `Leaderboard (Top ${rows.length}):\n${lines.join("\n")}`);
  }
}
