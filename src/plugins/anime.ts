import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

interface JikanAnime {
  mal_id: number;
  title: string;
  score: number | null;
  episodes: number | null;
  status: string | null;
  synopsis: string | null;
  genres: { name: string }[];
  broadcast: { day: string | null; time: string | null; string: string | null } | null;
  aired: { from: string | null; to: string | null; string: string | null } | null;
}

async function jikanDelay(): Promise<void> {
  await new Promise((r) => setTimeout(r, 400));
}

export class AnimePlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "anime";
  }

  get commands(): CommandDef[] {
    return [
      { name: "anime search", description: "Search anime by title", usage: "!anime search <title>" },
      { name: "anime watch", description: "Add anime to your watchlist", usage: "!anime watch <title>" },
      { name: "anime watching", description: "List your anime watchlist", usage: "!anime watching" },
      { name: "anime unwatch", description: "Remove anime from watchlist", usage: "!anime unwatch <title|id>" },
      { name: "anime season", description: "Current season top 10 by score", usage: "!anime season" },
      { name: "anime upcoming", description: "Next season preview top 10", usage: "!anime upcoming" },
      { name: "anime", description: "Get anime details", usage: "!anime <title or MAL ID>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "anime search ")) {
      await this.handleSearch(ctx);
    } else if (this.isCommand(ctx.body, "anime watch ")) {
      await this.handleWatch(ctx);
    } else if (this.isCommand(ctx.body, "anime watching")) {
      await this.handleWatching(ctx);
    } else if (this.isCommand(ctx.body, "anime unwatch ")) {
      await this.handleUnwatch(ctx);
    } else if (this.isCommand(ctx.body, "anime season")) {
      await this.handleSeason(ctx);
    } else if (this.isCommand(ctx.body, "anime upcoming")) {
      await this.handleUpcoming(ctx);
    } else if (this.isCommand(ctx.body, "anime ")) {
      await this.handleDetails(ctx);
    }
  }

  // ── Jikan helpers ──────────────────────────────────────────────────

  private getCachedAnime(malId: number): JikanAnime | null {
    const db = getDb();
    const row = db
      .prepare(`SELECT data FROM anime_cache WHERE mal_id = ? AND cached_at > unixepoch() - 86400`)
      .get(malId) as { data: string } | undefined;

    if (!row) return null;
    try {
      return JSON.parse(row.data) as JikanAnime;
    } catch {
      return null;
    }
  }

  private cacheAnime(anime: JikanAnime): void {
    const db = getDb();
    db.prepare(
      `INSERT INTO anime_cache (mal_id, data, cached_at) VALUES (?, ?, unixepoch())
       ON CONFLICT(mal_id) DO UPDATE SET data = excluded.data, cached_at = excluded.cached_at`
    ).run(anime.mal_id, JSON.stringify(anime));
  }

  private async fetchAnimeById(malId: number): Promise<JikanAnime | null> {
    const cached = this.getCachedAnime(malId);
    if (cached) return cached;

    try {
      await jikanDelay();
      const res = await fetch(`https://api.jikan.moe/v4/anime/${malId}`);
      if (!res.ok) return null;

      const json = (await res.json()) as any;
      const anime = json.data as JikanAnime;
      if (!anime) return null;

      this.cacheAnime(anime);
      return anime;
    } catch (err) {
      logger.error(`Jikan fetch by ID ${malId} failed: ${err}`);
      return null;
    }
  }

  private async searchAnime(query: string, limit = 3): Promise<JikanAnime[]> {
    try {
      await jikanDelay();
      const url = `https://api.jikan.moe/v4/anime?q=${encodeURIComponent(query)}&limit=${limit}`;
      const res = await fetch(url);
      if (!res.ok) return [];

      const json = (await res.json()) as any;
      const results = (json.data ?? []) as JikanAnime[];

      for (const anime of results) {
        this.cacheAnime(anime);
      }

      return results;
    } catch (err) {
      logger.error(`Jikan search failed for "${query}": ${err}`);
      return [];
    }
  }

  // ── Command handlers ───────────────────────────────────────────────

  private async handleSearch(ctx: MessageContext): Promise<void> {
    const query = this.getArgs(ctx.body, "anime search").trim();
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !anime search <title>");
      return;
    }

    const results = await this.searchAnime(query, 3);
    if (results.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No anime found for "${query}".`);
      return;
    }

    const lines = [`Search results for "${query}":`];
    for (const anime of results) {
      const score = anime.score != null ? anime.score.toFixed(2) : "N/A";
      const episodes = anime.episodes != null ? String(anime.episodes) : "?";
      const status = anime.status ?? "Unknown";
      lines.push(`[${anime.mal_id}] ${anime.title} — Score: ${score} | Episodes: ${episodes} | Status: ${status}`);
    }

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private async handleDetails(ctx: MessageContext): Promise<void> {
    const query = this.getArgs(ctx.body, "anime").trim();
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !anime <title or MAL ID>");
      return;
    }

    let anime: JikanAnime | null = null;

    if (/^\d+$/.test(query)) {
      anime = await this.fetchAnimeById(parseInt(query, 10));
    } else {
      const results = await this.searchAnime(query, 1);
      if (results.length > 0) {
        anime = results[0];
      }
    }

    if (!anime) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No anime found for "${query}".`);
      return;
    }

    const score = anime.score != null ? anime.score.toFixed(2) : "N/A";
    const episodes = anime.episodes != null ? String(anime.episodes) : "?";
    const genres = anime.genres.map((g) => g.name).join(", ") || "N/A";
    let synopsis = anime.synopsis ?? "No synopsis available.";
    if (synopsis.length > 300) {
      synopsis = synopsis.slice(0, 297) + "...";
    }

    const status = anime.status ?? "Unknown";
    let airingInfo = `Status: ${status}`;
    if (anime.broadcast?.string) {
      airingInfo += ` | Broadcast: ${anime.broadcast.string}`;
    } else if (anime.aired?.string) {
      airingInfo += ` | Aired: ${anime.aired.string}`;
    }

    const lines = [
      `${anime.title} [MAL ${anime.mal_id}]`,
      `Score: ${score} | Episodes: ${episodes}`,
      `Genres: ${genres}`,
      airingInfo,
      ``,
      synopsis,
    ];

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private async handleWatch(ctx: MessageContext): Promise<void> {
    const query = this.getArgs(ctx.body, "anime watch").trim();
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !anime watch <title>");
      return;
    }

    const results = await this.searchAnime(query, 1);
    if (results.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No anime found for "${query}".`);
      return;
    }

    const anime = results[0];
    const db = getDb();

    const existing = db
      .prepare(`SELECT id FROM anime_watchlist WHERE user_id = ? AND room_id = ? AND mal_id = ?`)
      .get(ctx.sender, ctx.roomId, anime.mal_id);

    if (existing) {
      await this.sendReply(ctx.roomId, ctx.eventId, `"${anime.title}" is already on your watchlist.`);
      return;
    }

    const airingDate = anime.aired?.from ?? null;

    db.prepare(
      `INSERT INTO anime_watchlist (user_id, room_id, mal_id, title, airing_date) VALUES (?, ?, ?, ?, ?)`
    ).run(ctx.sender, ctx.roomId, anime.mal_id, anime.title, airingDate);

    await this.sendReply(ctx.roomId, ctx.eventId, `Added "${anime.title}" to your watchlist.`);
  }

  private async handleWatching(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT mal_id, title FROM anime_watchlist WHERE user_id = ? AND room_id = ? ORDER BY created_at DESC`)
      .all(ctx.sender, ctx.roomId) as { mal_id: number; title: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Your anime watchlist is empty.");
      return;
    }

    const lines = ["Your anime watchlist:"];
    for (const row of rows) {
      const cached = this.getCachedAnime(row.mal_id);
      const status = cached?.status ?? "Unknown";
      lines.push(`[${row.mal_id}] ${row.title} — ${status}`);
    }

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  private async handleUnwatch(ctx: MessageContext): Promise<void> {
    const query = this.getArgs(ctx.body, "anime unwatch").trim();
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !anime unwatch <title|id>");
      return;
    }

    const db = getDb();
    let result;

    if (/^\d+$/.test(query)) {
      result = db
        .prepare(`DELETE FROM anime_watchlist WHERE user_id = ? AND room_id = ? AND mal_id = ?`)
        .run(ctx.sender, ctx.roomId, parseInt(query, 10));
    } else {
      result = db
        .prepare(`DELETE FROM anime_watchlist WHERE user_id = ? AND room_id = ? AND title LIKE ?`)
        .run(ctx.sender, ctx.roomId, `%${query}%`);
    }

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Removed ${result.changes} anime from your watchlist.`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `No matching anime found on your watchlist.`);
    }
  }

  private async handleSeason(ctx: MessageContext): Promise<void> {
    try {
      await jikanDelay();
      const res = await fetch(`https://api.jikan.moe/v4/seasons/now?limit=10&order_by=score&sort=desc`);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch current season data.");
        return;
      }

      const json = (await res.json()) as any;
      const animeList = (json.data ?? []) as JikanAnime[];

      if (animeList.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, "No current season anime found.");
        return;
      }

      for (const anime of animeList) {
        this.cacheAnime(anime);
      }

      const lines = ["Current Season — Top 10 by Score:"];
      for (let i = 0; i < animeList.length; i++) {
        const anime = animeList[i];
        const score = anime.score != null ? anime.score.toFixed(2) : "N/A";
        const episodes = anime.episodes != null ? String(anime.episodes) : "?";
        lines.push(`${i + 1}. ${anime.title} — Score: ${score} | Episodes: ${episodes}`);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`Season fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch season data. Try again later.");
    }
  }

  private async handleUpcoming(ctx: MessageContext): Promise<void> {
    try {
      await jikanDelay();
      const res = await fetch(`https://api.jikan.moe/v4/seasons/upcoming?limit=10&order_by=members&sort=desc`);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch upcoming season data.");
        return;
      }

      const json = (await res.json()) as any;
      const animeList = (json.data ?? []) as JikanAnime[];

      if (animeList.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, "No upcoming anime found.");
        return;
      }

      for (const anime of animeList) {
        this.cacheAnime(anime);
      }

      const lines = ["Upcoming Season — Top 10 by Popularity:"];
      for (let i = 0; i < animeList.length; i++) {
        const anime = animeList[i];
        const score = anime.score != null ? anime.score.toFixed(2) : "N/A";
        const episodes = anime.episodes != null ? String(anime.episodes) : "?";
        lines.push(`${i + 1}. ${anime.title} — Score: ${score} | Episodes: ${episodes}`);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`Upcoming fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch upcoming data. Try again later.");
    }
  }

  // ── Scheduled daily releases ───────────────────────────────────────

  /**
   * Called by DailyScheduler at 19:30 UTC to post today's airing watchlisted anime.
   */
  async postDailyReleases(botRooms: string[]): Promise<void> {
    const db = getDb();
    const dayNames = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
    const todayName = dayNames[new Date().getUTCDay()];

    // Get all unique mal_ids from watchlist that haven't been notified
    const watchedIds = db
      .prepare(`SELECT DISTINCT mal_id FROM anime_watchlist WHERE notified = 0`)
      .all() as { mal_id: number }[];

    if (watchedIds.length === 0) return;

    const airingToday: JikanAnime[] = [];

    for (const { mal_id } of watchedIds) {
      const anime = await this.fetchAnimeById(mal_id);
      if (!anime) continue;

      if (anime.status !== "Currently Airing") continue;

      const broadcastDay = anime.broadcast?.day ?? null;
      if (broadcastDay && broadcastDay.replace(/s$/, "") === todayName.replace(/s$/, "")) {
        airingToday.push(anime);
      }
    }

    if (airingToday.length === 0) return;

    // Build summary message
    const lines = ["Today's Airing Anime (from watchlists):"];
    for (const anime of airingToday) {
      const time = anime.broadcast?.time ? ` at ${anime.broadcast.time} JST` : "";
      lines.push(`- ${anime.title}${time}`);
    }
    const summary = lines.join("\n");

    // Post to each bot room
    for (const roomId of botRooms) {
      try {
        await this.sendMessage(roomId, summary);
      } catch (err) {
        logger.error(`Failed to post anime releases to ${roomId}: ${err}`);
      }
    }

    // DM users who have these shows on their watchlist
    const dmSent = new Set<string>();

    for (const anime of airingToday) {
      const watchers = db
        .prepare(`SELECT DISTINCT user_id FROM anime_watchlist WHERE mal_id = ? AND notified = 0`)
        .all(anime.mal_id) as { user_id: string }[];

      for (const { user_id } of watchers) {
        const key = `${user_id}:${anime.mal_id}`;
        if (dmSent.has(key)) continue;
        dmSent.add(key);

        try {
          const time = anime.broadcast?.time ? ` at ${anime.broadcast.time} JST` : "";
          await this.sendDm(user_id, `"${anime.title}" airs today${time}!`);
        } catch (err) {
          logger.error(`Failed to DM anime notification to ${user_id}: ${err}`);
        }
      }
    }
  }
}
