import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import fs from "fs";
import path from "path";
import logger from "../utils/logger";

const LLM_ENABLED = (process.env.OLLAMA_HOST ?? "") !== "" && (process.env.OLLAMA_MODEL ?? "") !== "";
const OLLAMA_HOST = (() => {
  const raw = process.env.OLLAMA_HOST ?? "";
  return raw && !raw.startsWith("http") ? `http://${raw}` : raw;
})();
const startTime = Date.now();
let messagesProcessed = 0;

export function incrementMessageCount(): void {
  messagesProcessed++;
}

export class BotInfoPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "botinfo";
  }

  get commands(): CommandDef[] {
    return [
      { name: "botinfo", description: "Bot diagnostics", adminOnly: true },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    incrementMessageCount();

    if (this.isCommand(ctx.body, "botinfo")) {
      await this.handleBotInfo(ctx);
    }
  }

  private async handleBotInfo(ctx: MessageContext): Promise<void> {
    if (!this.isAdmin(ctx.sender)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Admin only.");
      return;
    }

    const db = getDb();
    const uptimeMs = Date.now() - startTime;
    const uptime = this.formatUptime(uptimeMs);

    // DB size
    const dataDir = process.env.DATA_DIR ?? "./data";
    const dbPath = path.join(dataDir, "freebee.db");
    let dbSize = "unknown";
    try {
      const stats = fs.statSync(dbPath);
      dbSize = this.formatBytes(stats.size);
    } catch { /* ignore */ }

    // Active reminders
    let activeReminders = 0;
    try {
      const row = db.prepare(`SELECT COUNT(*) as count FROM reminders WHERE fired = 0`).get() as { count: number };
      activeReminders = row.count;
    } catch { /* ignore */ }

    // Total room messages
    let totalRoomMessages = 0;
    try {
      const row = db.prepare(`SELECT SUM(total_messages) as total FROM room_milestones`).get() as { total: number | null };
      totalRoomMessages = row?.total ?? 0;
    } catch { /* ignore */ }

    // LLM status
    let llmStatus = "disabled";
    if (LLM_ENABLED) {
      try {
        const res = await fetch(`${OLLAMA_HOST}/api/tags`, { signal: AbortSignal.timeout(5000) });
        llmStatus = res.ok ? `running (${process.env.OLLAMA_MODEL})` : `error (HTTP ${res.status})`;
      } catch {
        llmStatus = "error (unreachable)";
      }
    }

    // Estimated LLM tokens (rough: ~1.3 tokens per word for English)
    let estimatedTokens = "N/A";
    if (LLM_ENABLED) {
      try {
        const row = db.prepare(`SELECT COUNT(*) as classified FROM llm_classifications`).get() as { classified: number };
        const wordRow = db.prepare(`SELECT SUM(total_words) as words FROM user_stats`).get() as { words: number | null };
        const totalWords = wordRow?.words ?? 0;
        const tokens = Math.round(totalWords * 1.3);
        estimatedTokens = `~${this.formatNumber(tokens)} tokens (~${this.formatNumber(row.classified)} classifications)`;
      } catch { /* ignore */ }
    }

    const lines = [
      "Bot Info:",
      "",
      `Uptime: ${uptime}`,
      `Messages processed (session): ${this.formatNumber(messagesProcessed)}`,
      `Total room messages (all time): ${this.formatNumber(totalRoomMessages)}`,
      `Database size: ${dbSize}`,
      `Active reminders: ${activeReminders}`,
      `LLM classifier: ${llmStatus}`,
      `LLM tokens processed: ${estimatedTokens}`,
    ];

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private formatUptime(ms: number): string {
    const s = Math.floor(ms / 1000);
    const days = Math.floor(s / 86400);
    const hours = Math.floor((s % 86400) / 3600);
    const minutes = Math.floor((s % 3600) / 60);
    const parts: string[] = [];
    if (days > 0) parts.push(`${days}d`);
    if (hours > 0) parts.push(`${hours}h`);
    parts.push(`${minutes}m`);
    return parts.join(" ");
  }

  private formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  private formatNumber(n: number): string {
    return n.toLocaleString("en-US");
  }
}
