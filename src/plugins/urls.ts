import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { parse as parseHtml } from "node-html-parser";
import logger from "../utils/logger";

const URL_REGEX = /https?:\/\/[^\s<>]+/g;
const MEDIA_EXT = /\.(jpg|jpeg|png|gif|webp|mp4|webm|mp3|wav|pdf|zip|tar|gz)$/i;
const CACHE_TTL = 86400; // 24 hours in seconds
const FETCH_TIMEOUT = 3000; // 3 seconds
const ENABLED = process.env.FEATURE_URL_PREVIEW === "true";

export class UrlsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
    this.ensureTable();
  }

  get name() {
    return "urls";
  }

  get commands(): CommandDef[] {
    return [];
  }

  private ensureTable(): void {
    try {
      const db = getDb();
      db.exec(`
        CREATE TABLE IF NOT EXISTS url_cache (
          url TEXT PRIMARY KEY,
          title TEXT,
          description TEXT,
          cached_at INTEGER
        )
      `);
    } catch (err) {
      logger.debug(`Failed to create url_cache table: ${err}`);
    }
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (!ENABLED) return;

    try {
      const matches = ctx.body.match(URL_REGEX);
      if (!matches || matches.length === 0) return;

      // Only process the first URL to avoid spam
      const url = matches[0];

      // Skip direct media files
      if (MEDIA_EXT.test(url)) return;

      const db = getDb();

      // Check cache
      const cached = db
        .prepare(`SELECT title, description, cached_at FROM url_cache WHERE url = ? AND cached_at > (unixepoch() - ?)`)
        .get(url, CACHE_TTL) as { title: string | null; description: string | null; cached_at: number } | undefined;

      let title: string | null = null;
      let description: string | null = null;

      if (cached) {
        // If title is null, a previous fetch failed — skip
        if (cached.title === null) return;
        title = cached.title;
        description = cached.description;
      } else {
        // Fetch the URL
        const result = await this.fetchUrl(url);
        if (!result) {
          // Cache the failure
          db.prepare(`INSERT OR REPLACE INTO url_cache (url, title, description, cached_at) VALUES (?, NULL, NULL, unixepoch())`).run(url);
          return;
        }

        title = result.title;
        description = result.description;

        // Cache result
        db.prepare(`INSERT OR REPLACE INTO url_cache (url, title, description, cached_at) VALUES (?, ?, ?, unixepoch())`).run(
          url,
          title,
          description
        );

        if (!title) return;
      }

      // Skip if the message body already contains the title text
      if (title && ctx.body.toLowerCase().includes(title.toLowerCase())) return;

      // Build reply
      let reply = `\u{1F517} ${title}`;
      if (description) {
        const truncated = description.length > 200 ? description.slice(0, 200) + "..." : description;
        reply += `\n${truncated}`;
      }

      await this.sendReply(ctx.roomId, ctx.eventId, reply);
    } catch (err) {
      logger.debug(`URL preview error: ${err}`);
    }
  }

  private async fetchUrl(url: string): Promise<{ title: string | null; description: string | null } | null> {
    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT);

      try {
        const res = await fetch(url, {
          signal: controller.signal,
          headers: {
            "User-Agent": "Freebee Bot/1.0",
          },
          redirect: "follow",
        });

        if (!res.ok) return null;

        const contentType = res.headers.get("content-type") ?? "";
        if (!contentType.includes("text/html")) return null;

        const html = await res.text();
        const root = parseHtml(html);

        // Extract title: og:title -> <title>
        const ogTitle = root.querySelector('meta[property="og:title"]')?.getAttribute("content") ?? null;
        const titleTag = root.querySelector("title")?.text ?? null;
        const title = ogTitle || titleTag || null;

        // Extract description: og:description -> meta[name="description"]
        const ogDesc = root.querySelector('meta[property="og:description"]')?.getAttribute("content") ?? null;
        const metaDesc = root.querySelector('meta[name="description"]')?.getAttribute("content") ?? null;
        const description = ogDesc || metaDesc || null;

        return { title, description };
      } finally {
        clearTimeout(timeout);
      }
    } catch (err) {
      logger.debug(`Failed to fetch URL ${url}: ${err}`);
      return null;
    }
  }
}
