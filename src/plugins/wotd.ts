import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

const WOTD_BONUS_XP = 25;

export class WotdPlugin extends Plugin {
  private xpPlugin: XpPlugin;
  private todaysWord: string | null = null;

  constructor(client: IMatrixClient, xpPlugin: XpPlugin) {
    super(client);
    this.xpPlugin = xpPlugin;
    this.loadTodaysWord();
  }

  get name() {
    return "wotd";
  }

  get commands(): CommandDef[] {
    return [
      { name: "wotd", description: "Show today's Word of the Day" },
    ];
  }

  private loadTodaysWord(): void {
    try {
      const db = getDb();
      const today = new Date().toISOString().slice(0, 10);
      // Try today first, fall back to the most recent word
      let row = db
        .prepare(`SELECT word FROM wotd_log WHERE date = ? LIMIT 1`)
        .get(today) as { word: string } | undefined;
      if (!row) {
        row = db
          .prepare(`SELECT word FROM wotd_log ORDER BY date DESC LIMIT 1`)
          .get() as { word: string } | undefined;
      }
      this.todaysWord = row?.word?.toLowerCase() ?? null;
    } catch {
      this.todaysWord = null;
    }
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "wotd")) {
      await this.handleWotd(ctx);
      return;
    }

    // Passive: check if message contains today's word
    if (this.todaysWord) {
      this.checkWordUsage(ctx);
    }
  }

  private async handleWotd(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    // Try today first, fall back to the most recent word if today's hasn't been posted yet
    let row = db
      .prepare(`SELECT word, definition, example, part_of_speech, date FROM wotd_log WHERE date = ? LIMIT 1`)
      .get(today) as { word: string; definition: string | null; example: string | null; part_of_speech: string | null; date: string } | undefined;

    if (!row) {
      row = db
        .prepare(`SELECT word, definition, example, part_of_speech, date FROM wotd_log ORDER BY date DESC LIMIT 1`)
        .get() as typeof row;
    }

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No Word of the Day yet. It will be posted at the scheduled time.");
      return;
    }

    const isToday = row.date === today;
    const lines = [
      isToday ? `Word of the Day: ${row.word}` : `Most recent Word of the Day: ${row.word} (${row.date})`,
      row.part_of_speech ? `(${row.part_of_speech})` : "",
      "",
      row.definition ?? "No definition available.",
      row.example ? `\nExample: "${row.example}"` : "",
      "",
      isToday
        ? `Use "${row.word}" in a message today for ${WOTD_BONUS_XP} bonus XP!`
        : `Today's word hasn't been posted yet. Check back after the scheduled time.`,
    ]
      .filter(Boolean)
      .join("\n");

    await this.sendMessage(ctx.roomId, lines);
  }

  private checkWordUsage(ctx: MessageContext): void {
    if (!this.todaysWord) return;

    const bodyLower = ctx.body.toLowerCase();
    if (!bodyLower.includes(this.todaysWord)) return;

    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    // Check if already awarded today
    const existing = db
      .prepare(`SELECT id FROM wotd_usage WHERE user_id = ? AND room_id = ? AND date = ?`)
      .get(ctx.sender, ctx.roomId, today);
    if (existing) return;

    // Award bonus XP
    db.prepare(`INSERT INTO wotd_usage (user_id, room_id, date, xp_awarded) VALUES (?, ?, ?, ?)`).run(
      ctx.sender,
      ctx.roomId,
      today,
      WOTD_BONUS_XP
    );

    this.xpPlugin.grantXp(ctx.sender, ctx.roomId, WOTD_BONUS_XP, "wotd_usage");
    logger.debug(`${ctx.sender} used WOTD "${this.todaysWord}" — awarded ${WOTD_BONUS_XP} XP`);
  }

  /**
   * Prefetch WOTD data from API and cache it in the DB.
   * Called early (e.g. 00:05 UTC) so the post step has no network dependency.
   */
  async prefetch(): Promise<void> {
    const apiKey = process.env.WORDNIK_API_KEY;
    if (!apiKey) return;

    const today = new Date().toISOString().slice(0, 10);
    const db = getDb();

    const existing = db.prepare(`SELECT data FROM daily_prefetch WHERE job_name = 'wotd' AND date = ?`).get(today);
    if (existing) return;

    try {
      const url = `https://api.wordnik.com/v4/words.json/wordOfTheDay?api_key=${encodeURIComponent(apiKey)}`;
      const res = await fetch(url);
      if (!res.ok) {
        logger.warn(`Wordnik prefetch returned ${res.status}`);
        return;
      }

      const data = await res.json() as any;
      db.prepare(`INSERT OR REPLACE INTO daily_prefetch (job_name, date, data) VALUES ('wotd', ?, ?)`)
        .run(today, JSON.stringify(data));
      logger.info(`Prefetched WOTD for ${today}: ${data.word}`);
    } catch (err) {
      logger.error(`WOTD prefetch failed: ${err}`);
    }
  }

  /**
   * Called by DailyScheduler to post the word of the day.
   * Reads from prefetch cache first, falls back to live API call.
   */
  async postWotd(roomIds: string[]): Promise<void> {
    const apiKey = process.env.WORDNIK_API_KEY;
    if (!apiKey) {
      logger.warn("WORDNIK_API_KEY not set, skipping WOTD");
      return;
    }

    const today = new Date().toISOString().slice(0, 10);
    const db = getDb();

    // Check if already posted today (any room)
    const existing = db.prepare(`SELECT word FROM wotd_log WHERE date = ?`).get(today) as { word: string } | undefined;
    if (existing) {
      this.todaysWord = existing.word.toLowerCase();
      return;
    }

    try {
      // Try prefetch cache first, fall back to live API
      let data: any;
      const cached = db.prepare(`SELECT data FROM daily_prefetch WHERE job_name = 'wotd' AND date = ?`).get(today) as { data: string } | undefined;
      if (cached) {
        data = JSON.parse(cached.data);
      } else {
        const url = `https://api.wordnik.com/v4/words.json/wordOfTheDay?api_key=${encodeURIComponent(apiKey)}`;
        const res = await fetch(url);
        if (!res.ok) {
          logger.warn(`Wordnik API returned ${res.status}`);
          return;
        }
        data = await res.json() as any;
      }

      const word = data.word;
      const definitions = data.definitions ?? [];
      const examples = data.examples ?? [];

      const definition = definitions[0]?.text ?? "No definition available.";
      const partOfSpeech = definitions[0]?.partOfSpeech ?? "";
      const example = examples[0]?.text ?? "";

      this.todaysWord = word.toLowerCase();

      const message = [
        `Word of the Day: ${word}`,
        partOfSpeech ? `(${partOfSpeech})` : "",
        "",
        definition,
        example ? `\nExample: "${example}"` : "",
        "",
        `Use "${word}" in a message today for ${WOTD_BONUS_XP} bonus XP!`,
      ]
        .filter(Boolean)
        .join("\n");

      for (const roomId of roomIds) {
        try {
          await this.sendMessage(roomId, message);
          db.prepare(`
            INSERT OR IGNORE INTO wotd_log (room_id, date, word, definition, example, part_of_speech)
            VALUES (?, ?, ?, ?, ?, ?)
          `).run(roomId, today, word, definition, example || null, partOfSpeech || null);
        } catch (err) {
          logger.error(`Failed to post WOTD to ${roomId}: ${err}`);
        }
      }
    } catch (err) {
      logger.error(`WOTD fetch failed: ${err}`);
    }
  }
}
