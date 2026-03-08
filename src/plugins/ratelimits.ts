import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";

export class RateLimitsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
    this.ensureTable();
  }

  get name() {
    return "ratelimits";
  }

  get commands(): CommandDef[] {
    return [];
  }

  async onMessage(_ctx: MessageContext): Promise<void> {
    // No-op: this plugin provides utility methods only
  }

  private ensureTable(): void {
    const db = getDb();
    db.prepare(`
      CREATE TABLE IF NOT EXISTS command_usage (
        user_id TEXT,
        command TEXT,
        date TEXT,
        count INTEGER DEFAULT 0,
        PRIMARY KEY (user_id, command, date)
      )
    `).run();
  }

  private todayKey(): string {
    const now = new Date();
    const y = now.getUTCFullYear();
    const m = String(now.getUTCMonth() + 1).padStart(2, "0");
    const d = String(now.getUTCDate()).padStart(2, "0");
    return `${y}-${m}-${d}`;
  }

  private getCount(userId: string, command: string, date: string): number {
    const db = getDb();
    const row = db
      .prepare(`SELECT count FROM command_usage WHERE user_id = ? AND command = ? AND date = ?`)
      .get(userId, command, date) as { count: number } | undefined;
    return row?.count ?? 0;
  }

  checkLimit(userId: string, command: string, dailyMax: number): boolean {
    if (dailyMax === 0) return true;
    if (this.isAdmin(userId)) return true;

    const date = this.todayKey();
    const current = this.getCount(userId, command, date);

    if (current >= dailyMax) return false;

    const db = getDb();
    db.prepare(`
      INSERT INTO command_usage (user_id, command, date, count)
      VALUES (?, ?, ?, 1)
      ON CONFLICT(user_id, command, date) DO UPDATE SET count = count + 1
    `).run(userId, command, date);

    return true;
  }

  remaining(userId: string, command: string, dailyMax: number): number {
    if (dailyMax === 0) return Infinity;
    if (this.isAdmin(userId)) return Infinity;

    const date = this.todayKey();
    const current = this.getCount(userId, command, date);
    return Math.max(0, dailyMax - current);
  }
}
