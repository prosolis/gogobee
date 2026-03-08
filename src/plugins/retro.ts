import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

const RAWG_API_KEY = process.env.RAWG_API_KEY ?? "";
const ENABLED = RAWG_API_KEY !== "";
const CACHE_TTL = 7 * 24 * 60 * 60; // 7 days

export class RetroPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "retro";
  }

  get commands(): CommandDef[] {
    return [
      { name: "game", description: "Game lookup via RAWG", usage: "!game <game>" },
      { name: "retro", description: "Alias for !game", usage: "!retro <game>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "game")) {
      await this.handleGameLookup(ctx, "game");
    } else if (this.isCommand(ctx.body, "retro")) {
      await this.handleGameLookup(ctx, "retro");
    }
  }

  private async handleGameLookup(ctx: MessageContext, command: string): Promise<void> {
    if (!ENABLED) {
      await this.sendReply(ctx.roomId, ctx.eventId, "RAWG API is not configured. Set RAWG_API_KEY.");
      return;
    }

    const query = this.getArgs(ctx.body, command);
    if (!query) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Usage: !${command} <game>`);
      return;
    }

    const db = getDb();
    const searchKey = query.toLowerCase();

    // Check cache
    const cached = db.prepare(
      `SELECT data FROM retro_cache WHERE search_term = ? AND cached_at > unixepoch() - ?`
    ).get(searchKey, CACHE_TTL) as { data: string } | undefined;

    let games: any[];

    if (cached) {
      games = JSON.parse(cached.data);
    } else {
      try {
        // Search RAWG, sort by relevance
        const url = `https://api.rawg.io/api/games?key=${RAWG_API_KEY}&search=${encodeURIComponent(query)}&page_size=3&search_precise=true`;
        const res = await fetch(url, { signal: AbortSignal.timeout(10_000) });

        if (!res.ok) {
          await this.sendReply(ctx.roomId, ctx.eventId, "RAWG API request failed.");
          return;
        }

        const data = (await res.json()) as { results: any[] };
        games = data.results ?? [];

        // Fetch details for the first result to get description and developers
        if (games.length > 0) {
          try {
            const detailRes = await fetch(
              `https://api.rawg.io/api/games/${games[0].id}?key=${RAWG_API_KEY}`,
              { signal: AbortSignal.timeout(10_000) }
            );
            if (detailRes.ok) {
              const detail = (await detailRes.json()) as any;
              games[0]._detail = {
                description_raw: detail.description_raw,
                developers: detail.developers,
                publishers: detail.publishers,
              };
            }
          } catch { /* detail fetch is best-effort */ }
        }

        if (games.length > 0) {
          db.prepare(
            `INSERT INTO retro_cache (search_term, data) VALUES (?, ?)
             ON CONFLICT(search_term) DO UPDATE SET data = excluded.data, cached_at = unixepoch()`
          ).run(searchKey, JSON.stringify(games));
        }
      } catch (err) {
        logger.error(`Retro game lookup failed: ${err}`);
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to look up that game.");
        return;
      }
    }

    if (!games || games.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No results found.");
      return;
    }

    const reply = games.map((g, i) => this.formatGame(g, i === 0)).join("\n\n---\n\n");
    await this.sendMessage(ctx.roomId, reply);
  }

  private formatGame(game: any, detailed: boolean): string {
    const name = game.name ?? "Unknown";

    // Release year
    let year = "Unknown";
    if (game.released) {
      year = game.released.slice(0, 4);
    }

    // Platforms
    const platforms = (game.platforms ?? [])
      .map((p: any) => p.platform?.name)
      .filter(Boolean)
      .join(", ") || "Unknown";

    // Genres
    const genres = (game.genres ?? [])
      .map((g: any) => g.name)
      .join(", ");

    // Rating
    let rating = "";
    if (game.metacritic) {
      rating = `Metacritic: ${game.metacritic}`;
    } else if (game.rating) {
      rating = `Rating: ${game.rating}/5`;
    }

    // Developers & publishers (from detail fetch)
    const developers = (game._detail?.developers ?? [])
      .map((d: any) => d.name)
      .join(", ");
    const publishers = (game._detail?.publishers ?? [])
      .map((p: any) => p.name)
      .join(", ");

    let lines = [`${name} (${year})`];
    lines.push(`Platforms: ${platforms}`);
    if (developers) lines.push(`Developer: ${developers}`);
    if (publishers) lines.push(`Publisher: ${publishers}`);
    if (genres) lines.push(`Genre: ${genres}`);
    if (rating) lines.push(rating);

    if (detailed && game._detail?.description_raw) {
      let desc = game._detail.description_raw;
      if (desc.length > 300) desc = desc.slice(0, 300) + "...";
      lines.push("");
      lines.push(desc);
    }

    return lines.join("\n");
  }
}
