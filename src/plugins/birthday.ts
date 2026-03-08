import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

const MONTH_NAMES = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];

export class BirthdayPlugin extends Plugin {
  private xpPlugin: XpPlugin;

  constructor(client: IMatrixClient, xpPlugin: XpPlugin) {
    super(client);
    this.xpPlugin = xpPlugin;
  }

  get name() {
    return "birthday";
  }

  get commands(): CommandDef[] {
    return [
      { name: "birthday set", description: "Set your birthday", usage: "!birthday set <month> <day> [year]" },
      { name: "birthday remove", description: "Remove your birthday", usage: "!birthday remove" },
      { name: "birthday", description: "Show a birthday", usage: "!birthday [@user]" },
      { name: "birthdays", description: "Upcoming birthdays in the next 30 days", usage: "!birthdays" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "birthdays")) {
      await this.handleUpcoming(ctx);
    } else if (this.isCommand(ctx.body, "birthday set")) {
      await this.handleSet(ctx);
    } else if (this.isCommand(ctx.body, "birthday remove")) {
      await this.handleRemove(ctx);
    } else if (this.isCommand(ctx.body, "birthday")) {
      await this.handleShow(ctx);
    }
  }

  private async handleSet(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "birthday set").trim().split(/\s+/);

    if (args.length < 2) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !birthday set <month> <day> [year]\nExample: !birthday set 3 15 1990");
      return;
    }

    const month = parseInt(args[0]);
    const day = parseInt(args[1]);
    const year = args[2] ? parseInt(args[2]) : null;

    if (isNaN(month) || month < 1 || month > 12) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Month must be between 1 and 12.");
      return;
    }

    if (isNaN(day) || day < 1 || day > 31) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Day must be between 1 and 31.");
      return;
    }

    if (year !== null && isNaN(year)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Invalid year.");
      return;
    }

    const db = getDb();
    db.prepare(`
      INSERT INTO birthdays (user_id, room_id, month, day, year)
      VALUES (?, ?, ?, ?, ?)
      ON CONFLICT(user_id, room_id) DO UPDATE SET
        month = excluded.month,
        day = excluded.day,
        year = excluded.year
    `).run(ctx.sender, ctx.roomId, month, day, year);

    const label = `${MONTH_NAMES[month - 1]} ${day}${year ? `, ${year}` : ""}`;
    await this.sendReply(ctx.roomId, ctx.eventId, `Birthday set to ${label}.`);
  }

  private async handleRemove(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const result = db
      .prepare(`DELETE FROM birthdays WHERE user_id = ? AND room_id = ?`)
      .run(ctx.sender, ctx.roomId);

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Your birthday has been removed.");
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, "You don't have a birthday set in this room.");
    }
  }

  private async handleShow(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "birthday").trim();
    const target = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT month, day, year FROM birthdays WHERE user_id = ? AND room_id = ?`)
      .get(target, ctx.roomId) as { month: number; day: number; year: number | null } | undefined;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No birthday set for ${target}.`);
      return;
    }

    let message = `${target}'s birthday: ${MONTH_NAMES[row.month - 1]} ${row.day}`;

    // Only show year/age to the user themselves
    if (ctx.sender === target && row.year) {
      const now = new Date();
      const age = this.calculateAge(row.year, row.month, row.day, now);
      message += `, ${row.year} (age ${age})`;
    }

    await this.sendReply(ctx.roomId, ctx.eventId, message);
  }

  private async handleUpcoming(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, month, day FROM birthdays WHERE room_id = ?`)
      .all(ctx.roomId) as { user_id: string; month: number; day: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No birthdays set in this room.");
      return;
    }

    const now = new Date();
    const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));

    interface UpcomingEntry {
      userId: string;
      month: number;
      day: number;
      daysUntil: number;
    }

    const upcoming: UpcomingEntry[] = [];

    for (const row of rows) {
      let nextOccurrence = new Date(Date.UTC(today.getUTCFullYear(), row.month - 1, row.day));
      if (nextOccurrence < today) {
        nextOccurrence = new Date(Date.UTC(today.getUTCFullYear() + 1, row.month - 1, row.day));
      }

      const diffMs = nextOccurrence.getTime() - today.getTime();
      const daysUntil = Math.round(diffMs / (1000 * 60 * 60 * 24));

      if (daysUntil <= 30) {
        upcoming.push({ userId: row.user_id, month: row.month, day: row.day, daysUntil });
      }
    }

    if (upcoming.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No birthdays in the next 30 days.");
      return;
    }

    upcoming.sort((a, b) => a.daysUntil - b.daysUntil);

    const lines = upcoming.map((entry) => {
      return `${MONTH_NAMES[entry.month - 1]} ${entry.day} — ${entry.userId}`;
    });

    await this.sendMessage(ctx.roomId, `Upcoming birthdays (next 30 days):\n${lines.join("\n")}`);
  }

  /**
   * Called by DailyScheduler to post birthday announcements.
   */
  async checkAndPost(botRooms: string[]): Promise<void> {
    const now = new Date();
    const currentMonth = now.getUTCMonth() + 1;
    const currentDay = now.getUTCDate();
    const currentYear = now.getUTCFullYear();

    const db = getDb();

    const matches = db
      .prepare(`SELECT user_id, room_id, year FROM birthdays WHERE month = ? AND day = ?`)
      .all(currentMonth, currentDay) as { user_id: string; room_id: string; year: number | null }[];

    if (matches.length === 0) return;

    // Group by user to avoid duplicate DMs
    const dmSent = new Set<string>();

    for (const match of matches) {
      // Only post in rooms the bot is active in
      if (!botRooms.includes(match.room_id)) continue;

      // Check if already fired for this user this year
      const fired = db
        .prepare(`SELECT 1 FROM birthday_fired WHERE user_id = ? AND year = ?`)
        .get(match.user_id, currentYear);

      if (fired) continue;

      // Build announcement
      let announcement: string;
      if (match.year) {
        const age = this.calculateAge(match.year, currentMonth, currentDay, now);
        announcement = `Happy Birthday ${match.user_id} — turning ${age} today!`;
      } else {
        announcement = `Happy Birthday ${match.user_id}!`;
      }

      try {
        await this.sendMessage(match.room_id, announcement);
        this.xpPlugin.grantXp(match.user_id, match.room_id, 100, "birthday");
        logger.info(`Posted birthday for ${match.user_id} in ${match.room_id}`);
      } catch (err) {
        logger.error(`Failed to post birthday for ${match.user_id} in ${match.room_id}: ${err}`);
      }

      // Send DM once per user
      if (!dmSent.has(match.user_id)) {
        dmSent.add(match.user_id);
        try {
          await this.sendDm(match.user_id, "Happy Birthday! Hope you have an amazing day!");
        } catch (err) {
          logger.error(`Failed to DM birthday user ${match.user_id}: ${err}`);
        }
      }

      // Mark as fired for this year
      db.prepare(`INSERT OR IGNORE INTO birthday_fired (user_id, year) VALUES (?, ?)`).run(
        match.user_id,
        currentYear
      );
    }
  }

  private calculateAge(birthYear: number, birthMonth: number, birthDay: number, now: Date): number {
    let age = now.getUTCFullYear() - birthYear;
    const monthDiff = (now.getUTCMonth() + 1) - birthMonth;
    if (monthDiff < 0 || (monthDiff === 0 && now.getUTCDate() < birthDay)) {
      age--;
    }
    return age;
  }
}
