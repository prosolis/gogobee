import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

export class MarkovPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "markov";
  }

  get commands(): CommandDef[] {
    return [
      { name: "markov", description: "Generate a Markov chain sentence", usage: "!markov [@user|me]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: collect corpus from non-command messages
    if (!ctx.body.startsWith(this.prefix)) {
      this.recordMessage(ctx);
    }

    if (this.isCommand(ctx.body, "markov")) {
      await this.handleMarkov(ctx);
    }
  }

  private recordMessage(ctx: MessageContext): void {
    const db = getDb();
    const now = Math.floor(Date.now() / 1000);

    db.prepare(
      `INSERT INTO markov_corpus (user_id, room_id, message, added_at) VALUES (?, ?, ?, ?)`
    ).run(ctx.sender, ctx.roomId, ctx.body, now);

    const row = db
      .prepare(`SELECT COUNT(*) as cnt FROM markov_corpus WHERE user_id = ? AND room_id = ?`)
      .get(ctx.sender, ctx.roomId) as { cnt: number };

    if (row.cnt > 10000) {
      db.prepare(
        `DELETE FROM markov_corpus WHERE id IN (
          SELECT id FROM markov_corpus WHERE user_id = ? AND room_id = ? ORDER BY id ASC LIMIT 1000
        )`
      ).run(ctx.sender, ctx.roomId);
    }
  }

  private async handleMarkov(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "markov");
    const db = getDb();

    let messages: { message: string }[];
    let label: string;

    if (!args) {
      // Room-wide combined corpus
      messages = db
        .prepare(`SELECT message FROM markov_corpus WHERE room_id = ?`)
        .all(ctx.roomId) as { message: string }[];
      label = "room";
    } else if (args === "me") {
      messages = db
        .prepare(`SELECT message FROM markov_corpus WHERE user_id = ? AND room_id = ?`)
        .all(ctx.sender, ctx.roomId) as { message: string }[];
      label = "user";
    } else if (args.startsWith("@")) {
      const targetUser = args.split(/\s/)[0];
      messages = db
        .prepare(`SELECT message FROM markov_corpus WHERE user_id = ? AND room_id = ?`)
        .all(targetUser, ctx.roomId) as { message: string }[];
      label = "user";
    } else {
      // Treat unknown arg as a user mention without @
      return;
    }

    if (messages.length < 50) {
      const reply =
        label === "room"
          ? "Not enough data in this room yet."
          : `Not enough data to impersonate ${args === "me" ? ctx.sender : args.split(/\s/)[0]} yet.`;
      await this.sendReply(ctx.roomId, ctx.eventId, reply);
      return;
    }

    const sentence = this.generateSentence(messages);
    if (!sentence) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't generate anything. Try again later.");
      return;
    }

    await this.sendMessage(ctx.roomId, sentence);
  }

  private generateSentence(messages: { message: string }[]): string | null {
    const transitions = new Map<string, string[]>();
    const starters: string[] = [];

    for (const { message } of messages) {
      const words = message.split(/\s+/).filter((w) => w.length > 0);
      if (words.length < 3) continue;

      const starterKey = `${words[0]} ${words[1]}`;
      starters.push(starterKey);

      for (let i = 0; i < words.length - 2; i++) {
        const key = `${words[i]} ${words[i + 1]}`;
        const next = words[i + 2];
        let list = transitions.get(key);
        if (!list) {
          list = [];
          transitions.set(key, list);
        }
        list.push(next);
      }
    }

    if (starters.length === 0) return null;

    const startKey = starters[Math.floor(Math.random() * starters.length)];
    const result = startKey.split(" ");

    for (let i = 0; i < 48; i++) {
      const key = `${result[result.length - 2]} ${result[result.length - 1]}`;
      const candidates = transitions.get(key);
      if (!candidates || candidates.length === 0) break;

      const next = candidates[Math.floor(Math.random() * candidates.length)];
      result.push(next);

      if (/[.!?]$/.test(next)) break;
    }

    return result.join(" ");
  }
}
