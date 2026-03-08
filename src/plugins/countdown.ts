import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

const MONTH_NAMES = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];

function formatDate(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00Z");
  const month = MONTH_NAMES[d.getUTCMonth()];
  const day = d.getUTCDate();
  const year = d.getUTCFullYear();
  return `${month} ${day}, ${year}`;
}

function todayStr(): string {
  const now = new Date();
  const y = now.getUTCFullYear();
  const m = String(now.getUTCMonth() + 1).padStart(2, "0");
  const d = String(now.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function daysBetween(a: string, b: string): number {
  const msA = new Date(a + "T00:00:00Z").getTime();
  const msB = new Date(b + "T00:00:00Z").getTime();
  return Math.round((msB - msA) / (1000 * 60 * 60 * 24));
}

interface CountdownRow {
  id: number;
  user_id: string;
  room_id: string;
  label: string;
  target_date: string;
  public: number;
  completed: number;
  created_at: number;
}

export class CountdownPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "countdown";
  }

  get commands(): CommandDef[] {
    return [
      { name: "countdown add", description: "Add a public countdown", usage: '!countdown add "<label>" <YYYY-MM-DD>' },
      { name: "countdown private", description: "Add a private countdown", usage: '!countdown private "<label>" <YYYY-MM-DD>' },
      { name: "countdown mine", description: "List your own countdowns" },
      { name: "countdown remove", description: "Remove a countdown", usage: "!countdown remove <id>" },
      { name: "countdown", description: "List countdowns or show a specific one", usage: "!countdown [id]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "countdown add")) {
      await this.handleAdd(ctx, true);
    } else if (this.isCommand(ctx.body, "countdown private")) {
      await this.handleAdd(ctx, false);
    } else if (this.isCommand(ctx.body, "countdown mine")) {
      await this.handleMine(ctx);
    } else if (this.isCommand(ctx.body, "countdown remove")) {
      await this.handleRemove(ctx);
    } else if (this.isCommand(ctx.body, "countdown")) {
      const args = this.getArgs(ctx.body, "countdown").trim();
      if (args && /^\d+$/.test(args)) {
        await this.handleShow(ctx, parseInt(args));
      } else if (!args) {
        await this.handleList(ctx);
      }
    }
  }

  private parseAddArgs(raw: string): { label: string; date: string } | null {
    // Expect: "<label>" <YYYY-MM-DD>
    const match = raw.match(/^"([^"]+)"\s+(\d{4}-\d{2}-\d{2})$/);
    if (!match) return null;
    return { label: match[1], date: match[2] };
  }

  private isValidDate(dateStr: string): boolean {
    const d = new Date(dateStr + "T00:00:00Z");
    return !isNaN(d.getTime());
  }

  private async handleAdd(ctx: MessageContext, isPublic: boolean): Promise<void> {
    const command = isPublic ? "countdown add" : "countdown private";
    const args = this.getArgs(ctx.body, command).trim();
    const parsed = this.parseAddArgs(args);

    if (!parsed) {
      const usage = isPublic
        ? '!countdown add "<label>" <YYYY-MM-DD>'
        : '!countdown private "<label>" <YYYY-MM-DD>';
      await this.sendReply(ctx.roomId, ctx.eventId, `Usage: ${usage}`);
      return;
    }

    if (!this.isValidDate(parsed.date)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Invalid date format. Use YYYY-MM-DD.");
      return;
    }

    const db = getDb();
    const result = db.prepare(
      `INSERT INTO countdowns (user_id, room_id, label, target_date, public) VALUES (?, ?, ?, ?, ?)`
    ).run(ctx.sender, ctx.roomId, parsed.label, parsed.date, isPublic ? 1 : 0);

    const id = result.lastInsertRowid;
    const today = todayStr();
    const days = daysBetween(today, parsed.date);
    const direction = days >= 0 ? `${days} days away` : `${Math.abs(days)} days ago`;
    const visibility = isPublic ? "public" : "private";

    await this.sendReply(
      ctx.roomId,
      ctx.eventId,
      `Countdown #${id} added (${visibility}): "${parsed.label}" on ${formatDate(parsed.date)} — ${direction}.`
    );
  }

  private async handleRemove(ctx: MessageContext): Promise<void> {
    const idStr = this.getArgs(ctx.body, "countdown remove").trim();
    if (!idStr || !/^\d+$/.test(idStr)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !countdown remove <id>");
      return;
    }

    const id = parseInt(idStr);
    const db = getDb();
    const result = db.prepare(
      `DELETE FROM countdowns WHERE id = ? AND user_id = ?`
    ).run(id, ctx.sender);

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Countdown #${id} removed.`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `No countdown found with ID ${id} (or it's not yours).`);
    }
  }

  private async handleShow(ctx: MessageContext, id: number): Promise<void> {
    const db = getDb();
    const row = db.prepare(
      `SELECT * FROM countdowns WHERE id = ? AND room_id = ? AND completed = 0`
    ).get(id, ctx.roomId) as CountdownRow | undefined;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No active countdown found with ID ${id}.`);
      return;
    }

    // Only show private countdowns to their owner
    if (!row.public && row.user_id !== ctx.sender) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No active countdown found with ID ${id}.`);
      return;
    }

    const today = todayStr();
    const days = daysBetween(today, row.target_date);
    const visibility = row.public ? "public" : "private";

    let detail: string;
    if (days > 0) {
      detail = `#${row.id} "${row.label}" — ${days} days  (${formatDate(row.target_date)}) [${visibility}]`;
    } else if (days === 0) {
      detail = `#${row.id} "${row.label}" — today!  (${formatDate(row.target_date)}) [${visibility}]`;
    } else {
      detail = `#${row.id} "${row.label}" — happened ${Math.abs(days)} days ago  (${formatDate(row.target_date)}) [${visibility}]`;
    }

    await this.sendMessage(ctx.roomId, detail);
  }

  private async handleList(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const today = todayStr();

    // Fetch all public countdowns for this room + sender's private ones, not yet completed
    const rows = db.prepare(
      `SELECT * FROM countdowns
       WHERE room_id = ? AND completed = 0
         AND (public = 1 OR user_id = ?)
       ORDER BY target_date ASC`
    ).all(ctx.roomId, ctx.sender) as CountdownRow[];

    // Cleanup: mark completed any that are past > 7 days
    const toComplete: number[] = [];
    const visible: CountdownRow[] = [];

    for (const row of rows) {
      const days = daysBetween(today, row.target_date);
      if (days < 0 && Math.abs(days) > 7) {
        toComplete.push(row.id);
      } else {
        visible.push(row);
      }
    }

    if (toComplete.length > 0) {
      const placeholders = toComplete.map(() => "?").join(",");
      db.prepare(`UPDATE countdowns SET completed = 1 WHERE id IN (${placeholders})`).run(...toComplete);
    }

    if (visible.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No active countdowns.");
      return;
    }

    const lines = this.formatCountdownLines(visible, today);
    await this.sendMessage(ctx.roomId, `Community Countdowns\n\n${lines.join("\n")}`);
  }

  private async handleMine(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const today = todayStr();

    const rows = db.prepare(
      `SELECT * FROM countdowns
       WHERE user_id = ? AND room_id = ? AND completed = 0
       ORDER BY target_date ASC`
    ).all(ctx.sender, ctx.roomId) as CountdownRow[];

    // Cleanup: mark completed any that are past > 7 days
    const toComplete: number[] = [];
    const visible: CountdownRow[] = [];

    for (const row of rows) {
      const days = daysBetween(today, row.target_date);
      if (days < 0 && Math.abs(days) > 7) {
        toComplete.push(row.id);
      } else {
        visible.push(row);
      }
    }

    if (toComplete.length > 0) {
      const placeholders = toComplete.map(() => "?").join(",");
      db.prepare(`UPDATE countdowns SET completed = 1 WHERE id IN (${placeholders})`).run(...toComplete);
    }

    if (visible.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You have no active countdowns.");
      return;
    }

    const lines = this.formatCountdownLines(visible, today);
    await this.sendMessage(ctx.roomId, `Your Countdowns\n\n${lines.join("\n")}`);
  }

  private formatCountdownLines(rows: CountdownRow[], today: string): string[] {
    return rows.map((row) => {
      const days = daysBetween(today, row.target_date);
      const label = row.label;

      if (days > 0) {
        const pad = " ".repeat(Math.max(0, 30 - label.length));
        return `${label}${pad} — ${days} days  (${formatDate(row.target_date)})`;
      } else if (days === 0) {
        const pad = " ".repeat(Math.max(0, 30 - label.length));
        return `${label}${pad} — today!  (${formatDate(row.target_date)})`;
      } else {
        const ago = Math.abs(days);
        const padLabel = `\u2705 ${label}`;
        const pad = " ".repeat(Math.max(0, 30 - padLabel.length));
        return `${padLabel}${pad} — happened ${ago} days ago`;
      }
    });
  }
}
