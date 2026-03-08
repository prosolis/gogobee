import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { v4 as uuidv4 } from "uuid";
import * as chrono from "chrono-node";
import logger from "../utils/logger";

export class RemindersPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "reminders";
  }

  get commands(): CommandDef[] {
    return [
      { name: "remindme", description: "Set a reminder", usage: '!remindme <time> <message>' },
      { name: "reminders", description: "List your pending reminders" },
      { name: "unremind", description: "Cancel a reminder", usage: "!unremind <id>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "remindme")) {
      await this.handleRemindMe(ctx);
    } else if (this.isCommand(ctx.body, "unremind")) {
      await this.handleUnremind(ctx);
    } else if (this.isCommand(ctx.body, "reminders")) {
      await this.handleListReminders(ctx);
    }
  }

  /**
   * Called by DailyScheduler every 30 seconds to fire due reminders.
   */
  async checkReminders(): Promise<void> {
    const db = getDb();
    const now = new Date().toISOString();

    const due = db
      .prepare(`SELECT * FROM reminders WHERE fired = 0 AND remind_at <= ?`)
      .all(now) as {
      id: string;
      user_id: string;
      room_id: string;
      message: string;
      remind_at: string;
    }[];

    for (const reminder of due) {
      try {
        await this.sendMessage(
          reminder.room_id,
          `Reminder for ${reminder.user_id}: ${reminder.message}`
        );
        db.prepare(`UPDATE reminders SET fired = 1 WHERE id = ?`).run(reminder.id);
      } catch (err) {
        logger.error(`Failed to fire reminder ${reminder.id}: ${err}`);
      }
    }
  }

  private async handleRemindMe(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "remindme");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !remindme <time> <message>\nExample: !remindme in 2 hours check the oven");
      return;
    }

    // Get user timezone for parsing context
    const db = getDb();
    const userRow = db
      .prepare(`SELECT timezone FROM users WHERE user_id = ? AND room_id = ?`)
      .get(ctx.sender, ctx.roomId) as { timezone: string | null } | undefined;

    const refDate = new Date();
    const parsed = chrono.parse(args, refDate, { forwardDate: true });

    if (parsed.length === 0 || !parsed[0].start) {
      await this.sendReply(ctx.roomId, ctx.eventId, "I couldn't understand that time. Try something like: in 30 minutes, tomorrow at 3pm, next friday");
      return;
    }

    const remindAt = parsed[0].start.date();
    // Extract message: everything after the parsed time expression
    const timeText = parsed[0].text;
    let message = args.slice(args.indexOf(timeText) + timeText.length).trim();
    if (!message) message = "(no message)";

    const id = uuidv4().slice(0, 8);

    db.prepare(`INSERT INTO reminders (id, user_id, room_id, message, remind_at) VALUES (?, ?, ?, ?, ?)`).run(
      id,
      ctx.sender,
      ctx.roomId,
      message,
      remindAt.toISOString()
    );

    const timeStr = remindAt.toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" });
    await this.sendReply(ctx.roomId, ctx.eventId, `Reminder set for ${timeStr} (UTC). ID: ${id}`);
  }

  private async handleListReminders(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT id, message, remind_at FROM reminders WHERE user_id = ? AND room_id = ? AND fired = 0 ORDER BY remind_at ASC`)
      .all(ctx.sender, ctx.roomId) as { id: string; message: string; remind_at: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You have no pending reminders.");
      return;
    }

    const lines = rows.map((r) => {
      const time = new Date(r.remind_at).toLocaleString("en-US", { dateStyle: "medium", timeStyle: "short" });
      return `[${r.id}] ${time} — ${r.message}`;
    });

    await this.sendMessage(ctx.roomId, `Your reminders:\n${lines.join("\n")}`);
  }

  private async handleUnremind(ctx: MessageContext): Promise<void> {
    const id = this.getArgs(ctx.body, "unremind").trim();
    if (!id) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !unremind <id>");
      return;
    }

    const db = getDb();
    const result = db
      .prepare(`DELETE FROM reminders WHERE id = ? AND user_id = ? AND room_id = ? AND fired = 0`)
      .run(id, ctx.sender, ctx.roomId);

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Reminder ${id} cancelled.`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `No pending reminder found with ID ${id}.`);
    }
  }
}
