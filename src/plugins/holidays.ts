import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

// Countries/sources considered "Western" for featured selection
const WESTERN_COUNTRIES = new Set(["US", "GB", "CA", "AU", "NZ", "IE"]);

const ASIAN_COUNTRIES: Record<string, string> = {
  JP: "Japan",
  CN: "China",
  KR: "South Korea",
  IN: "India",
  TH: "Thailand",
  VN: "Vietnam",
  TW: "Taiwan",
  PH: "Philippines",
};

function getAsianCountryCodes(): string[] {
  const override = process.env.ASIAN_HOLIDAY_COUNTRIES;
  if (override) return override.split(",").map((s) => s.trim().toUpperCase()).filter(Boolean);
  return Object.keys(ASIAN_COUNTRIES);
}

function countryLabel(code: string): string {
  return ASIAN_COUNTRIES[code] ?? code;
}

const RANGE_CACHE_TTL_MS = 60 * 60 * 1000; // 1 hour

interface RangeCacheEntry {
  results: { date: string; holiday: Holiday }[];
  fetchedAt: number;
}

interface Holiday {
  name: string;
  description?: string;
  country?: string;
  type?: string;
  source: "calendarific" | "hebcal" | "aladhan";
}

export class HolidaysPlugin extends Plugin {
  private rangeCache = new Map<string, RangeCacheEntry>();

  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "holidays";
  }

  get commands(): CommandDef[] {
    return [{ name: "holidays", description: "Holidays and observances", usage: "!holidays [week|month]" }];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "holidays")) {
      await this.handleHolidays(ctx);
    }
  }

  private async handleHolidays(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "holidays").trim().toLowerCase();

    if (args === "week" || args === "month") {
      await this.handleHolidaysRange(ctx, args);
      return;
    }

    // Default: today — try DB cache first, fall back to live fetch
    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    let holidays: Holiday[];

    const row = db
      .prepare(`SELECT holidays_json FROM holidays_log WHERE room_id = ? AND date = ?`)
      .get(ctx.roomId, today) as { holidays_json: string } | undefined;

    if (row) {
      holidays = JSON.parse(row.holidays_json);
    } else {
      // Live fetch from all sources
      holidays = await this.fetchAllForDate(today);

      // Cache the result so subsequent calls don't re-fetch
      if (holidays.length > 0) {
        db.prepare(`INSERT OR IGNORE INTO holidays_log (room_id, date, holidays_json) VALUES (?, ?, ?)`)
          .run(ctx.roomId, today, JSON.stringify(holidays));
      }
    }

    if (holidays.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No holidays found for today.");
      return;
    }

    const asianCodes = new Set(Object.keys(ASIAN_COUNTRIES));
    const lines = holidays.map((h) => {
      let src = "";
      if (h.source === "hebcal") src = " (Jewish)";
      else if (h.source === "aladhan") src = " (Islamic)";
      else if (h.country && asianCodes.has(h.country)) src = ` (${countryLabel(h.country)})`;
      return `- ${h.name}${src}${h.description ? `: ${h.description}` : ""}`;
    });

    await this.sendMessage(ctx.roomId, `Today's holidays & observances:\n${lines.join("\n")}`);
  }

  private async handleHolidaysRange(ctx: MessageContext, range: "week" | "month"): Promise<void> {
    const today = new Date();
    const startDate = today.toISOString().slice(0, 10);
    const endDate = new Date(today);
    endDate.setDate(endDate.getDate() + (range === "week" ? 7 : 30));
    const endDateStr = endDate.toISOString().slice(0, 10);
    const label = range === "week" ? "This Week" : "This Month";

    const cacheKey = `${startDate}:${endDateStr}`;
    const cached = this.rangeCache.get(cacheKey);
    let allResults: { date: string; holiday: Holiday }[];

    if (cached && Date.now() - cached.fetchedAt < RANGE_CACHE_TTL_MS) {
      allResults = cached.results;
    } else {
      // Fetch from Calendarific (US + Asian countries) and HebCal in parallel
      const asianCodes = getAsianCountryCodes();
      const [calendarificHolidays, hebcalHolidays, ...asianRangeResults] = await Promise.all([
        this.fetchCalendarificRange(startDate, endDateStr),
        this.fetchHebcalRange(startDate, endDateStr),
        ...asianCodes.map((code) => this.fetchCalendarificRange(startDate, endDateStr, code)),
      ]);

      allResults = [...calendarificHolidays, ...hebcalHolidays, ...asianRangeResults.flat()];
      this.rangeCache.set(cacheKey, { results: allResults, fetchedAt: Date.now() });
    }

    const asianCodesSet = new Set(Object.keys(ASIAN_COUNTRIES));
    const byDate = new Map<string, Holiday[]>();
    for (const { date, holiday } of allResults) {
      if (date < startDate || date > endDateStr) continue;
      if (!byDate.has(date)) byDate.set(date, []);
      byDate.get(date)!.push(holiday);
    }

    const sortedDates = [...byDate.keys()].sort();
    if (sortedDates.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No holidays found for ${label.toLowerCase()}.`);
      return;
    }

    const lines: string[] = [`Holidays & Observances — ${label}:`];
    for (const date of sortedDates) {
      const dateLabel = new Date(date + "T00:00:00Z").toLocaleDateString("en-US", {
        weekday: "short",
        month: "short",
        day: "numeric",
        timeZone: "UTC",
      });
      const holidays = byDate.get(date)!;
      lines.push(`\n${dateLabel}:`);
      for (const h of holidays) {
        let src = "";
        if (h.source === "hebcal") src = " (Jewish)";
        else if (h.source === "aladhan") src = " (Islamic)";
        else if (h.country && asianCodesSet.has(h.country)) src = ` (${countryLabel(h.country)})`;
        lines.push(`  - ${h.name}${src}`);
      }
    }

    // Truncate if too long for a single message
    const message = lines.join("\n");
    if (message.length > 4000) {
      await this.sendMessage(ctx.roomId, message.slice(0, 3950) + "\n\n... (truncated)");
    } else {
      await this.sendMessage(ctx.roomId, message);
    }
  }

  private async fetchCalendarificRange(startDate: string, endDate: string, country: string = "US"): Promise<{ date: string; holiday: Holiday }[]> {
    const apiKey = process.env.CALENDARIFIC_API_KEY;
    if (!apiKey) return [];

    try {
      // Calendarific supports year+month queries — fetch each month in the range
      const start = new Date(startDate + "T00:00:00Z");
      const end = new Date(endDate + "T00:00:00Z");
      const results: { date: string; holiday: Holiday }[] = [];
      const fetchedMonths = new Set<string>();

      for (let d = new Date(start); d <= end; d.setUTCMonth(d.getUTCMonth() + 1)) {
        const monthKey = `${d.getUTCFullYear()}-${d.getUTCMonth() + 1}`;
        if (fetchedMonths.has(monthKey)) continue;
        fetchedMonths.add(monthKey);

        const url = `https://calendarific.com/api/v2/holidays?api_key=${encodeURIComponent(apiKey)}&country=${encodeURIComponent(country)}&year=${d.getUTCFullYear()}&month=${d.getUTCMonth() + 1}`;
        const res = await fetch(url);
        if (!res.ok) continue;

        const data = await res.json() as any;
        const holidays = data.response?.holidays ?? [];

        for (const h of holidays) {
          const hDate = h.date?.iso?.slice(0, 10);
          if (!hDate) continue;
          results.push({
            date: hDate,
            holiday: {
              name: h.name,
              description: h.description,
              country: h.country?.id ?? country,
              type: h.type?.[0] ?? "observance",
              source: "calendarific",
            },
          });
        }
      }

      return results;
    } catch (err) {
      logger.error(`Calendarific range fetch failed: ${err}`);
      return [];
    }
  }

  private async fetchHebcalRange(startDate: string, endDate: string): Promise<{ date: string; holiday: Holiday }[]> {
    try {
      const url = `https://www.hebcal.com/hebcal?cfg=json&v=1&start=${startDate}&end=${endDate}&maj=on&min=on&mod=on&nx=on&ss=on`;
      const res = await fetch(url);
      if (!res.ok) return [];

      const data = await res.json() as any;
      const events = data.items ?? [];

      return events.map((evt: any) => ({
        date: evt.date?.slice(0, 10) ?? startDate,
        holiday: {
          name: evt.title,
          description: evt.memo ?? undefined,
          source: "hebcal" as const,
        },
      }));
    } catch (err) {
      logger.error(`HebCal range fetch failed: ${err}`);
      return [];
    }
  }

  /**
   * Called by DailyScheduler to post the morning holiday summary.
   */
  async postHolidays(roomIds: string[]): Promise<void> {
    const today = new Date().toISOString().slice(0, 10);
    const db = getDb();

    // Already posted?
    const existing = db.prepare(`SELECT id FROM holidays_log WHERE date = ? LIMIT 1`).get(today);
    if (existing) return;

    const { holidays, hebrewDate, hijriDate } = await this.fetchAllForDate(today, true);

    if (holidays.length === 0) {
      logger.info("No holidays found for today, skipping post.");
      return;
    }

    // Build date header
    const gregorianDate = new Date().toLocaleDateString("en-US", {
      weekday: "long",
      year: "numeric",
      month: "long",
      day: "numeric",
    });

    const dateHeader = [
      gregorianDate,
      hebrewDate ? `Hebrew: ${hebrewDate}` : null,
      hijriDate ? `Hijri: ${hijriDate}` : null,
    ]
      .filter(Boolean)
      .join(" | ");

    // Select featured holiday (prefer non-Western)
    const featured = this.selectFeatured(holidays);

    // Build sections
    const asianCodesSet = new Set(Object.keys(ASIAN_COUNTRIES));
    const religiousHolidays = holidays.filter(
      (h) => h.source === "hebcal" || h.source === "aladhan" || h.type === "religious"
    );
    const asianHolidays = holidays.filter(
      (h) => h.source === "calendarific" && h.country && asianCodesSet.has(h.country)
    );
    const otherHolidays = holidays.filter(
      (h) => h.source === "calendarific" && h.type !== "religious" && (!h.country || !asianCodesSet.has(h.country))
    );

    const lines: string[] = [dateHeader, ""];

    if (featured) {
      lines.push(`Featured: ${featured.name}${featured.description ? ` — ${featured.description}` : ""}`);
      lines.push("");
    }

    if (religiousHolidays.length > 0) {
      lines.push("Religious Observances:");
      for (const h of religiousHolidays) {
        lines.push(`  - ${h.name}`);
      }
      lines.push("");
    }

    if (asianHolidays.length > 0) {
      lines.push("Asian Holidays:");
      for (const h of asianHolidays) {
        lines.push(`  - ${h.name} (${countryLabel(h.country!)})`);
      }
      lines.push("");
    }

    if (otherHolidays.length > 0) {
      lines.push("Other Observances:");
      for (const h of otherHolidays) {
        lines.push(`  - ${h.name}`);
      }
    }

    const message = lines.join("\n").trim();

    for (const roomId of roomIds) {
      try {
        await this.sendMessage(roomId, message);
        db.prepare(`INSERT OR IGNORE INTO holidays_log (room_id, date, holidays_json) VALUES (?, ?, ?)`).run(
          roomId,
          today,
          JSON.stringify(holidays)
        );
      } catch (err) {
        logger.error(`Failed to post holidays to ${roomId}: ${err}`);
      }
    }
  }

  /**
   * Fetch holidays for a given date from all sources (Calendarific US + Asian, HebCal, Aladhan).
   */
  private async fetchAllForDate(date: string): Promise<Holiday[]>;
  private async fetchAllForDate(date: string, withCalendarDates: true): Promise<{ holidays: Holiday[]; hebrewDate: string | null; hijriDate: string | null }>;
  private async fetchAllForDate(date: string, withCalendarDates?: boolean): Promise<Holiday[] | { holidays: Holiday[]; hebrewDate: string | null; hijriDate: string | null }> {
    const holidays: Holiday[] = [];
    const asianCodes = getAsianCountryCodes();

    const [calendarific, hebcal, aladhan, ...asianResults] = await Promise.allSettled([
      this.fetchCalendarific(date),
      this.fetchHebcal(date),
      this.fetchAladhan(date),
      ...asianCodes.map((code) => this.fetchCalendarificForCountry(date, code)),
    ]);

    if (calendarific.status === "fulfilled") holidays.push(...calendarific.value);
    if (hebcal.status === "fulfilled") holidays.push(...hebcal.value);
    if (aladhan.status === "fulfilled") holidays.push(...aladhan.value);
    for (const result of asianResults) {
      if (result.status === "fulfilled") holidays.push(...result.value);
    }

    if (withCalendarDates) {
      const hebrewDate = hebcal.status === "fulfilled" ? (hebcal.value as any).__hebrewDate ?? null : null;
      const hijriDate = aladhan.status === "fulfilled" ? (aladhan.value as any).__hijriDate ?? null : null;
      return { holidays, hebrewDate, hijriDate };
    }

    return holidays;
  }

  private selectFeatured(holidays: Holiday[]): Holiday | null {
    // Prefer non-Western holidays
    const nonWestern = holidays.filter(
      (h) => !h.country || !WESTERN_COUNTRIES.has(h.country)
    );

    if (nonWestern.length > 0) {
      return nonWestern[Math.floor(Math.random() * nonWestern.length)];
    }

    return holidays.length > 0 ? holidays[0] : null;
  }

  private async fetchCalendarific(date: string): Promise<Holiday[]> {
    return this.fetchCalendarificForCountry(date, "US");
  }

  private async fetchCalendarificForCountry(date: string, country: string): Promise<Holiday[]> {
    const apiKey = process.env.CALENDARIFIC_API_KEY;
    if (!apiKey) return [];

    try {
      const [year, month, day] = date.split("-");
      const url = `https://calendarific.com/api/v2/holidays?api_key=${encodeURIComponent(apiKey)}&country=${encodeURIComponent(country)}&year=${year}&month=${month}&day=${day}`;
      const res = await fetch(url);
      if (!res.ok) {
        logger.warn(`Calendarific API (${country}) returned ${res.status}`);
        return [];
      }

      const data = await res.json() as any;
      const holidays = data.response?.holidays ?? [];

      return holidays.map((h: any) => ({
        name: h.name,
        description: h.description,
        country: h.country?.id ?? country,
        type: h.type?.[0] ?? "observance",
        source: "calendarific" as const,
      }));
    } catch (err) {
      logger.error(`Calendarific fetch (${country}) failed: ${err}`);
      return [];
    }
  }

  private async fetchHebcal(date: string): Promise<Holiday[] & { __hebrewDate?: string }> {
    try {
      const [year, month, day] = date.split("-");
      const url = `https://www.hebcal.com/converter?cfg=json&gy=${year}&gm=${parseInt(month)}&gd=${parseInt(day)}&g2h=1`;
      const res = await fetch(url);
      if (!res.ok) {
        logger.warn(`HebCal API returned ${res.status}`);
        return [];
      }

      const data = await res.json() as any;

      const result: Holiday[] & { __hebrewDate?: string } = [];
      result.__hebrewDate = data.hebrew ?? null;

      // Fetch events for this date
      const eventsUrl = `https://www.hebcal.com/hebcal?cfg=json&v=1&start=${date}&end=${date}&maj=on&min=on&mod=on&nx=on&ss=on`;
      const eventsRes = await fetch(eventsUrl);
      if (eventsRes.ok) {
        const eventsData = await eventsRes.json() as any;
        const events = eventsData.items ?? [];
        for (const evt of events) {
          result.push({
            name: evt.title,
            description: evt.memo ?? undefined,
            source: "hebcal",
          });
        }
      }

      return result;
    } catch (err) {
      logger.error(`HebCal fetch failed: ${err}`);
      return [];
    }
  }

  private async fetchAladhan(date: string): Promise<Holiday[] & { __hijriDate?: string }> {
    try {
      const [year, month, day] = date.split("-");
      const url = `https://api.aladhan.com/v1/timings/${day}-${month}-${year}?latitude=21.4225&longitude=39.8262`;
      const res = await fetch(url, { redirect: "follow" });
      if (!res.ok) {
        logger.warn(`Aladhan API returned ${res.status}`);
        return [];
      }

      const data = await res.json() as any;
      const hijri = data.data?.date?.hijri;

      const result: Holiday[] & { __hijriDate?: string } = [];
      if (hijri) {
        result.__hijriDate = `${hijri.day} ${hijri.month?.en} ${hijri.year} AH`;

        // Check for holidays in the response
        const holidays = hijri.holidays ?? [];
        for (const name of holidays) {
          result.push({
            name,
            source: "aladhan",
          });
        }
      }

      return result;
    } catch (err) {
      logger.error(`Aladhan fetch failed: ${err}`);
      return [];
    }
  }
}
