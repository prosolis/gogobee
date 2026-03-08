import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

const THANKS_REGEX = /\b(?:thanks?|thank\s*you|thx|ty|tysm|tyvm|cheers|kudos|props)\b.*?(@\S+:\S+)/i;
const THANKS_SIMPLE_REGEX = /\b(?:thanks?|thank\s*you|thx|ty|tysm|tyvm|cheers|kudos|props)\b/i;
const REP_XP_BONUS = 5;
const COOLDOWN_HOURS = 24;
const LLM_ENABLED = (process.env.OLLAMA_HOST ?? "") !== "" && (process.env.OLLAMA_MODEL ?? "") !== "";

export class ReputationPlugin extends Plugin {
  private xpPlugin: XpPlugin;

  constructor(client: IMatrixClient, xpPlugin: XpPlugin) {
    super(client);
    this.xpPlugin = xpPlugin;
  }

  get name() {
    return "reputation";
  }

  get commands(): CommandDef[] {
    return [
      { name: "rep", description: "Show reputation score", usage: "!rep [@user]" },
      { name: "repboard", description: "Reputation leaderboard" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive thanks detection (disabled when LLM handles it with sarcasm awareness)
    if (!LLM_ENABLED) this.detectThanks(ctx);

    if (this.isCommand(ctx.body, "repboard")) {
      await this.handleRepboard(ctx);
    } else if (this.isCommand(ctx.body, "rep")) {
      await this.handleRep(ctx);
    }
  }

  private detectThanks(ctx: MessageContext): void {
    // Try to find a mentioned user in the thanks message
    const mentionMatch = ctx.body.match(THANKS_REGEX);
    if (!mentionMatch) return;

    const receiverId = mentionMatch[1];
    if (receiverId === ctx.sender) return; // Can't thank yourself

    this.grantRep(ctx.sender, receiverId, ctx.roomId, ctx.eventId);
  }

  private grantRep(giverId: string, receiverId: string, roomId: string, eventId: string): void {
    const db = getDb();

    // Check 24h cooldown
    const cooldown = db
      .prepare(
        `SELECT granted_at FROM rep_cooldowns
         WHERE giver_id = ? AND receiver_id = ? AND room_id = ?`
      )
      .get(giverId, receiverId, roomId) as { granted_at: string } | undefined;

    if (cooldown) {
      const grantedAt = new Date(cooldown.granted_at + "Z").getTime();
      if (Date.now() - grantedAt < COOLDOWN_HOURS * 60 * 60 * 1000) return;
    }

    // Update or insert cooldown
    db.prepare(
      `INSERT INTO rep_cooldowns (giver_id, receiver_id, room_id, granted_at)
       VALUES (?, ?, ?, datetime('now'))
       ON CONFLICT(giver_id, receiver_id, room_id) DO UPDATE SET granted_at = datetime('now')`
    ).run(giverId, receiverId, roomId);

    // Grant rep
    db.prepare(
      `INSERT INTO users (user_id, room_id, rep)
       VALUES (?, ?, 1)
       ON CONFLICT(user_id, room_id) DO UPDATE SET rep = rep + 1`
    ).run(receiverId, roomId);

    // Bonus XP
    this.xpPlugin.grantXp(receiverId, roomId, REP_XP_BONUS, "reputation");

    // React with checkmark to acknowledge
    this.sendReact(roomId, eventId, "\u2705");

    logger.debug(`${giverId} gave rep to ${receiverId} in ${roomId}`);
  }

  private async handleRep(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "rep");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT rep FROM users WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { rep: number } | undefined;

    const rep = row?.rep ?? 0;
    await this.sendMessage(ctx.roomId, `${targetUser} has ${rep} reputation point${rep !== 1 ? "s" : ""}.`);
  }

  private async handleRepboard(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, rep FROM users WHERE room_id = ? AND rep > 0 ORDER BY rep DESC LIMIT 10`)
      .all(ctx.roomId) as { user_id: string; rep: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No reputation data yet.");
      return;
    }

    const lines = rows.map((r, i) => `${i + 1}. ${r.user_id} — ${r.rep} rep`);
    await this.sendMessage(ctx.roomId, `Reputation Leaderboard:\n${lines.join("\n")}`);
  }
}
