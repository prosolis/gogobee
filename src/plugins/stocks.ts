import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

const CACHE_TTL_QUOTE = 15 * 60;       // 15 minutes
const CACHE_TTL_PROFILE = 24 * 60 * 60; // 24 hours
const CACHE_TTL_METRICS = 60 * 60;      // 1 hour
const COOLDOWN_MS = 60_000;             // 60 seconds per user

export class StocksPlugin extends Plugin {
  private cooldowns: Map<string, number> = new Map();

  constructor(client: IMatrixClient) {
    super(client);
    this.initDb();
  }

  get name() {
    return "stocks";
  }

  get commands(): CommandDef[] {
    return [
      { name: "stock", description: "Get stock price info", usage: "!stock <TICKER> [TICKER2] [TICKER3]" },
      { name: "stockwatch", description: "Manage your stock watchlist", usage: "!stockwatch <TICKER|list|remove TICKER>" },
    ];
  }

  private initDb(): void {
    const db = getDb();
    db.exec(`
      CREATE TABLE IF NOT EXISTS stocks_cache (
        ticker TEXT PRIMARY KEY,
        data TEXT NOT NULL,
        cached_at INTEGER DEFAULT (unixepoch())
      )
    `);
    db.exec(`
      CREATE TABLE IF NOT EXISTS stock_watchlist (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id TEXT,
        room_id TEXT,
        ticker TEXT,
        created_at INTEGER,
        UNIQUE(user_id, room_id, ticker)
      )
    `);
  }

  private get apiKey(): string | undefined {
    return process.env.FINNHUB_API_KEY;
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "stockwatch")) {
      await this.handleStockwatch(ctx);
    } else if (this.isCommand(ctx.body, "stock")) {
      await this.handleStock(ctx);
    }
  }

  private checkCooldown(userId: string): number | null {
    const last = this.cooldowns.get(userId);
    if (last) {
      const elapsed = Date.now() - last;
      if (elapsed < COOLDOWN_MS) {
        return Math.ceil((COOLDOWN_MS - elapsed) / 1000);
      }
    }
    return null;
  }

  private setCooldown(userId: string): void {
    this.cooldowns.set(userId, Date.now());
  }

  private async handleStock(ctx: MessageContext): Promise<void> {
    if (!this.apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Finnhub API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "stock");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !stock <TICKER> [TICKER2] [TICKER3]");
      return;
    }

    const remaining = this.checkCooldown(ctx.sender);
    if (remaining !== null) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Please wait ${remaining}s before requesting stock data again.`);
      return;
    }

    const tickers = args.split(/\s+/).map((t) => t.toUpperCase());
    const blocks: string[] = [];

    for (const ticker of tickers) {
      try {
        const block = await this.getStockBlock(ticker);
        blocks.push(block);
      } catch (err) {
        logger.error(`Stock lookup failed for ${ticker}: ${err}`);
        blocks.push(`${ticker} — Failed to retrieve data.`);
      }
    }

    this.setCooldown(ctx.sender);
    await this.sendMessage(ctx.roomId, blocks.join("\n\n"));
  }

  private async getStockBlock(ticker: string): Promise<string> {
    const quote = await this.fetchQuote(ticker);
    if (!quote || quote.c === 0) {
      return `${ticker} — No data found. Verify the ticker symbol.`;
    }

    const profile = await this.fetchProfile(ticker);
    const metrics = await this.fetchMetrics(ticker);

    const change = quote.d ?? 0;
    const pctChange = quote.dp ?? 0;
    const arrow = change >= 0 ? "\u25B2" : "\u25BC";
    const sign = change >= 0 ? "+" : "";

    const header = profile && profile.name
      ? `${ticker} \u2014 ${profile.name}${profile.exchange ? ` (${profile.exchange})` : ""}`
      : ticker;

    const price = `$${quote.c.toFixed(2)}  ${arrow} ${sign}${change.toFixed(2)} (${sign}${pctChange.toFixed(2)}%)`;

    const high52 = metrics?.metric?.["52WeekHigh"];
    const low52 = metrics?.metric?.["52WeekLow"];
    const weekRange = high52 != null && low52 != null
      ? `$${low52.toFixed(2)} \u2013 $${high52.toFixed(2)}`
      : "N/A";

    const ts = quote.t ? new Date(quote.t * 1000) : null;
    const timeStr = ts
      ? `${ts.getUTCHours().toString().padStart(2, "0")}:${ts.getUTCMinutes().toString().padStart(2, "0")} UTC`
      : "N/A";

    return [
      header,
      `$${quote.c.toFixed(2)}  ${arrow} ${sign}${change.toFixed(2)} (${sign}${pctChange.toFixed(2)}%)`,
      `Volume: N/A  |  52W: ${weekRange}`,
      `Last updated: ${timeStr}`,
    ].join("\n");
  }

  private getCached(key: string, maxAge: number): any | null {
    const db = getDb();
    const row = db
      .prepare(`SELECT data, cached_at FROM stocks_cache WHERE ticker = ?`)
      .get(key) as { data: string; cached_at: number } | undefined;

    if (!row) return null;

    const age = Math.floor(Date.now() / 1000) - row.cached_at;
    if (age > maxAge) return null;

    try {
      return JSON.parse(row.data);
    } catch {
      return null;
    }
  }

  private setCache(key: string, data: any): void {
    const db = getDb();
    db.prepare(`INSERT OR REPLACE INTO stocks_cache (ticker, data, cached_at) VALUES (?, ?, unixepoch())`)
      .run(key, JSON.stringify(data));
  }

  private async fetchQuote(ticker: string): Promise<any> {
    const cacheKey = `quote:${ticker}`;
    const cached = this.getCached(cacheKey, CACHE_TTL_QUOTE);
    if (cached) return cached;

    const url = `https://finnhub.io/api/v1/quote?symbol=${encodeURIComponent(ticker)}&token=${encodeURIComponent(this.apiKey!)}`;
    const res = await fetch(url);
    if (!res.ok) {
      logger.warn(`Finnhub quote API returned ${res.status} for ${ticker}`);
      return null;
    }

    const data = await res.json();
    this.setCache(cacheKey, data);
    return data;
  }

  private async fetchProfile(ticker: string): Promise<any> {
    const cacheKey = `profile:${ticker}`;
    const cached = this.getCached(cacheKey, CACHE_TTL_PROFILE);
    if (cached) return cached;

    const url = `https://finnhub.io/api/v1/stock/profile2?symbol=${encodeURIComponent(ticker)}&token=${encodeURIComponent(this.apiKey!)}`;
    const res = await fetch(url);
    if (!res.ok) {
      logger.warn(`Finnhub profile API returned ${res.status} for ${ticker}`);
      return null;
    }

    const data = await res.json();
    this.setCache(cacheKey, data);
    return data;
  }

  private async fetchMetrics(ticker: string): Promise<any> {
    const cacheKey = `metrics:${ticker}`;
    const cached = this.getCached(cacheKey, CACHE_TTL_METRICS);
    if (cached) return cached;

    const url = `https://finnhub.io/api/v1/stock/metric?symbol=${encodeURIComponent(ticker)}&metric=all&token=${encodeURIComponent(this.apiKey!)}`;
    const res = await fetch(url);
    if (!res.ok) {
      logger.warn(`Finnhub metrics API returned ${res.status} for ${ticker}`);
      return null;
    }

    const data = await res.json();
    this.setCache(cacheKey, data);
    return data;
  }

  private async handleStockwatch(ctx: MessageContext): Promise<void> {
    if (!this.apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Finnhub API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "stockwatch").trim();

    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !stockwatch <TICKER> | !stockwatch list | !stockwatch remove <TICKER>");
      return;
    }

    if (args.toLowerCase() === "list") {
      await this.handleWatchlistList(ctx);
      return;
    }

    if (args.toLowerCase().startsWith("remove")) {
      const ticker = args.slice(6).trim().toUpperCase();
      if (!ticker) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !stockwatch remove <TICKER>");
        return;
      }
      await this.handleWatchlistRemove(ctx, ticker);
      return;
    }

    const ticker = args.toUpperCase();
    await this.handleWatchlistAdd(ctx, ticker);
  }

  private async handleWatchlistAdd(ctx: MessageContext, ticker: string): Promise<void> {
    const db = getDb();
    db.prepare(`INSERT OR IGNORE INTO stock_watchlist (user_id, room_id, ticker, created_at) VALUES (?, ?, ?, unixepoch())`)
      .run(ctx.sender, ctx.roomId, ticker);
    await this.sendReply(ctx.roomId, ctx.eventId, `Added ${ticker} to your watchlist.`);
  }

  private async handleWatchlistRemove(ctx: MessageContext, ticker: string): Promise<void> {
    const db = getDb();
    const result = db
      .prepare(`DELETE FROM stock_watchlist WHERE user_id = ? AND room_id = ? AND ticker = ?`)
      .run(ctx.sender, ctx.roomId, ticker);

    if (result.changes > 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Removed ${ticker} from your watchlist.`);
    } else {
      await this.sendReply(ctx.roomId, ctx.eventId, `${ticker} is not on your watchlist.`);
    }
  }

  private async handleWatchlistList(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT ticker FROM stock_watchlist WHERE user_id = ? AND room_id = ? ORDER BY ticker ASC`)
      .all(ctx.sender, ctx.roomId) as { ticker: string }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Your watchlist is empty. Use !stockwatch <TICKER> to add one.");
      return;
    }

    const lines = rows.map((row) => {
      const cached = this.getCached(`quote:${row.ticker}`, CACHE_TTL_QUOTE);
      if (cached && cached.c) {
        const change = cached.d ?? 0;
        const sign = change >= 0 ? "+" : "";
        const arrow = change >= 0 ? "\u25B2" : "\u25BC";
        return `${row.ticker}  $${cached.c.toFixed(2)}  ${arrow} ${sign}${change.toFixed(2)}`;
      }
      return `${row.ticker}  (no data)`;
    });

    await this.sendMessage(ctx.roomId, `Your watchlist:\n${lines.join("\n")}`);
  }
}
