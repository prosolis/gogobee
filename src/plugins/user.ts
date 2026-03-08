import { IMatrixClient, Plugin, CommandDef, MessageContext, ReactionContext } from "./base";
import { getDb } from "../db";
import { DateTime, IANAZone } from "luxon";
import logger from "../utils/logger";

// Common city → IANA timezone mapping
const CITY_TIMEZONE_MAP: Record<string, string> = {
  "new york": "America/New_York",
  "nyc": "America/New_York",
  "los angeles": "America/Los_Angeles",
  "la": "America/Los_Angeles",
  "chicago": "America/Chicago",
  "denver": "America/Denver",
  "phoenix": "America/Phoenix",
  "london": "Europe/London",
  "paris": "Europe/Paris",
  "berlin": "Europe/Berlin",
  "tokyo": "Asia/Tokyo",
  "sydney": "Australia/Sydney",
  "melbourne": "Australia/Melbourne",
  "auckland": "Pacific/Auckland",
  "toronto": "America/Toronto",
  "vancouver": "America/Vancouver",
  "mumbai": "Asia/Kolkata",
  "delhi": "Asia/Kolkata",
  "dubai": "Asia/Dubai",
  "singapore": "Asia/Singapore",
  "hong kong": "Asia/Hong_Kong",
  "seoul": "Asia/Seoul",
  "beijing": "Asia/Shanghai",
  "shanghai": "Asia/Shanghai",
  "moscow": "Europe/Moscow",
  "istanbul": "Europe/Istanbul",
  "cairo": "Africa/Cairo",
  "johannesburg": "Africa/Johannesburg",
  "sao paulo": "America/Sao_Paulo",
  "mexico city": "America/Mexico_City",
  "amsterdam": "Europe/Amsterdam",
  "rome": "Europe/Rome",
  "madrid": "Europe/Madrid",
  "lisbon": "Europe/Lisbon",
  "bangkok": "Asia/Bangkok",
  "jakarta": "Asia/Jakarta",
  "manila": "Asia/Manila",
  "kuala lumpur": "Asia/Kuala_Lumpur",
  "taipei": "Asia/Taipei",
  "oslo": "Europe/Oslo",
  "stockholm": "Europe/Stockholm",
  "helsinki": "Europe/Helsinki",
  "warsaw": "Europe/Warsaw",
  "prague": "Europe/Prague",
  "vienna": "Europe/Vienna",
  "zurich": "Europe/Zurich",
  "athens": "Europe/Athens",
  "honolulu": "Pacific/Honolulu",
  "anchorage": "America/Anchorage",
  "hawaii": "Pacific/Honolulu",
};

/** Try to resolve a city name to an IANA timezone by checking Region/City patterns. */
function guessIANAZone(city: string): string | null {
  const formatted = city
    .split(/[\s-]+/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join("_");

  const regions = ["Europe", "America", "Asia", "Africa", "Pacific", "Australia", "Atlantic", "Indian", "Arctic"];
  for (const region of regions) {
    const candidate = `${region}/${formatted}`;
    if (IANAZone.isValidZone(candidate)) return candidate;
  }
  return null;
}

export class UserPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "user";
  }

  get commands(): CommandDef[] {
    return [
      { name: "settz", description: "Set your timezone", usage: "!settz <city|IANA>" },
      { name: "mytz", description: "Show your timezone" },
      { name: "timezone list", description: "World clock for everyone in the room" },
      { name: "quote", description: "Random starred quote", usage: "!quote [@user]" },
      { name: "np", description: "Now playing", usage: "!np [game|@user|list]" },
      { name: "backlog", description: "Game backlog", usage: "!backlog [add|list|random|done] [game]" },
      { name: "watch", description: "Keyword DM alert", usage: "!watch <keyword>" },
      { name: "watching", description: "List your keyword watches" },
      { name: "unwatch", description: "Remove a keyword watch", usage: "!unwatch <keyword|id>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: check keyword watches
    this.checkKeywordWatches(ctx);

    if (this.isCommand(ctx.body, "timezone list")) {
      await this.handleTimezoneList(ctx);
    } else if (this.isCommand(ctx.body, "settz")) {
      await this.handleSetTz(ctx);
    } else if (this.isCommand(ctx.body, "mytz")) {
      await this.handleMyTz(ctx);
    } else if (this.isCommand(ctx.body, "quote")) {
      await this.handleQuote(ctx);
    } else if (this.isCommand(ctx.body, "np")) {
      await this.handleNp(ctx);
    } else if (this.isCommand(ctx.body, "backlog")) {
      await this.handleBacklog(ctx);
    } else if (this.isCommand(ctx.body, "unwatch")) {
      await this.handleUnwatch(ctx);
    } else if (this.isCommand(ctx.body, "watching")) {
      await this.handleWatching(ctx);
    } else if (this.isCommand(ctx.body, "watch")) {
      await this.handleWatch(ctx);
    }
  }

  async onReaction(ctx: ReactionContext): Promise<void> {
    // Star reactions save quotes
    if (ctx.reactionKey === "\u2B50" || ctx.reactionKey === "\u2B50\uFE0F") {
      await this.saveQuote(ctx);
    }
  }

  private checkKeywordWatches(ctx: MessageContext): void {
    const db = getDb();
    const watches = db
      .prepare(`SELECT user_id, keyword FROM keyword_watches WHERE room_id = ?`)
      .all(ctx.roomId) as { user_id: string; keyword: string }[];

    const bodyLower = ctx.body.toLowerCase();

    for (const watch of watches) {
      if (watch.user_id === ctx.sender) continue; // Don't alert on own messages
      if (bodyLower.includes(watch.keyword.toLowerCase())) {
        this.sendDm(
          watch.user_id,
          `Keyword alert "${watch.keyword}" triggered by ${ctx.sender} in ${ctx.roomId}:\n${ctx.body.slice(0, 200)}`
        ).catch((err) => logger.error(`Failed keyword DM: ${err}`));
      }
    }
  }

  private resolveTimezone(input: string): string | null {
    const lower = input.toLowerCase().trim();

    // Check city map first
    if (CITY_TIMEZONE_MAP[lower]) {
      return CITY_TIMEZONE_MAP[lower];
    }

    // Try direct IANA validation
    if (IANAZone.isValidZone(input.trim())) {
      return input.trim();
    }

    // Try guessing from IANA zone database (e.g., "Lisbon" → "Europe/Lisbon")
    return guessIANAZone(lower);
  }

  private async handleTimezoneList(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, timezone FROM users WHERE room_id = ? AND timezone IS NOT NULL`)
      .all(ctx.roomId) as { user_id: string; timezone: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No one in this room has set a timezone yet. Use !settz to set yours.");
      return;
    }

    // Group users by timezone, sort by UTC offset
    const groups = new Map<string, string[]>();
    for (const row of rows) {
      const list = groups.get(row.timezone) ?? [];
      list.push(row.user_id);
      groups.set(row.timezone, list);
    }

    const entries = [...groups.entries()].map(([tz, users]) => {
      const dt = DateTime.now().setZone(tz);
      return { tz, users, offset: dt.offset, time: dt.toFormat("HH:mm (EEE)") };
    }).sort((a, b) => a.offset - b.offset);

    const lines = entries.map((e) => {
      const userList = e.users.join(", ");
      return `${e.time} — ${e.tz}\n  ${userList}`;
    });

    await this.sendMessage(ctx.roomId, `World Clock:\n\n${lines.join("\n\n")}`);
  }

  private async handleSetTz(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "settz");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !settz <city or IANA timezone>\nExamples: !settz Tokyo, !settz America/New_York");
      return;
    }

    const tz = this.resolveTimezone(args);
    if (!tz) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Unknown timezone "${args}". Try a city name or IANA zone (e.g., America/New_York).`);
      return;
    }

    const db = getDb();
    db.prepare(`
      INSERT INTO users (user_id, room_id, timezone)
      VALUES (?, ?, ?)
      ON CONFLICT(user_id, room_id) DO UPDATE SET timezone = ?
    `).run(ctx.sender, ctx.roomId, tz, tz);

    const now = DateTime.now().setZone(tz);
    await this.sendReply(ctx.roomId, ctx.eventId, `Timezone set to ${tz}. Your current time: ${now.toFormat("HH:mm (ZZZZZ)")}`);
  }

  private async handleMyTz(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const row = db
      .prepare(`SELECT timezone FROM users WHERE user_id = ? AND room_id = ?`)
      .get(ctx.sender, ctx.roomId) as { timezone: string | null } | undefined;

    if (!row?.timezone) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You haven't set a timezone yet. Use !settz <city or IANA zone>.");
      return;
    }

    const now = DateTime.now().setZone(row.timezone);
    await this.sendMessage(ctx.roomId, `${ctx.sender}: ${row.timezone} — ${now.toFormat("HH:mm (ZZZZZ)")}`);
  }

  private async saveQuote(ctx: ReactionContext): Promise<void> {
    try {
      const event = await this.client.getEvent(ctx.roomId, ctx.targetEventId);
      const body = event?.content?.body;
      if (!body) return;

      const db = getDb();
      // Avoid duplicate quotes for the same event
      const existing = db
        .prepare(`SELECT id FROM quotes WHERE event_id = ? AND room_id = ?`)
        .get(ctx.targetEventId, ctx.roomId);
      if (existing) return;

      db.prepare(`INSERT INTO quotes (user_id, room_id, message, quoted_by, event_id) VALUES (?, ?, ?, ?, ?)`).run(
        event.sender,
        ctx.roomId,
        body,
        ctx.sender,
        ctx.targetEventId
      );

      logger.debug(`Quote saved from ${event.sender} by ${ctx.sender}`);
    } catch (err) {
      logger.error(`Failed to save quote: ${err}`);
    }
  }

  private async handleQuote(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "quote");
    const db = getDb();

    let row: { user_id: string; message: string } | undefined;
    if (args.startsWith("@")) {
      const targetUser = args.split(/\s/)[0];
      row = db
        .prepare(`SELECT user_id, message FROM quotes WHERE user_id = ? AND room_id = ? ORDER BY RANDOM() LIMIT 1`)
        .get(targetUser, ctx.roomId) as typeof row;
    } else {
      row = db
        .prepare(`SELECT user_id, message FROM quotes WHERE room_id = ? ORDER BY RANDOM() LIMIT 1`)
        .get(ctx.roomId) as typeof row;
    }

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No quotes found. Star a message with a reaction to save it!");
      return;
    }

    await this.sendMessage(ctx.roomId, `"${row.message}" — ${row.user_id}`);
  }

  private async handleNp(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "np");
    const db = getDb();

    if (!args) {
      // Show current now-playing
      const row = db
        .prepare(`SELECT game FROM now_playing WHERE user_id = ? AND room_id = ?`)
        .get(ctx.sender, ctx.roomId) as { game: string } | undefined;

      if (!row) {
        await this.sendReply(ctx.roomId, ctx.eventId, "You're not playing anything. Use !np <game> to set it.");
        return;
      }
      await this.sendMessage(ctx.roomId, `${ctx.sender} is playing: ${row.game}`);
      return;
    }

    if (args.startsWith("@")) {
      const targetUser = args.split(/\s/)[0];
      const row = db
        .prepare(`SELECT game FROM now_playing WHERE user_id = ? AND room_id = ?`)
        .get(targetUser, ctx.roomId) as { game: string } | undefined;

      if (!row) {
        await this.sendMessage(ctx.roomId, `${targetUser} isn't playing anything.`);
        return;
      }
      await this.sendMessage(ctx.roomId, `${targetUser} is playing: ${row.game}`);
      return;
    }

    if (args === "list") {
      const rows = db
        .prepare(`SELECT user_id, game FROM now_playing WHERE room_id = ?`)
        .all(ctx.roomId) as { user_id: string; game: string }[];

      if (rows.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Nobody is playing anything right now.");
        return;
      }

      const lines = rows.map((r) => `${r.user_id}: ${r.game}`);
      await this.sendMessage(ctx.roomId, `Now Playing:\n${lines.join("\n")}`);
      return;
    }

    // Set now playing
    db.prepare(`
      INSERT INTO now_playing (user_id, room_id, game)
      VALUES (?, ?, ?)
      ON CONFLICT(user_id, room_id) DO UPDATE SET game = ?, started_at = datetime('now')
    `).run(ctx.sender, ctx.roomId, args, args);

    await this.sendReply(ctx.roomId, ctx.eventId, `Now playing: ${args}`);
  }

  private async handleBacklog(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "backlog");
    const parts = args.split(/\s+/);
    const subcommand = parts[0]?.toLowerCase() || "list";
    const game = parts.slice(1).join(" ");
    const db = getDb();

    switch (subcommand) {
      case "add": {
        if (!game) {
          await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !backlog add <game name>");
          return;
        }
        db.prepare(`
          INSERT OR IGNORE INTO backlog (user_id, room_id, game) VALUES (?, ?, ?)
        `).run(ctx.sender, ctx.roomId, game);
        await this.sendReply(ctx.roomId, ctx.eventId, `Added "${game}" to your backlog.`);
        break;
      }
      case "done": {
        if (!game) {
          await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !backlog done <game name>");
          return;
        }
        const result = db
          .prepare(`UPDATE backlog SET completed = 1, completed_at = datetime('now') WHERE user_id = ? AND room_id = ? AND game = ? AND completed = 0`)
          .run(ctx.sender, ctx.roomId, game);
        if (result.changes > 0) {
          await this.sendReply(ctx.roomId, ctx.eventId, `Marked "${game}" as completed!`);
        } else {
          await this.sendReply(ctx.roomId, ctx.eventId, `"${game}" not found in your active backlog.`);
        }
        break;
      }
      case "random": {
        const row = db
          .prepare(`SELECT game FROM backlog WHERE user_id = ? AND room_id = ? AND completed = 0 ORDER BY RANDOM() LIMIT 1`)
          .get(ctx.sender, ctx.roomId) as { game: string } | undefined;
        if (!row) {
          await this.sendReply(ctx.roomId, ctx.eventId, "Your backlog is empty!");
          return;
        }
        await this.sendMessage(ctx.roomId, `Random pick from your backlog: ${row.game}`);
        break;
      }
      case "list":
      default: {
        const rows = db
          .prepare(`SELECT game, completed FROM backlog WHERE user_id = ? AND room_id = ? ORDER BY completed ASC, added_at DESC`)
          .all(ctx.sender, ctx.roomId) as { game: string; completed: number }[];

        if (rows.length === 0) {
          await this.sendReply(ctx.roomId, ctx.eventId, "Your backlog is empty. Use !backlog add <game> to start.");
          return;
        }

        const lines = rows.map((r) => `${r.completed ? "[done]" : "[ ]"} ${r.game}`);
        await this.sendMessage(ctx.roomId, `${ctx.sender}'s backlog:\n${lines.join("\n")}`);
        break;
      }
    }
  }

  private async handleWatch(ctx: MessageContext): Promise<void> {
    const keyword = this.getArgs(ctx.body, "watch").trim();
    if (!keyword) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !watch <keyword>");
      return;
    }

    const db = getDb();
    try {
      db.prepare(`INSERT INTO keyword_watches (user_id, room_id, keyword) VALUES (?, ?, ?)`).run(
        ctx.sender,
        ctx.roomId,
        keyword
      );
      await this.sendReply(ctx.roomId, ctx.eventId, `Watching for "${keyword}". You'll get a DM when someone mentions it.`);
    } catch {
      await this.sendReply(ctx.roomId, ctx.eventId, `You're already watching for "${keyword}".`);
    }
  }

  private async handleWatching(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT id, keyword FROM keyword_watches WHERE user_id = ? AND room_id = ?`)
      .all(ctx.sender, ctx.roomId) as { id: number; keyword: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You're not watching any keywords. Use !watch <keyword> to start.");
      return;
    }

    const lines = rows.map((r) => `[${r.id}] ${r.keyword}`);
    await this.sendMessage(ctx.roomId, `Your keyword watches:\n${lines.join("\n")}`);
  }

  private async handleUnwatch(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "unwatch").trim();
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !unwatch <keyword|id>");
      return;
    }

    const db = getDb();
    // Try by ID first
    const idNum = parseInt(args);
    let result;
    if (!isNaN(idNum)) {
      result = db
        .prepare(`DELETE FROM keyword_watches WHERE id = ? AND user_id = ? AND room_id = ?`)
        .run(idNum, ctx.sender, ctx.roomId);
    }

    if (!result || result.changes === 0) {
      // Try by keyword
      result = db
        .prepare(`DELETE FROM keyword_watches WHERE keyword = ? AND user_id = ? AND room_id = ?`)
        .run(args, ctx.sender, ctx.roomId);
    }

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Removed watch for "${args}".`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `No watch found matching "${args}".`);
    }
  }
}
