import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";

export class StreaksPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "streaks";
  }

  get commands(): CommandDef[] {
    return [
      { name: "streak", description: "Current and record streak", usage: "!streak [@user]" },
      { name: "firstboard", description: "Early bird leaderboard", usage: "!firstboard [@user|month]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: track daily activity and first poster
    this.trackActivity(ctx);

    if (this.isCommand(ctx.body, "firstboard")) {
      await this.handleFirstboard(ctx);
    } else if (this.isCommand(ctx.body, "streak")) {
      await this.handleStreak(ctx);
    }
  }

  private trackActivity(ctx: MessageContext): void {
    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    // Track daily activity
    db.prepare(`
      INSERT INTO daily_activity (user_id, room_id, date, message_count)
      VALUES (?, ?, ?, 1)
      ON CONFLICT(user_id, room_id, date) DO UPDATE SET message_count = message_count + 1
    `).run(ctx.sender, ctx.roomId, today);

    // Track first poster (INSERT OR IGNORE — first one wins)
    db.prepare(`
      INSERT OR IGNORE INTO daily_first (room_id, date, user_id, timestamp)
      VALUES (?, ?, ?, datetime('now'))
    `).run(ctx.roomId, today, ctx.sender);

    // Update streak in user_stats
    this.updateStreak(ctx.sender, ctx.roomId, today);
  }

  private updateStreak(userId: string, roomId: string, today: string): void {
    const db = getDb();
    const stats = db
      .prepare(`SELECT last_active_date, current_streak, longest_streak FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as
      | { last_active_date: string | null; current_streak: number; longest_streak: number }
      | undefined;

    if (!stats) return;

    const lastDate = stats.last_active_date;
    if (lastDate === today) return; // Already counted today

    // Check if yesterday was active
    const yesterday = new Date();
    yesterday.setUTCDate(yesterday.getUTCDate() - 1);
    const yesterdayStr = yesterday.toISOString().slice(0, 10);

    let newStreak: number;
    if (lastDate === yesterdayStr) {
      newStreak = stats.current_streak + 1;
    } else {
      newStreak = 1;
    }

    const newLongest = Math.max(stats.longest_streak, newStreak);

    db.prepare(`
      UPDATE user_stats SET current_streak = ?, longest_streak = ?
      WHERE user_id = ? AND room_id = ?
    `).run(newStreak, newLongest, userId, roomId);
  }

  private async handleStreak(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "streak");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const stats = db
      .prepare(`SELECT current_streak, longest_streak FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { current_streak: number; longest_streak: number } | undefined;

    if (!stats) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No streak data for ${targetUser}.`);
      return;
    }

    await this.sendMessage(
      ctx.roomId,
      `${targetUser}'s streak: ${stats.current_streak} day${stats.current_streak !== 1 ? "s" : ""} (record: ${stats.longest_streak})`
    );
  }

  private async handleFirstboard(ctx: MessageContext): Promise<void> {
    const db = getDb();

    const rows = db
      .prepare(`
        SELECT user_id, COUNT(*) as firsts
        FROM daily_first
        WHERE room_id = ?
        GROUP BY user_id
        ORDER BY firsts DESC
        LIMIT 10
      `)
      .all(ctx.roomId) as { user_id: string; firsts: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No first-poster data yet.");
      return;
    }

    const lines = rows.map((r, i) => `${i + 1}. ${r.user_id} — ${r.firsts} first${r.firsts !== 1 ? "s" : ""}`);
    await this.sendMessage(ctx.roomId, `Early Bird Leaderboard:\n${lines.join("\n")}`);
  }
}
