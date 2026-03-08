import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

export class GamingPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "gaming";
  }

  get commands(): CommandDef[] {
    return [
      { name: "releases", description: "Upcoming game releases", usage: "!releases [week|month|search <game>]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "releases")) {
      await this.handleReleases(ctx);
    }
  }

  private async handleReleases(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "releases").trim();

    if (args.startsWith("search ")) {
      const query = args.slice(7).trim();
      await this.handleSearch(ctx, query);
      return;
    }

    const apiKey = process.env.RAWG_API_KEY;
    if (!apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "RAWG API key not configured.");
      return;
    }

    const today = new Date();
    const startDate = today.toISOString().slice(0, 10);

    let endDate: string;
    let label: string;
    if (args === "month") {
      const end = new Date(today);
      end.setDate(end.getDate() + 30);
      endDate = end.toISOString().slice(0, 10);
      label = "This Month";
    } else if (args === "week") {
      const end = new Date(today);
      end.setDate(end.getDate() + 7);
      endDate = end.toISOString().slice(0, 10);
      label = "This Week";
    } else {
      // Default: today
      endDate = startDate;
      label = "Today";
    }

    try {
      const url = `https://api.rawg.io/api/games?key=${encodeURIComponent(apiKey)}&dates=${startDate},${endDate}&ordering=-rating&page_size=10`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch releases from RAWG.");
        return;
      }

      const data = await res.json() as any;
      const games = data.results ?? [];

      if (games.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No releases found for ${label.toLowerCase()}.`);
        return;
      }

      const lines = [`Game Releases — ${label}:`];
      for (const game of games) {
        const platforms = (game.platforms ?? []).map((p: any) => p.platform?.name).filter(Boolean).join(", ");
        const hltbInfo = this.getHltbInfo(game.name);
        let line = `- ${game.name} (${game.released ?? "TBA"})`;
        if (platforms) line += ` [${platforms}]`;
        if (hltbInfo) line += ` | ${hltbInfo}`;
        lines.push(line);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`Releases fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch releases. Try again later.");
    }
  }

  private async handleSearch(ctx: MessageContext, query: string): Promise<void> {
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !releases search <game name>");
      return;
    }

    const apiKey = process.env.RAWG_API_KEY;
    if (!apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "RAWG API key not configured.");
      return;
    }

    try {
      const url = `https://api.rawg.io/api/games?key=${encodeURIComponent(apiKey)}&search=${encodeURIComponent(query)}&page_size=5`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "RAWG search failed.");
        return;
      }

      const data = await res.json() as any;
      const games = data.results ?? [];

      if (games.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No games found for "${query}".`);
        return;
      }

      const lines = [`Search results for "${query}":`];
      for (const game of games) {
        const platforms = (game.platforms ?? []).map((p: any) => p.platform?.name).filter(Boolean).join(", ");
        let line = `- ${game.name} (${game.released ?? "TBA"})`;
        if (platforms) line += ` [${platforms}]`;
        if (game.rating) line += ` Rating: ${game.rating}/5`;
        lines.push(line);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`RAWG search failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Search failed. Try again later.");
    }
  }

  private getHltbInfo(gameName: string): string | null {
    try {
      const db = getDb();
      const row = db
        .prepare(`SELECT main_story, main_extra FROM hltb_cache WHERE game_name = ? AND fetched_at > datetime('now', '-24 hours') LIMIT 1`)
        .get(gameName) as { main_story: number | null; main_extra: number | null } | undefined;

      if (!row) return null;

      const parts: string[] = [];
      if (row.main_story) parts.push(`Main: ${row.main_story}h`);
      if (row.main_extra) parts.push(`Extra: ${row.main_extra}h`);
      return parts.length > 0 ? `HLTB ${parts.join(", ")}` : null;
    } catch {
      return null;
    }
  }

  /**
   * Called by DailyScheduler to post the evening releases summary.
   */
  async postReleases(roomIds: string[]): Promise<void> {
    const apiKey = process.env.RAWG_API_KEY;
    if (!apiKey) {
      logger.warn("RAWG_API_KEY not set, skipping releases post");
      return;
    }

    const today = new Date().toISOString().slice(0, 10);

    try {
      const url = `https://api.rawg.io/api/games?key=${encodeURIComponent(apiKey)}&dates=${today},${today}&ordering=-rating&page_size=10`;
      const res = await fetch(url);
      if (!res.ok) {
        logger.warn(`RAWG API returned ${res.status}`);
        return;
      }

      const data = await res.json() as any;
      const games = data.results ?? [];

      if (games.length === 0) return;

      const lines = ["Today's Game Releases:"];
      for (const game of games) {
        const platforms = (game.platforms ?? []).map((p: any) => p.platform?.name).filter(Boolean).join(", ");
        const hltbInfo = this.getHltbInfo(game.name);
        let line = `- ${game.name}`;
        if (platforms) line += ` [${platforms}]`;
        if (hltbInfo) line += ` | ${hltbInfo}`;
        lines.push(line);

        // Cache the release data
        const db = getDb();
        db.prepare(`
          INSERT INTO releases_cache (game_name, game_slug, release_date, platforms, genres, rating, data_json)
          VALUES (?, ?, ?, ?, ?, ?, ?)
        `).run(
          game.name,
          game.slug ?? null,
          game.released ?? null,
          platforms || null,
          (game.genres ?? []).map((g: any) => g.name).join(", ") || null,
          game.rating ?? null,
          JSON.stringify(game)
        );
      }

      const message = lines.join("\n");

      for (const roomId of roomIds) {
        try {
          await this.sendMessage(roomId, message);
        } catch (err) {
          logger.error(`Failed to post releases to ${roomId}: ${err}`);
        }
      }

      // Check watchlist notifications
      await this.checkWatchlistNotifications(games);
    } catch (err) {
      logger.error(`Releases daily post failed: ${err}`);
    }
  }

  private async checkWatchlistNotifications(games: any[]): Promise<void> {
    const db = getDb();

    for (const game of games) {
      const watchers = db
        .prepare(`SELECT user_id, room_id FROM release_watchlist WHERE (game_name = ? OR game_slug = ?) AND notified_day_of = 0`)
        .all(game.name, game.slug ?? "") as { user_id: string; room_id: string }[];

      for (const watcher of watchers) {
        await this.sendDm(watcher.user_id, `${game.name} releases today!`);
        db.prepare(`UPDATE release_watchlist SET notified_day_of = 1 WHERE user_id = ? AND room_id = ? AND game_name = ?`).run(
          watcher.user_id,
          watcher.room_id,
          game.name
        );
      }
    }
  }
}
