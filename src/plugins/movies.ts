import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

const CACHE_TTL = 24 * 60 * 60; // 24 hours

export class MoviesPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "movies";
  }

  get commands(): CommandDef[] {
    return [
      { name: "movie", description: "Search movies or manage watchlist", usage: "!movie <title> | !movie watch <title> | !movie watching | !movie unwatch <title>" },
      { name: "tv", description: "Search TV shows or add to watchlist", usage: "!tv <title> | !tv watch <title>" },
      { name: "upcoming", description: "Upcoming theatrical releases", usage: "!upcoming movies [week|month]" },
    ];
  }

  private get apiKey(): string | undefined {
    return process.env.TMDB_API_KEY;
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "upcoming movies")) {
      await this.handleUpcoming(ctx);
    } else if (this.isCommand(ctx.body, "movie")) {
      await this.handleMovie(ctx);
    } else if (this.isCommand(ctx.body, "tv")) {
      await this.handleTv(ctx);
    }
  }

  // --- Movie command ---

  private async handleMovie(ctx: MessageContext): Promise<void> {
    if (!this.apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "TMDB API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "movie");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !movie <title> | !movie watch <title> | !movie watching | !movie unwatch <title>");
      return;
    }

    if (args === "watching") {
      await this.handleWatching(ctx);
      return;
    }

    if (args.startsWith("unwatch ")) {
      const query = args.slice(8).trim();
      if (!query) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !movie unwatch <title>");
        return;
      }
      await this.handleUnwatch(ctx, query);
      return;
    }

    if (args.startsWith("watch ")) {
      const title = args.slice(6).trim();
      if (!title) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !movie watch <title>");
        return;
      }
      await this.handleWatch(ctx, title, "movie");
      return;
    }

    await this.handleMovieSearch(ctx, args);
  }

  // --- TV command ---

  private async handleTv(ctx: MessageContext): Promise<void> {
    if (!this.apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "TMDB API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "tv");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !tv <title> | !tv watch <title>");
      return;
    }

    if (args.startsWith("watch ")) {
      const title = args.slice(6).trim();
      if (!title) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !tv watch <title>");
        return;
      }
      await this.handleWatch(ctx, title, "tv");
      return;
    }

    await this.handleTvSearch(ctx, args);
  }

  // --- Movie search ---

  private async handleMovieSearch(ctx: MessageContext, title: string): Promise<void> {
    try {
      const url = `https://api.themoviedb.org/3/search/movie?api_key=${this.apiKey}&query=${encodeURIComponent(title)}`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to search TMDB.");
        return;
      }

      const data = await res.json() as any;
      const results = data.results ?? [];
      if (results.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No movies found for "${title}".`);
        return;
      }

      const movie = results[0];
      const details = await this.fetchDetails(movie.id, "movie");
      if (!details) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch movie details.");
        return;
      }

      const year = details.release_date ? details.release_date.slice(0, 4) : "N/A";
      const rating = details.vote_average != null ? details.vote_average.toFixed(1) : "N/A";
      const runtime = details.runtime ?? "N/A";
      const genres = (details.genres ?? []).map((g: any) => g.name).join(", ") || "N/A";
      const overview = details.overview
        ? details.overview.length > 300
          ? details.overview.slice(0, 300) + "..."
          : details.overview
        : "No overview available.";
      const releaseDate = details.release_date ?? "N/A";

      const output = [
        `${details.title} (${year}) — ${rating}/10 | ${runtime} min | ${genres}`,
        overview,
        `Release: ${releaseDate}`,
      ].join("\n");

      await this.sendMessage(ctx.roomId, output);
    } catch (err) {
      logger.error(`Movie search failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while searching.");
    }
  }

  // --- TV search ---

  private async handleTvSearch(ctx: MessageContext, title: string): Promise<void> {
    try {
      const url = `https://api.themoviedb.org/3/search/tv?api_key=${this.apiKey}&query=${encodeURIComponent(title)}`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to search TMDB.");
        return;
      }

      const data = await res.json() as any;
      const results = data.results ?? [];
      if (results.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No TV shows found for "${title}".`);
        return;
      }

      const show = results[0];
      const details = await this.fetchDetails(show.id, "tv");
      if (!details) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch TV details.");
        return;
      }

      const year = details.first_air_date ? details.first_air_date.slice(0, 4) : "N/A";
      const rating = details.vote_average != null ? details.vote_average.toFixed(1) : "N/A";
      const seasons = details.number_of_seasons ?? "N/A";
      const status = details.status ?? "N/A";
      const genres = (details.genres ?? []).map((g: any) => g.name).join(", ") || "N/A";
      const overview = details.overview
        ? details.overview.length > 300
          ? details.overview.slice(0, 300) + "..."
          : details.overview
        : "No overview available.";

      const lines = [
        `${details.name} (${year}) — ${rating}/10 | Seasons: ${seasons} | ${status}`,
        overview,
      ];

      if (details.next_episode_to_air && details.next_episode_to_air.air_date) {
        lines.push(`Next episode: ${details.next_episode_to_air.air_date}`);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`TV search failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while searching.");
    }
  }

  // --- Watch (add to watchlist) ---

  private async handleWatch(ctx: MessageContext, title: string, mediaType: "movie" | "tv"): Promise<void> {
    try {
      const searchType = mediaType === "movie" ? "movie" : "tv";
      const url = `https://api.themoviedb.org/3/search/${searchType}?api_key=${this.apiKey}&query=${encodeURIComponent(title)}`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to search TMDB.");
        return;
      }

      const data = await res.json() as any;
      const results = data.results ?? [];
      if (results.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No ${mediaType === "movie" ? "movies" : "TV shows"} found for "${title}".`);
        return;
      }

      const item = results[0];
      const tmdbId = item.id;
      const itemTitle = mediaType === "movie" ? item.title : item.name;
      const releaseDate = mediaType === "movie" ? (item.release_date ?? null) : (item.first_air_date ?? null);

      const db = getDb();
      db.prepare(`
        INSERT INTO movie_watchlist (user_id, room_id, tmdb_id, title, media_type, release_date, created_at)
        VALUES (?, ?, ?, ?, ?, ?, unixepoch())
      `).run(ctx.sender, ctx.roomId, tmdbId, itemTitle, mediaType, releaseDate);

      await this.sendReply(ctx.roomId, ctx.eventId, `Added "${itemTitle}" (${mediaType}) to your watchlist.`);
    } catch (err: any) {
      if (err?.code === "SQLITE_CONSTRAINT") {
        await this.sendReply(ctx.roomId, ctx.eventId, "That title is already on your watchlist.");
        return;
      }
      logger.error(`Watch add failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while adding to watchlist.");
    }
  }

  // --- Watching (list watchlist) ---

  private async handleWatching(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT title, media_type, release_date FROM movie_watchlist WHERE user_id = ? AND room_id = ? ORDER BY media_type ASC, title ASC`)
      .all(ctx.sender, ctx.roomId) as { title: string; media_type: string; release_date: string | null }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Your watchlist is empty.");
      return;
    }

    const movies = rows.filter((r) => r.media_type === "movie");
    const tvShows = rows.filter((r) => r.media_type === "tv");

    const lines: string[] = [];

    if (movies.length > 0) {
      lines.push("Movies:");
      for (const m of movies) {
        lines.push(`  - ${m.title}${m.release_date ? ` (${m.release_date})` : ""}`);
      }
    }

    if (tvShows.length > 0) {
      lines.push("TV Shows:");
      for (const t of tvShows) {
        lines.push(`  - ${t.title}${t.release_date ? ` (${t.release_date})` : ""}`);
      }
    }

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }

  // --- Unwatch ---

  private async handleUnwatch(ctx: MessageContext, query: string): Promise<void> {
    const db = getDb();
    const row = db
      .prepare(`SELECT id, title FROM movie_watchlist WHERE user_id = ? AND room_id = ? AND title LIKE ? LIMIT 1`)
      .get(ctx.sender, ctx.roomId, `%${query}%`) as { id: number; title: string } | undefined;

    if (!row) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No watchlist entry matching "${query}" found.`);
      return;
    }

    db.prepare(`DELETE FROM movie_watchlist WHERE id = ?`).run(row.id);
    await this.sendReply(ctx.roomId, ctx.eventId, `Removed "${row.title}" from your watchlist.`);
  }

  // --- Upcoming movies ---

  private async handleUpcoming(ctx: MessageContext): Promise<void> {
    if (!this.apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "TMDB API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "upcoming movies").trim().toLowerCase();
    const days = args === "week" ? 7 : 30;

    try {
      const url = `https://api.themoviedb.org/3/movie/upcoming?api_key=${this.apiKey}&region=US`;
      const res = await fetch(url);
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch upcoming movies.");
        return;
      }

      const data = await res.json() as any;
      const movies = data.results ?? [];

      const now = new Date();
      const cutoff = new Date(now.getTime() + days * 24 * 60 * 60 * 1000);
      const todayStr = now.toISOString().slice(0, 10);
      const cutoffStr = cutoff.toISOString().slice(0, 10);

      const upcoming = movies.filter((m: any) => {
        if (!m.release_date) return false;
        return m.release_date >= todayStr && m.release_date <= cutoffStr;
      });

      if (upcoming.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No upcoming theatrical releases in the next ${days} days.`);
        return;
      }

      const lines = [`Upcoming Movies (next ${days} days):`];
      for (const m of upcoming) {
        lines.push(`- ${m.title} (${m.release_date})`);
      }

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`Upcoming movies fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while fetching upcoming movies.");
    }
  }

  // --- TMDB details with cache ---

  private async fetchDetails(tmdbId: number, mediaType: string): Promise<any | null> {
    const cached = this.getCached(tmdbId, mediaType);
    if (cached) return cached;

    try {
      const url = `https://api.themoviedb.org/3/${mediaType}/${tmdbId}?api_key=${this.apiKey}`;
      const res = await fetch(url);
      if (!res.ok) {
        logger.warn(`TMDB details API returned ${res.status} for ${mediaType}/${tmdbId}`);
        return null;
      }

      const data = await res.json();
      this.setCache(tmdbId, mediaType, data);
      return data;
    } catch (err) {
      logger.error(`TMDB details fetch failed for ${mediaType}/${tmdbId}: ${err}`);
      return null;
    }
  }

  private getCached(tmdbId: number, mediaType: string): any | null {
    const db = getDb();
    const row = db
      .prepare(`SELECT data, cached_at FROM movie_cache WHERE tmdb_id = ? AND media_type = ?`)
      .get(tmdbId, mediaType) as { data: string; cached_at: number } | undefined;

    if (!row) return null;

    const age = Math.floor(Date.now() / 1000) - row.cached_at;
    if (age > CACHE_TTL) return null;

    try {
      return JSON.parse(row.data);
    } catch {
      return null;
    }
  }

  private setCache(tmdbId: number, mediaType: string, data: any): void {
    const db = getDb();
    db.prepare(`INSERT OR REPLACE INTO movie_cache (tmdb_id, media_type, data, cached_at) VALUES (?, ?, ?, unixepoch())`)
      .run(tmdbId, mediaType, JSON.stringify(data));
  }

  // --- Daily releases (called by scheduler at 20:00 UTC) ---

  async postDailyReleases(botRooms: string[]): Promise<void> {
    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    const releases = db
      .prepare(`SELECT DISTINCT tmdb_id, title, media_type FROM movie_watchlist WHERE release_date = ? AND notified = 0`)
      .all(today) as { tmdb_id: number; title: string; media_type: string }[];

    if (releases.length === 0) return;

    const lines = ["Today's Watchlist Releases:"];
    for (const r of releases) {
      const typeLabel = r.media_type === "movie" ? "Movie" : "TV";
      lines.push(`- [${typeLabel}] ${r.title}`);
    }
    const message = lines.join("\n");

    // Post to each bot room
    for (const roomId of botRooms) {
      try {
        await this.sendMessage(roomId, message);
      } catch (err) {
        logger.error(`Failed to post daily movie releases to ${roomId}: ${err}`);
      }
    }

    // DM users who have these on their watchlist
    const userEntries = db
      .prepare(`SELECT id, user_id, title, media_type FROM movie_watchlist WHERE release_date = ? AND notified = 0`)
      .all(today) as { id: number; user_id: string; title: string; media_type: string }[];

    const userMap = new Map<string, string[]>();
    for (const entry of userEntries) {
      const typeLabel = entry.media_type === "movie" ? "Movie" : "TV";
      const label = `[${typeLabel}] ${entry.title}`;
      const existing = userMap.get(entry.user_id) ?? [];
      existing.push(label);
      userMap.set(entry.user_id, existing);
    }

    for (const [userId, titles] of userMap) {
      try {
        const dmMessage = `Your watchlist releases today:\n${titles.map((t) => `- ${t}`).join("\n")}`;
        await this.sendDm(userId, dmMessage);
      } catch (err) {
        logger.error(`Failed to DM movie releases to ${userId}: ${err}`);
      }
    }

    // Mark all as notified
    db.prepare(`UPDATE movie_watchlist SET notified = 1 WHERE release_date = ? AND notified = 0`).run(today);
  }
}
