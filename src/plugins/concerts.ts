import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { RateLimitsPlugin } from "./ratelimits";
import { getDb } from "../db";
import logger from "../utils/logger";

interface BandsintownEvent {
  venue: { name: string; city: string; country: string };
  datetime: string;
  offers: any[];
  lineup: string[];
}

export class ConcertsPlugin extends Plugin {
  private rateLimitPlugin: RateLimitsPlugin;

  constructor(client: IMatrixClient, rateLimitPlugin: RateLimitsPlugin) {
    super(client);
    this.rateLimitPlugin = rateLimitPlugin;
  }

  get name() {
    return "concerts";
  }

  get commands(): CommandDef[] {
    return [
      {
        name: "concerts",
        description: "Search upcoming concerts or manage your artist watchlist",
        usage: "!concerts <artist> | !concerts watch <artist> | !concerts watching | !concerts unwatch <artist>",
      },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (!this.isCommand(ctx.body, "concerts")) return;

    const args = this.getArgs(ctx.body, "concerts").trim();

    if (!args) {
      await this.sendReply(
        ctx.roomId,
        ctx.eventId,
        "Usage: !concerts <artist> | !concerts watch <artist> | !concerts watching | !concerts unwatch <artist>"
      );
      return;
    }

    if (args.toLowerCase().startsWith("watch ")) {
      const artist = args.slice(6).trim();
      if (!artist) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Specify an artist to watch.");
        return;
      }
      await this.handleWatch(ctx, artist);
      return;
    }

    if (args.toLowerCase() === "watching") {
      await this.handleWatching(ctx);
      return;
    }

    if (args.toLowerCase().startsWith("unwatch ")) {
      const artist = args.slice(8).trim();
      if (!artist) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Specify an artist to unwatch.");
        return;
      }
      await this.handleUnwatch(ctx, artist);
      return;
    }

    // Default: search by artist name
    await this.handleArtistSearch(ctx, args);
  }

  private getApiKey(): string | undefined {
    return process.env.BANDSINTOWN_API_KEY;
  }

  private async handleArtistSearch(ctx: MessageContext, artist: string): Promise<void> {
    const key = this.getApiKey();
    if (!key) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Bandsintown API key not configured.");
      return;
    }

    const allowed = this.rateLimitPlugin.checkLimit(
      ctx.sender,
      "concerts",
      parseInt(process.env.RATELIMIT_CONCERTS ?? "10")
    );
    if (!allowed) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You've reached your daily concerts lookup quota. Try again tomorrow.");
      return;
    }

    try {
      const events = await this.fetchArtistEvents(artist, key);

      if (!events || events.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No upcoming events found for "${artist}".`);
        return;
      }

      const lines = events.slice(0, 10).map((evt) => {
        const lineup = evt.lineup?.join(", ") ?? artist;
        const venue = evt.venue?.name ?? "Unknown Venue";
        const city = evt.venue?.city ?? "Unknown City";
        const date = this.formatDate(evt.datetime);
        return `${lineup} — ${venue}, ${city} — ${date}`;
      });

      const header = `Upcoming events for "${artist}" (${Math.min(events.length, 10)} of ${events.length}):`;
      await this.sendMessage(ctx.roomId, `${header}\n${lines.join("\n")}`);
    } catch (err) {
      logger.error(`Concerts artist search failed for "${artist}": ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch concert data. Try again later.");
    }
  }

  private async fetchArtistEvents(artist: string, apiKey: string): Promise<BandsintownEvent[]> {
    const cacheKey = artist.toLowerCase();
    const db = getDb();

    // Check cache (6 hours = 21600 seconds)
    const cached = db
      .prepare(`SELECT data FROM concerts_cache WHERE location_key = ? AND cached_at > unixepoch() - 21600`)
      .get(cacheKey) as { data: string } | undefined;

    if (cached) {
      return JSON.parse(cached.data);
    }

    const url = `https://rest.bandsintown.com/artists/${encodeURIComponent(artist)}/events?app_id=${encodeURIComponent(apiKey)}&date=upcoming`;
    const res = await fetch(url);

    if (!res.ok) {
      logger.warn(`Bandsintown API returned ${res.status} for artist "${artist}"`);
      return [];
    }

    const data = (await res.json()) as BandsintownEvent[];

    if (!Array.isArray(data)) {
      return [];
    }

    // Cache the result
    db.prepare(
      `INSERT INTO concerts_cache (location_key, data, cached_at) VALUES (?, ?, unixepoch())
       ON CONFLICT(location_key) DO UPDATE SET data = excluded.data, cached_at = unixepoch()`
    ).run(cacheKey, JSON.stringify(data));

    return data;
  }

  private async handleWatch(ctx: MessageContext, artist: string): Promise<void> {
    const db = getDb();

    try {
      db.prepare(
        `INSERT INTO concert_watchlist (user_id, room_id, artist, created_at) VALUES (?, ?, ?, unixepoch())`
      ).run(ctx.sender, ctx.roomId, artist.toLowerCase());

      await this.sendReply(ctx.roomId, ctx.eventId, `Now watching "${artist}" for upcoming concerts.`);
    } catch (err: any) {
      if (err?.code === "SQLITE_CONSTRAINT_UNIQUE" || err?.message?.includes("UNIQUE")) {
        await this.sendReply(ctx.roomId, ctx.eventId, `You're already watching "${artist}".`);
      } else {
        logger.error(`Failed to add concert watch for ${ctx.sender}: ${err}`);
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to add artist to watchlist.");
      }
    }
  }

  private async handleWatching(ctx: MessageContext): Promise<void> {
    const db = getDb();

    const rows = db
      .prepare(`SELECT artist FROM concert_watchlist WHERE user_id = ? AND room_id = ? ORDER BY artist`)
      .all(ctx.sender, ctx.roomId) as { artist: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You're not watching any artists. Use !concerts watch <artist> to add one.");
      return;
    }

    const list = rows.map((r) => `- ${r.artist}`).join("\n");
    await this.sendMessage(ctx.roomId, `Your watched artists:\n${list}`);
  }

  private async handleUnwatch(ctx: MessageContext, artist: string): Promise<void> {
    const db = getDb();

    const result = db
      .prepare(`DELETE FROM concert_watchlist WHERE user_id = ? AND room_id = ? AND artist = ?`)
      .run(ctx.sender, ctx.roomId, artist.toLowerCase());

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Stopped watching "${artist}".`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `You weren't watching "${artist}".`);
    }
  }

  /**
   * Called by the scheduler to post a weekly concert digest.
   * Only posts on Sundays (day 0).
   */
  async postWeeklyDigest(botRooms: string[]): Promise<void> {
    const today = new Date();
    if (today.getUTCDay() !== 0) return;

    const key = this.getApiKey();
    if (!key) {
      logger.warn("BANDSINTOWN_API_KEY not set, skipping concert digest");
      return;
    }

    const db = getDb();

    // Get all unique watched artists across all users
    const watchedArtists = db
      .prepare(`SELECT DISTINCT artist FROM concert_watchlist`)
      .all() as { artist: string }[];

    if (watchedArtists.length === 0) return;

    // Calculate the date range for the next 7 days
    const now = new Date();
    const weekFromNow = new Date(now);
    weekFromNow.setUTCDate(weekFromNow.getUTCDate() + 7);

    const startIso = now.toISOString().slice(0, 10);
    const endIso = weekFromNow.toISOString().slice(0, 10);

    // Fetch events for each watched artist
    const upcomingShows: { artist: string; events: BandsintownEvent[] }[] = [];

    for (const { artist } of watchedArtists) {
      try {
        const events = await this.fetchArtistEvents(artist, key);
        const thisWeek = events.filter((evt) => {
          const eventDate = evt.datetime?.slice(0, 10);
          return eventDate && eventDate >= startIso && eventDate <= endIso;
        });

        if (thisWeek.length > 0) {
          upcomingShows.push({ artist, events: thisWeek });
        }
      } catch (err) {
        logger.error(`Failed to fetch events for watched artist "${artist}": ${err}`);
      }
    }

    if (upcomingShows.length === 0) return;

    // Build summary message for bot rooms
    const lines: string[] = ["Concert Digest — This Week:", ""];
    for (const { artist, events } of upcomingShows) {
      for (const evt of events) {
        const venue = evt.venue?.name ?? "Unknown Venue";
        const city = evt.venue?.city ?? "Unknown City";
        const date = this.formatDate(evt.datetime);
        const lineup = evt.lineup?.join(", ") ?? artist;
        lines.push(`${lineup} — ${venue}, ${city} — ${date}`);
      }
    }

    const message = lines.join("\n");

    for (const roomId of botRooms) {
      try {
        await this.sendMessage(roomId, message);
      } catch (err) {
        logger.error(`Failed to post concert digest to ${roomId}: ${err}`);
      }
    }

    // DM users who are watching artists with shows this week
    const artistsWithShows = new Set(upcomingShows.map((s) => s.artist));

    const watchers = db
      .prepare(`SELECT DISTINCT user_id, artist FROM concert_watchlist WHERE artist IN (${[...artistsWithShows].map(() => "?").join(",")})`)
      .all(...artistsWithShows) as { user_id: string; artist: string }[];

    // Group by user
    const userArtists = new Map<string, string[]>();
    for (const { user_id, artist } of watchers) {
      if (!userArtists.has(user_id)) userArtists.set(user_id, []);
      userArtists.get(user_id)!.push(artist);
    }

    for (const [userId, artists] of userArtists) {
      const dmLines: string[] = ["Concerts this week for your watched artists:", ""];
      for (const artist of artists) {
        const showData = upcomingShows.find((s) => s.artist === artist);
        if (!showData) continue;
        for (const evt of showData.events) {
          const venue = evt.venue?.name ?? "Unknown Venue";
          const city = evt.venue?.city ?? "Unknown City";
          const date = this.formatDate(evt.datetime);
          const lineup = evt.lineup?.join(", ") ?? artist;
          dmLines.push(`${lineup} — ${venue}, ${city} — ${date}`);
        }
      }

      try {
        await this.sendDm(userId, dmLines.join("\n"));
      } catch (err) {
        logger.error(`Failed to DM concert digest to ${userId}: ${err}`);
      }
    }
  }

  private formatDate(datetime: string): string {
    try {
      const d = new Date(datetime);
      return d.toLocaleDateString("en-US", {
        weekday: "short",
        month: "short",
        day: "numeric",
        year: "numeric",
        timeZone: "UTC",
      });
    } catch {
      return datetime?.slice(0, 10) ?? "Unknown Date";
    }
  }
}
