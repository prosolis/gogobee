import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";
import { RateLimitsPlugin } from "./ratelimits";

const LANG_CODES = ["pt", "es", "fr", "de", "ja", "ko", "zh", "ar", "ru", "it"];

export class LookupPlugin extends Plugin {
  private rateLimitPlugin: RateLimitsPlugin;

  constructor(client: IMatrixClient, rateLimitPlugin: RateLimitsPlugin) {
    super(client);
    this.rateLimitPlugin = rateLimitPlugin;
  }

  get name() {
    return "lookup";
  }

  get commands(): CommandDef[] {
    return [
      { name: "wiki", description: "Look up a Wikipedia article", usage: "!wiki <topic>" },
      { name: "define", description: "Look up a word definition", usage: "!define <word>" },
      { name: "translate", description: "Translate text using LibreTranslate", usage: "!translate [lang] <text>" },
      { name: "urban", description: "Urban Dictionary lookup", usage: "!urban <term>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "wiki")) {
      await this.handleWiki(ctx);
    } else if (this.isCommand(ctx.body, "define")) {
      await this.handleDefine(ctx);
    } else if (this.isCommand(ctx.body, "translate")) {
      await this.handleTranslate(ctx);
    } else if (this.isCommand(ctx.body, "urban")) {
      await this.handleUrban(ctx);
    }
  }

  private async handleWiki(ctx: MessageContext): Promise<void> {
    const topic = this.getArgs(ctx.body, "wiki");
    if (!topic) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !wiki <topic>");
      return;
    }

    try {
      const url = `https://en.wikipedia.org/api/rest_v1/page/summary/${encodeURIComponent(topic)}`;
      const res = await fetch(url);

      if (res.status === 404) {
        await this.sendReply(ctx.roomId, ctx.eventId, "No Wikipedia article found for that topic.");
        return;
      }

      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch Wikipedia article.");
        return;
      }

      const data = await res.json() as any;
      const title = data.title ?? topic;
      let extract = data.extract ?? "";
      if (extract.length > 500) {
        extract = extract.slice(0, 500) + "...";
      }
      const pageUrl = data.content_urls?.desktop?.page ?? "";

      const reply = `${title}\n\n${extract}\n\nRead more: ${pageUrl}`;
      await this.sendMessage(ctx.roomId, reply);
    } catch (err) {
      logger.error(`Wiki lookup failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while looking up that topic.");
    }
  }

  private async handleDefine(ctx: MessageContext): Promise<void> {
    const word = this.getArgs(ctx.body, "define");
    if (!word) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !define <word>");
      return;
    }

    try {
      const url = `https://api.dictionaryapi.dev/api/v2/entries/en/${encodeURIComponent(word)}`;
      const res = await fetch(url);

      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "No definition found.");
        return;
      }

      const data = await res.json() as any[];
      if (!data || data.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, "No definition found.");
        return;
      }

      const entry = data[0];
      const entryWord = entry.word ?? word;
      const phonetic = entry.phonetic ?? "";
      const meaning = entry.meanings?.[0];
      const partOfSpeech = meaning?.partOfSpeech ?? "";
      const definition = meaning?.definitions?.[0]?.definition ?? "";
      const example = meaning?.definitions?.[0]?.example;

      let reply = `${entryWord}${phonetic ? ` (${phonetic})` : ""}\n[${partOfSpeech}] ${definition}`;
      if (example) {
        reply += `\nExample: "${example}"`;
      }

      await this.sendMessage(ctx.roomId, reply);
    } catch (err) {
      logger.error(`Define lookup failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while looking up that word.");
    }
  }

  private async handleUrban(ctx: MessageContext): Promise<void> {
    const term = this.getArgs(ctx.body, "urban");
    if (!term) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !urban <term>");
      return;
    }

    const db = getDb();
    const termLower = term.toLowerCase();
    const CACHE_TTL = 24 * 60 * 60; // 24 hours

    // Check cache
    const cached = db.prepare(
      `SELECT data FROM urban_cache WHERE term = ? AND cached_at > unixepoch() - ?`
    ).get(termLower, CACHE_TTL) as { data: string } | undefined;

    let entry: any;

    if (cached) {
      entry = JSON.parse(cached.data);
    } else {
      try {
        const url = `https://api.urbandictionary.com/v0/define?term=${encodeURIComponent(term)}`;
        const res = await fetch(url);
        if (!res.ok) {
          await this.sendReply(ctx.roomId, ctx.eventId, "Failed to look up that term.");
          return;
        }

        const data = (await res.json()) as { list: any[] };
        if (!data.list || data.list.length === 0) {
          await this.sendReply(ctx.roomId, ctx.eventId, "No Urban Dictionary entry found.");
          return;
        }

        entry = data.list[0];

        db.prepare(
          `INSERT INTO urban_cache (term, data) VALUES (?, ?)
           ON CONFLICT(term) DO UPDATE SET data = excluded.data, cached_at = unixepoch()`
        ).run(termLower, JSON.stringify(entry));
      } catch (err) {
        logger.error(`Urban Dictionary lookup failed: ${err}`);
        await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while looking up that term.");
        return;
      }
    }

    const word = entry.word ?? term;
    let definition = (entry.definition ?? "").replace(/\[|\]/g, "");
    if (definition.length > 400) definition = definition.slice(0, 400) + "...";
    let example = (entry.example ?? "").replace(/\[|\]/g, "");
    if (example.length > 200) example = example.slice(0, 200) + "...";
    const thumbsUp = entry.thumbs_up ?? 0;
    const thumbsDown = entry.thumbs_down ?? 0;

    let reply = `${word}\n\n${definition}`;
    if (example) reply += `\n\nExample: "${example}"`;
    reply += `\n\n\uD83D\uDC4D ${thumbsUp} \uD83D\uDC4E ${thumbsDown}`;

    await this.sendMessage(ctx.roomId, reply);
  }

  private async handleTranslate(ctx: MessageContext): Promise<void> {
    const LIBRETRANSLATE_URL = process.env.LIBRETRANSLATE_URL;
    if (!LIBRETRANSLATE_URL) {
      await this.sendReply(ctx.roomId, ctx.eventId, "LibreTranslate is not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "translate");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !translate [lang] <text>");
      return;
    }

    const allowed = this.rateLimitPlugin.checkLimit(
      ctx.sender,
      "translate",
      parseInt(process.env.RATELIMIT_TRANSLATE ?? "20"),
    );
    if (!allowed) {
      await this.sendReply(ctx.roomId, ctx.eventId, "You have reached your daily translate quota. Try again tomorrow.");
      return;
    }

    let targetLang = "en";
    let text = args;

    const words = args.split(/\s+/);
    if (words.length > 1 && words[0].length === 2 && LANG_CODES.includes(words[0].toLowerCase())) {
      targetLang = words[0].toLowerCase();
      text = words.slice(1).join(" ");
    }

    try {
      const res = await fetch(`${LIBRETRANSLATE_URL}/translate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          q: text,
          source: "auto",
          target: targetLang,
        }),
      });

      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Translation request failed.");
        return;
      }

      const data = await res.json() as any;
      const translatedText = data.translatedText ?? "";
      const detectedLang = data.detectedLanguage?.language ?? "auto";

      const reply = `[${detectedLang} \u2192 ${targetLang}] ${translatedText}`;
      await this.sendMessage(ctx.roomId, reply);
    } catch (err) {
      logger.error(`Translate failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "An error occurred while translating.");
    }
  }
}
