import { IMatrixClient, Plugin, CommandDef, MessageContext, ReactionContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

export class ReactionsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "reactions";
  }

  get commands(): CommandDef[] {
    return [
      { name: "emojiboard", description: "Reaction leaderboard" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "emojiboard")) {
      await this.handleEmojiboard(ctx);
    }
  }

  async onReaction(ctx: ReactionContext): Promise<void> {
    if (!ctx.reactionKey || !ctx.targetEventId) return;

    try {
      const targetEvent = await this.client.getEvent(ctx.roomId, ctx.targetEventId);
      const receiverId: string = targetEvent?.sender;
      if (!receiverId || receiverId === ctx.sender) return; // Don't track self-reactions

      const db = getDb();
      db.prepare(
        `INSERT INTO reaction_log (giver_id, receiver_id, room_id, emoji, event_id)
         VALUES (?, ?, ?, ?, ?)`
      ).run(ctx.sender, receiverId, ctx.roomId, ctx.reactionKey, ctx.eventId);
    } catch {
      // Event may not be found (deleted, redacted, or not yet decrypted) — silently skip
    }
  }

  private async handleEmojiboard(ctx: MessageContext): Promise<void> {
    const db = getDb();

    const topGivers = db.prepare(
      `SELECT giver_id, COUNT(*) as count FROM reaction_log
       WHERE room_id = ? GROUP BY giver_id ORDER BY count DESC LIMIT 5`
    ).all(ctx.roomId) as { giver_id: string; count: number }[];

    const topReceivers = db.prepare(
      `SELECT receiver_id, COUNT(*) as count FROM reaction_log
       WHERE room_id = ? GROUP BY receiver_id ORDER BY count DESC LIMIT 5`
    ).all(ctx.roomId) as { receiver_id: string; count: number }[];

    const topEmojis = db.prepare(
      `SELECT emoji, COUNT(*) as count FROM reaction_log
       WHERE room_id = ? GROUP BY emoji ORDER BY count DESC LIMIT 5`
    ).all(ctx.roomId) as { emoji: string; count: number }[];

    if (topGivers.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No reaction data yet.");
      return;
    }

    const giverLines = topGivers.map((r, i) => `${i + 1}. ${r.giver_id} — ${r.count}`);
    const receiverLines = topReceivers.map((r, i) => `${i + 1}. ${r.receiver_id} — ${r.count}`);
    const emojiLines = topEmojis.map((r, i) => `${i + 1}. ${r.emoji} — ${r.count}`);

    const msg = [
      "Reaction Leaderboard:",
      "",
      "Most Reactions Given:",
      ...giverLines,
      "",
      "Most Reactions Received:",
      ...receiverLines,
      "",
      "Most Used Emoji:",
      ...emojiLines,
    ].join("\n");

    await this.sendMessage(ctx.roomId, msg);
  }
}
