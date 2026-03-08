import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { DateTime, IANAZone } from "luxon";
import logger from "../utils/logger";

// City map (shared with user.ts but duplicated here to avoid coupling)
const CITY_TIMEZONE_MAP: Record<string, string> = {
  "new york": "America/New_York", "nyc": "America/New_York",
  "los angeles": "America/Los_Angeles", "la": "America/Los_Angeles",
  "chicago": "America/Chicago", "denver": "America/Denver",
  "london": "Europe/London", "paris": "Europe/Paris",
  "berlin": "Europe/Berlin", "tokyo": "Asia/Tokyo",
  "sydney": "Australia/Sydney", "toronto": "America/Toronto",
  "vancouver": "America/Vancouver", "mumbai": "Asia/Kolkata",
  "dubai": "Asia/Dubai", "singapore": "Asia/Singapore",
  "hong kong": "Asia/Hong_Kong", "seoul": "Asia/Seoul",
  "beijing": "Asia/Shanghai", "moscow": "Europe/Moscow",
  "istanbul": "Europe/Istanbul", "cairo": "Africa/Cairo",
  "sao paulo": "America/Sao_Paulo", "mexico city": "America/Mexico_City",
  "amsterdam": "Europe/Amsterdam", "rome": "Europe/Rome",
  "madrid": "Europe/Madrid", "bangkok": "Asia/Bangkok",
  "honolulu": "Pacific/Honolulu", "hawaii": "Pacific/Honolulu",
  "lisbon": "Europe/Lisbon", "dublin": "Europe/Dublin",
  "athens": "Europe/Athens", "helsinki": "Europe/Helsinki",
  "warsaw": "Europe/Warsaw", "prague": "Europe/Prague",
  "vienna": "Europe/Vienna", "zurich": "Europe/Zurich",
  "brussels": "Europe/Brussels", "oslo": "Europe/Oslo",
  "stockholm": "Europe/Stockholm", "copenhagen": "Europe/Copenhagen",
  "bucharest": "Europe/Bucharest", "budapest": "Europe/Budapest",
  "jakarta": "Asia/Jakarta", "manila": "Asia/Manila",
  "taipei": "Asia/Taipei", "shanghai": "Asia/Shanghai",
  "kolkata": "Asia/Kolkata", "delhi": "Asia/Kolkata",
  "karachi": "Asia/Karachi", "dhaka": "Asia/Dhaka",
  "riyadh": "Asia/Riyadh", "tehran": "Asia/Tehran",
  "johannesburg": "Africa/Johannesburg", "nairobi": "Africa/Nairobi",
  "lagos": "Africa/Lagos", "casablanca": "Africa/Casablanca",
  "lima": "America/Lima", "bogota": "America/Bogota",
  "buenos aires": "America/Argentina/Buenos_Aires",
  "santiago": "America/Santiago", "anchorage": "America/Anchorage",
  "auckland": "Pacific/Auckland",
};

/** Try to resolve a city name to an IANA timezone by checking Region/City patterns. */
function guessIANAZone(city: string): string | null {
  // Capitalize words and replace spaces with underscores to match IANA format
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

const EIGHT_BALL_RESPONSES = [
  "It is certain.", "It is decidedly so.", "Without a doubt.",
  "Yes, definitely.", "You may rely on it.", "As I see it, yes.",
  "Most likely.", "Outlook good.", "Yes.", "Signs point to yes.",
  "Reply hazy, try again.", "Ask again later.", "Better not tell you now.",
  "Cannot predict now.", "Concentrate and ask again.",
  "Don't count on it.", "My reply is no.", "My sources say no.",
  "Outlook not so good.", "Very doubtful.",
];

const TWINBEE_FACTS = [
  "TwinBee first appeared in 1985 as a Konami arcade game.",
  "TwinBee is a cute 'em up (kawaii shooter) — one of the first of its kind.",
  "The bell power-up system is TwinBee's signature mechanic — juggle bells to change their color!",
  "Yellow bells give points, blue bells increase speed, white bells give twin shots, green bells give a shield.",
  "TwinBee's partner is WinBee, a pink ship piloted by Light's girlfriend Pastel.",
  "The TwinBee series inspired the Parodius games, Konami's parody shooter line.",
  "Detana!! TwinBee (1991) is considered the series' masterpiece.",
  "TwinBee Rainbow Bell Adventure turned the shooter into a platformer.",
  "TwinBee had an anime series called 'TwinBee Paradise' in the 90s.",
  "In Pop'n TwinBee, you could punch enemies by getting close — a mechanic unique to the SNES version.",
  "GwinBee was TwinBee's baby sibling, first appearing in Stinger (1987).",
  "TwinBee's pilot is named Light, and he's the grandson of Dr. Cinnamon.",
  "The TwinBee series has over 15 entries, spanning arcade, console, and handheld.",
  "Parodius (from Parody + Gradius) used TwinBee as a playable character alongside Vic Viper.",
];

export class FunPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "fun";
  }

  get commands(): CommandDef[] {
    return [
      { name: "roll", description: "Dice roller", usage: "!roll <NdN+M>" },
      { name: "8ball", description: "Magic 8-ball", usage: "!8ball <question>" },
      { name: "coin", description: "Coin flip" },
      { name: "time", description: "World clock", usage: "!time <city|@user>..." },
      { name: "hltb", description: "HowLongToBeat lookup", usage: "!hltb <game>" },
      { name: "twinbee", description: "Random TwinBee lore" },
      { name: "poll", description: "Reaction poll", usage: '!poll "Q" "A" "B" ...' },
      { name: "weather", description: "Current weather", usage: "!weather <city|zip[,CC]|@user>" },
      { name: "dadjoke", description: "Random dad joke" },
      { name: "randomwiki", description: "Random Wikipedia article" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "8ball")) {
      await this.handle8ball(ctx);
    } else if (this.isCommand(ctx.body, "roll")) {
      await this.handleRoll(ctx);
    } else if (this.isCommand(ctx.body, "coin")) {
      await this.handleCoin(ctx);
    } else if (this.isCommand(ctx.body, "time")) {
      await this.handleTime(ctx);
    } else if (this.isCommand(ctx.body, "hltb")) {
      await this.handleHltb(ctx);
    } else if (this.isCommand(ctx.body, "twinbee")) {
      await this.handleTwinbee(ctx);
    } else if (this.isCommand(ctx.body, "weather")) {
      await this.handleWeather(ctx);
    } else if (this.isCommand(ctx.body, "poll")) {
      await this.handlePoll(ctx);
    } else if (this.isCommand(ctx.body, "dadjoke")) {
      await this.handleDadJoke(ctx);
    } else if (this.isCommand(ctx.body, "randomwiki")) {
      await this.handleRandomWiki(ctx);
    }
  }

  private async handleRoll(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "roll") || "1d6";
    const match = args.match(/^(\d+)?d(\d+)([+-]\d+)?$/i);

    if (!match) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !roll [N]d<sides>[+/-modifier]\nExamples: !roll d20, !roll 2d6+3");
      return;
    }

    const count = Math.min(parseInt(match[1] || "1"), 100);
    const sides = Math.min(parseInt(match[2]), 1000);
    const modifier = parseInt(match[3] || "0");

    if (sides < 1) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Dice need at least 1 side!");
      return;
    }

    const rolls: number[] = [];
    for (let i = 0; i < count; i++) {
      rolls.push(Math.floor(Math.random() * sides) + 1);
    }

    const sum = rolls.reduce((a, b) => a + b, 0) + modifier;
    const rollStr = count > 1 ? ` (${rolls.join(", ")})` : "";
    const modStr = modifier !== 0 ? ` ${modifier > 0 ? "+" : ""}${modifier}` : "";

    await this.sendMessage(ctx.roomId, `${ctx.sender} rolled ${count}d${sides}${modStr}: ${sum}${rollStr}`);
  }

  private async handle8ball(ctx: MessageContext): Promise<void> {
    const response = EIGHT_BALL_RESPONSES[Math.floor(Math.random() * EIGHT_BALL_RESPONSES.length)];
    await this.sendMessage(ctx.roomId, `The Magic 8-Ball says: ${response}`);
  }

  private async handleCoin(ctx: MessageContext): Promise<void> {
    const result = Math.random() < 0.5 ? "Heads" : "Tails";
    await this.sendMessage(ctx.roomId, `${ctx.sender} flipped a coin: ${result}!`);
  }

  private async handleTime(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "time");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !time <city|@user> [city2] ...");
      return;
    }

    // Split by comma or multiple words that could be cities/@users
    const queries = args.split(/,\s*/).map((s) => s.trim()).filter(Boolean);
    const results: string[] = [];

    for (const query of queries) {
      if (query.startsWith("@")) {
        // Look up user timezone
        const db = getDb();
        const row = db
          .prepare(`SELECT timezone FROM users WHERE user_id = ? AND room_id = ?`)
          .get(query, ctx.roomId) as { timezone: string | null } | undefined;

        if (row?.timezone) {
          const now = DateTime.now().setZone(row.timezone);
          results.push(`${query}: ${now.toFormat("HH:mm (ZZZZZ)")} [${row.timezone}]`);
        } else {
          results.push(`${query}: timezone not set`);
        }
      } else {
        const lower = query.toLowerCase();
        const tz = CITY_TIMEZONE_MAP[lower] || (IANAZone.isValidZone(query) ? query : null) || guessIANAZone(lower);

        if (tz) {
          const now = DateTime.now().setZone(tz);
          results.push(`${query}: ${now.toFormat("HH:mm (ZZZZZ)")} [${tz}]`);
        } else {
          results.push(`${query}: unknown timezone`);
        }
      }
    }

    await this.sendMessage(ctx.roomId, results.join("\n"));
  }

  private async handleHltb(ctx: MessageContext): Promise<void> {
    const game = this.getArgs(ctx.body, "hltb");
    if (!game) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !hltb <game name>");
      return;
    }

    const db = getDb();

    // Check cache (24h TTL)
    const cached = db
      .prepare(`SELECT * FROM hltb_cache WHERE search_term = ? AND fetched_at > datetime('now', '-24 hours')`)
      .get(game.toLowerCase()) as any;

    if (cached) {
      await this.sendHltbResult(ctx.roomId, cached);
      return;
    }

    try {
      const { HowLongToBeatService } = await import("howlongtobeat");
      const service = new HowLongToBeatService();
      const results = await service.search(game);

      if (!results || results.length === 0) {
        await this.sendReply(ctx.roomId, ctx.eventId, `No HLTB results found for "${game}".`);
        return;
      }

      const best = results[0];
      db.prepare(`
        INSERT INTO hltb_cache (game_name, search_term, main_story, main_extra, completionist, data_json)
        VALUES (?, ?, ?, ?, ?, ?)
      `).run(
        best.name,
        game.toLowerCase(),
        best.gameplayMain ?? null,
        best.gameplayMainExtra ?? null,
        best.gameplayCompletionist ?? null,
        JSON.stringify(best)
      );

      await this.sendHltbResult(ctx.roomId, {
        game_name: best.name,
        main_story: best.gameplayMain,
        main_extra: best.gameplayMainExtra,
        completionist: best.gameplayCompletionist,
      });
    } catch (err) {
      logger.error(`HLTB lookup failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "HLTB lookup failed. Try again later.");
    }
  }

  private async sendHltbResult(roomId: string, data: { game_name: string; main_story: number | null; main_extra: number | null; completionist: number | null }): Promise<void> {
    const formatHours = (h: number | null) => (h ? `${h} hours` : "N/A");
    await this.sendMessage(
      roomId,
      `${data.game_name}:\n  Main Story: ${formatHours(data.main_story)}\n  Main + Extra: ${formatHours(data.main_extra)}\n  Completionist: ${formatHours(data.completionist)}`
    );
  }

  private async handleTwinbee(ctx: MessageContext): Promise<void> {
    const fact = TWINBEE_FACTS[Math.floor(Math.random() * TWINBEE_FACTS.length)];
    await this.sendMessage(ctx.roomId, fact);
  }

  private async handleWeather(ctx: MessageContext): Promise<void> {
    const apiKey = process.env.OPENWEATHER_API_KEY;
    if (!apiKey) {
      await this.sendReply(ctx.roomId, ctx.eventId, "OpenWeather API key not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "weather");
    if (!args) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !weather <city> or !weather @user");
      return;
    }

    let query = args;

    // If it's a user mention, look up their timezone and derive a city
    if (args.startsWith("@")) {
      const userId = args.split(/\s/)[0];
      const db = getDb();
      const row = db
        .prepare(`SELECT timezone FROM users WHERE user_id = ? AND room_id = ?`)
        .get(userId, ctx.roomId) as { timezone: string | null } | undefined;

      if (!row?.timezone) {
        await this.sendReply(ctx.roomId, ctx.eventId, `${userId} hasn't set a timezone. Use !weather <city> instead.`);
        return;
      }

      // Try to extract a city name from the IANA zone (e.g., "America/New_York" -> "New York")
      const parts = row.timezone.split("/");
      query = parts[parts.length - 1].replace(/_/g, " ");
    }

    try {
      // Resolve location via geocoding first for accurate results
      const location = await this.resolveWeatherLocation(query, apiKey);
      if (!location) {
        await this.sendReply(ctx.roomId, ctx.eventId, `Location "${query}" not found.`);
        return;
      }

      const url = `https://api.openweathermap.org/data/2.5/weather?lat=${location.lat}&lon=${location.lon}&appid=${encodeURIComponent(apiKey)}&units=metric`;
      const res = await fetch(url);

      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Weather lookup failed. Try again later.");
        return;
      }

      const data = await res.json() as any;
      const temp = data.main.temp;
      const feelsLike = data.main.feels_like;
      const tempF = (temp * 9) / 5 + 32;
      const feelsLikeF = (feelsLike * 9) / 5 + 32;
      const humidity = data.main.humidity;
      const description = data.weather?.[0]?.description ?? "unknown";
      const windSpeed = data.wind?.speed ?? 0;

      const lines = [
        `Weather for ${location.displayName}:`,
        `  ${description.charAt(0).toUpperCase() + description.slice(1)}`,
        `  Temp: ${temp.toFixed(1)}°C / ${tempF.toFixed(1)}°F (feels like ${feelsLike.toFixed(1)}°C / ${feelsLikeF.toFixed(1)}°F)`,
        `  Humidity: ${humidity}% | Wind: ${windSpeed} m/s`,
      ];

      await this.sendMessage(ctx.roomId, lines.join("\n"));
    } catch (err) {
      logger.error(`Weather lookup failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Weather lookup failed. Try again later.");
    }
  }

  /**
   * Resolve a user query to coordinates using OpenWeather Geocoding API.
   * Handles zip codes (with optional country: "90210" or "90210,US"),
   * and disambiguates city names by preferring US matches first, then
   * returning the most relevant result with state/country info.
   */
  private async resolveWeatherLocation(
    query: string,
    apiKey: string
  ): Promise<{ lat: number; lon: number; displayName: string } | null> {
    const trimmed = query.trim();

    // Check if input looks like a zip/postal code (digits, optionally with country suffix)
    const zipMatch = trimmed.match(/^(\d{3,10})(?:\s*,\s*([A-Za-z]{2}))?$/);
    if (zipMatch) {
      const zip = zipMatch[1];
      const country = zipMatch[2]?.toUpperCase() ?? "US";
      const url = `https://api.openweathermap.org/geo/1.0/zip?zip=${encodeURIComponent(zip)},${country}&appid=${encodeURIComponent(apiKey)}`;
      const res = await fetch(url);
      if (res.ok) {
        const data = await res.json() as any;
        if (data.lat != null && data.lon != null) {
          return {
            lat: data.lat,
            lon: data.lon,
            displayName: `${data.name ?? zip}, ${data.country ?? country}`,
          };
        }
      }
      // If US zip failed and no country was specified, don't fall through to city search for pure numbers
      if (!zipMatch[2]) return null;
    }

    // City name lookup via geocoding API (returns up to 5 results)
    const geoUrl = `https://api.openweathermap.org/geo/1.0/direct?q=${encodeURIComponent(trimmed)}&limit=5&appid=${encodeURIComponent(apiKey)}`;
    const geoRes = await fetch(geoUrl);
    if (!geoRes.ok) return null;

    const results = await geoRes.json() as any[];
    if (!results || results.length === 0) return null;

    // If there's only one result, use it directly
    // If multiple, prefer a US match for ambiguous names, otherwise take the first
    let best = results[0];
    if (results.length > 1) {
      const usMatch = results.find((r: any) => r.country === "US");
      if (usMatch) best = usMatch;
    }

    const state = best.state ? `, ${best.state}` : "";
    const country = best.country ? `, ${best.country}` : "";
    return {
      lat: best.lat,
      lon: best.lon,
      displayName: `${best.name}${state}${country}`,
    };
  }

  private async handlePoll(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "poll");
    // Parse quoted strings: !poll "Question" "Option A" "Option B" ...
    const matches = args.match(/"([^"]+)"/g);

    if (!matches || matches.length < 3) {
      await this.sendReply(ctx.roomId, ctx.eventId, 'Usage: !poll "Question?" "Option A" "Option B" ...');
      return;
    }

    const question = matches[0].replace(/"/g, "");
    const options = matches.slice(1).map((m) => m.replace(/"/g, ""));

    if (options.length > 10) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Maximum 10 options allowed.");
      return;
    }

    const numberEmojis = ["1\uFE0F\u20E3", "2\uFE0F\u20E3", "3\uFE0F\u20E3", "4\uFE0F\u20E3", "5\uFE0F\u20E3", "6\uFE0F\u20E3", "7\uFE0F\u20E3", "8\uFE0F\u20E3", "9\uFE0F\u20E3", "\uD83D\uDD1F"];

    const lines = [`Poll: ${question}`];
    options.forEach((opt, i) => {
      lines.push(`${numberEmojis[i]} ${opt}`);
    });

    const pollEventId = await this.sendMessage(ctx.roomId, lines.join("\n"));

    // Auto-react with number emojis (fire-and-forget with retry)
    for (let i = 0; i < options.length; i++) {
      this.sendReact(ctx.roomId, pollEventId, numberEmojis[i]);
    }
  }

  private async handleDadJoke(ctx: MessageContext): Promise<void> {
    try {
      const res = await fetch("https://icanhazdadjoke.com/", {
        headers: { Accept: "application/json" },
      });
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't fetch a dad joke right now.");
        return;
      }
      const data = (await res.json()) as { joke: string };
      await this.sendMessage(ctx.roomId, data.joke);
    } catch (err) {
      logger.error(`Dad joke fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Dad joke machine broke.");
    }
  }

  private async handleRandomWiki(ctx: MessageContext): Promise<void> {
    try {
      const res = await fetch("https://en.wikipedia.org/api/rest_v1/page/random/summary");
      if (!res.ok) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't fetch a random article.");
        return;
      }
      const data = (await res.json()) as any;
      const title = data.title ?? "Unknown";
      let extract = data.extract ?? "";
      if (extract.length > 500) extract = extract.slice(0, 500) + "...";
      const pageUrl = data.content_urls?.desktop?.page ?? "";
      await this.sendMessage(ctx.roomId, `${title}\n\n${extract}\n\nRead more: ${pageUrl}`);
    } catch (err) {
      logger.error(`Random wiki fetch failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch a random article.");
    }
  }
}
