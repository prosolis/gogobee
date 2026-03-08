import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { parseMessage, deriveArchetype } from "../utils/parser";
import logger from "../utils/logger";

const MILESTONES = [1000, 5000, 10000, 25000, 50000, 100000, 250000, 500000, 1000000];

export class StatsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "stats";
  }

  get commands(): CommandDef[] {
    return [
      { name: "stats", description: "Full message metrics", usage: "!stats [@user]" },
      { name: "rankings", description: "Category leaderboard", usage: "!rankings [category] [month]" },
      { name: "personality", description: "Derived community archetype", usage: "!personality [@user]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: track stats for every message
    this.trackMessage(ctx);
    this.checkMilestone(ctx);

    if (this.isCommand(ctx.body, "personality")) {
      await this.handlePersonality(ctx);
    } else if (this.isCommand(ctx.body, "stats")) {
      await this.handleStats(ctx);
    } else if (this.isCommand(ctx.body, "rankings")) {
      await this.handleRankings(ctx);
    }
  }

  private trackMessage(ctx: MessageContext): void {
    const parsed = parseMessage(ctx.body);
    const db = getDb();
    const now = new Date();
    const hour = now.getUTCHours();
    const day = now.getUTCDay();

    // Get existing stats to update distributions
    const existing = db
      .prepare(`SELECT hourly_distribution, daily_distribution, total_messages, total_words FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(ctx.sender, ctx.roomId) as
      | { hourly_distribution: string; daily_distribution: string; total_messages: number; total_words: number }
      | undefined;

    const hourly: Record<string, number> = JSON.parse(existing?.hourly_distribution ?? "{}");
    const daily: Record<string, number> = JSON.parse(existing?.daily_distribution ?? "{}");
    hourly[hour] = (hourly[hour] ?? 0) + 1;
    daily[day] = (daily[day] ?? 0) + 1;

    const newTotalMessages = (existing?.total_messages ?? 0) + 1;
    const newTotalWords = (existing?.total_words ?? 0) + parsed.wordCount;
    const newAvgWords = newTotalWords / newTotalMessages;

    db.prepare(`
      INSERT INTO user_stats (
        user_id, room_id, total_messages, total_words, total_characters,
        total_links, total_images, total_questions, total_exclamations,
        total_emojis, longest_message, shortest_message, avg_words_per_message,
        hourly_distribution, daily_distribution, last_active_date
      ) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, date('now'))
      ON CONFLICT(user_id, room_id) DO UPDATE SET
        total_messages = total_messages + 1,
        total_words = total_words + ?,
        total_characters = total_characters + ?,
        total_links = total_links + ?,
        total_images = total_images + ?,
        total_questions = total_questions + ?,
        total_exclamations = total_exclamations + ?,
        total_emojis = total_emojis + ?,
        longest_message = MAX(longest_message, ?),
        shortest_message = MIN(COALESCE(shortest_message, ?), ?),
        avg_words_per_message = ?,
        hourly_distribution = ?,
        daily_distribution = ?,
        last_active_date = date('now')
    `).run(
      ctx.sender,
      ctx.roomId,
      parsed.wordCount,
      parsed.charCount,
      parsed.linkCount,
      parsed.imageCount,
      parsed.questionCount,
      parsed.exclamationCount,
      parsed.emojiCount,
      parsed.charCount,
      parsed.charCount,
      parsed.wordCount,
      JSON.stringify(hourly),
      JSON.stringify(daily),
      // ON CONFLICT params:
      parsed.wordCount,
      parsed.charCount,
      parsed.linkCount,
      parsed.imageCount,
      parsed.questionCount,
      parsed.exclamationCount,
      parsed.emojiCount,
      parsed.charCount,
      parsed.charCount,
      parsed.charCount,
      newAvgWords,
      JSON.stringify(hourly),
      JSON.stringify(daily)
    );
  }

  private checkMilestone(ctx: MessageContext): void {
    try {
      const db = getDb();
      const row = db
        .prepare(
          `INSERT INTO room_milestones (room_id, total_messages, last_milestone)
           VALUES (?, 1, 0)
           ON CONFLICT(room_id) DO UPDATE SET total_messages = total_messages + 1
           RETURNING total_messages, last_milestone`
        )
        .get(ctx.roomId) as { total_messages: number; last_milestone: number };

      const nextMilestone = MILESTONES.find((m) => m > row.last_milestone && m <= row.total_messages);
      if (nextMilestone) {
        db.prepare(`UPDATE room_milestones SET last_milestone = ? WHERE room_id = ?`)
          .run(nextMilestone, ctx.roomId);

        const formatted = nextMilestone >= 1000000
          ? `${(nextMilestone / 1000000).toFixed(nextMilestone % 1000000 === 0 ? 0 : 1)}M`
          : nextMilestone >= 1000
            ? `${(nextMilestone / 1000).toFixed(nextMilestone % 1000 === 0 ? 0 : 1)}k`
            : String(nextMilestone);

        this.sendMessage(ctx.roomId, `This room just hit ${formatted} messages! Congrats to everyone who contributed to this milestone.`)
          .catch((err) => logger.error(`Failed to send milestone: ${err}`));
      }
    } catch (err) {
      logger.error(`Milestone check failed: ${err}`);
    }
  }

  private async handleStats(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "stats");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT * FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as any;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No stats found for ${targetUser}.`);
      return;
    }

    const lines = [
      `Stats for ${targetUser}:`,
      `Messages: ${row.total_messages} | Words: ${row.total_words}`,
      `Avg words/msg: ${row.avg_words_per_message.toFixed(1)}`,
      `Links: ${row.total_links} | Images: ${row.total_images}`,
      `Questions: ${row.total_questions} | Exclamations: ${row.total_exclamations}`,
      `Emojis: ${row.total_emojis}`,
      `Longest msg: ${row.longest_message} chars`,
      `Streak: ${row.current_streak}d (record: ${row.longest_streak}d)`,
    ];

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private async handleRankings(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "rankings");
    const parts = args.split(/\s+/);
    const category = parts[0] || "messages";

    const columnMap: Record<string, string> = {
      messages: "total_messages",
      words: "total_words",
      links: "total_links",
      images: "total_images",
      questions: "total_questions",
      emojis: "total_emojis",
      streak: "longest_streak",
    };

    const column = columnMap[category];
    if (!column) {
      const valid = Object.keys(columnMap).join(", ");
      await this.sendReply(ctx.roomId, ctx.eventId, `Unknown category. Valid: ${valid}`);
      return;
    }

    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, ${column} as value FROM user_stats WHERE room_id = ? ORDER BY ${column} DESC LIMIT 10`)
      .all(ctx.roomId) as { user_id: string; value: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No data yet.");
      return;
    }

    const lines = rows.map((r, i) => `${i + 1}. ${r.user_id} — ${r.value}`);
    await this.sendMessage(ctx.roomId, `Rankings (${category}):\n${lines.join("\n")}`);
  }

  private async handlePersonality(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "personality");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT total_messages, avg_words_per_message, total_questions, total_links, total_images, total_exclamations, hourly_distribution FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as any;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No data found for ${targetUser}.`);
      return;
    }

    const archetype = deriveArchetype(row);
    await this.sendMessage(ctx.roomId, `${targetUser}'s community archetype: ${archetype}`);
  }
}
