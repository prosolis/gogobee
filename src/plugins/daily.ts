import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import { RemindersPlugin } from "./reminders";
import { WotdPlugin } from "./wotd";
import { HolidaysPlugin } from "./holidays";
import { GamingPlugin } from "./gaming";
import { BirthdayPlugin } from "./birthday";
import { AnimePlugin } from "./anime";
import { MoviesPlugin } from "./movies";
import { ConcertsPlugin } from "./concerts";
import logger from "../utils/logger";

interface ScheduledJob {
  job_name: string;
  hour: number;
  minute: number;
  enabled: number;
}

export class DailyScheduler extends Plugin {
  private remindersPlugin: RemindersPlugin;
  private wotdPlugin: WotdPlugin;
  private holidaysPlugin: HolidaysPlugin;
  private gamingPlugin: GamingPlugin;
  private birthdayPlugin: BirthdayPlugin;
  private animePlugin: AnimePlugin;
  private moviesPlugin: MoviesPlugin;
  private concertsPlugin: ConcertsPlugin;
  private botRooms: string[];
  private firedToday = new Set<string>();
  private tickInterval: ReturnType<typeof setInterval> | null = null;
  private reminderInterval: ReturnType<typeof setInterval> | null = null;

  constructor(
    client: IMatrixClient,
    remindersPlugin: RemindersPlugin,
    wotdPlugin: WotdPlugin,
    holidaysPlugin: HolidaysPlugin,
    gamingPlugin: GamingPlugin,
    birthdayPlugin: BirthdayPlugin,
    animePlugin: AnimePlugin,
    moviesPlugin: MoviesPlugin,
    concertsPlugin: ConcertsPlugin,
    botRooms: string[]
  ) {
    super(client);
    this.remindersPlugin = remindersPlugin;
    this.wotdPlugin = wotdPlugin;
    this.holidaysPlugin = holidaysPlugin;
    this.gamingPlugin = gamingPlugin;
    this.birthdayPlugin = birthdayPlugin;
    this.animePlugin = animePlugin;
    this.moviesPlugin = moviesPlugin;
    this.concertsPlugin = concertsPlugin;
    this.botRooms = botRooms;
  }

  get name() {
    return "daily";
  }

  get commands(): CommandDef[] {
    return [
      { name: "schedule", description: "Adjust scheduled post time (admin)", usage: "!schedule <job> <HH:MM>", adminOnly: true },
    ];
  }

  start(): void {
    // Check scheduled jobs every 60 seconds
    this.tickInterval = setInterval(() => this.tick(), 60_000);

    // Check reminders every 30 seconds
    this.reminderInterval = setInterval(() => {
      this.remindersPlugin.checkReminders().catch((err) => {
        logger.error(`Reminder check failed: ${err}`);
      });
    }, 30_000);

    logger.info("DailyScheduler started");
  }

  /**
   * Run missed jobs. Call this AFTER the Matrix client has fully started
   * and E2EE is initialized.
   */
  async catchUpMissedJobs(): Promise<void> {
    // Short delay to ensure crypto is fully ready
    await new Promise((resolve) => setTimeout(resolve, 5000));
    await this.catchUp();
  }

  private async catchUp(): Promise<void> {
    const now = new Date();
    const currentHour = now.getUTCHours();
    const currentMinute = now.getUTCMinutes();
    const today = now.toISOString().slice(0, 10);

    const db = getDb();
    const jobs = db.prepare(`SELECT * FROM scheduler_config WHERE enabled = 1`).all() as ScheduledJob[];

    for (const job of jobs) {
      const scheduledMinutes = job.hour * 60 + job.minute;
      const currentMinutes = currentHour * 60 + currentMinute;

      // If the scheduled time has already passed today, run it now
      if (scheduledMinutes <= currentMinutes) {
        const key = `${job.job_name}:${today}`;
        logger.info(`Catching up missed job: ${job.job_name}`);
        try {
          await this.runJob(job.job_name);
        } catch (err) {
          logger.error(`Catch-up for ${job.job_name} failed: ${err}`);
        }
        // Only mark as fired if the job actually produced output.
        // Jobs that use DB logs (holidays, wotd) check for existing entries,
        // so leaving firedToday unset lets the tick retry on the next minute.
        if (this.didJobComplete(job.job_name, today)) {
          this.firedToday.add(key);
        } else {
          logger.warn(`Job ${job.job_name} did not produce output — will retry on next tick`);
        }
      }
    }
  }

  stop(): void {
    if (this.tickInterval) clearInterval(this.tickInterval);
    if (this.reminderInterval) clearInterval(this.reminderInterval);
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "schedule")) {
      await this.handleSchedule(ctx);
    }
  }

  private tick(): void {
    const now = new Date();
    const currentHour = now.getUTCHours();
    const currentMinute = now.getUTCMinutes();
    const today = now.toISOString().slice(0, 10);

    const db = getDb();
    const jobs = db.prepare(`SELECT * FROM scheduler_config WHERE enabled = 1`).all() as ScheduledJob[];

    for (const job of jobs) {
      const key = `${job.job_name}:${today}`;
      if (this.firedToday.has(key)) continue;
      // Fire at the scheduled minute, or any minute after if it was missed (e.g. connection blip)
      const scheduledMinutes = job.hour * 60 + job.minute;
      const currentMinutes = currentHour * 60 + currentMinute;
      if (currentMinutes < scheduledMinutes) continue;

      this.runJob(job.job_name).then(() => {
        if (this.didJobComplete(job.job_name, today)) {
          this.firedToday.add(key);
        }
      }).catch((err) => {
        logger.error(`Scheduled job ${job.job_name} failed: ${err}`);
      });
    }

    // Clean up old entries from firedToday (keep only today's)
    for (const key of this.firedToday) {
      if (!key.endsWith(today)) {
        this.firedToday.delete(key);
      }
    }
  }

  private async runJob(jobName: string): Promise<void> {
    logger.info(`Running scheduled job: ${jobName}`);

    switch (jobName) {
      case "prefetch":
        await this.runPrefetch();
        break;
      case "maintenance":
        this.runMaintenance();
        break;
      case "wotd":
        await this.wotdPlugin.postWotd(this.botRooms);
        break;
      case "holidays":
        await this.holidaysPlugin.postHolidays(this.botRooms);
        break;
      case "releases":
        await this.gamingPlugin.postReleases(this.botRooms);
        break;
      case "birthday_check":
        await this.birthdayPlugin.checkAndPost(this.botRooms);
        break;
      case "anime_releases":
        await this.animePlugin.postDailyReleases(this.botRooms);
        break;
      case "movie_releases":
        await this.moviesPlugin.postDailyReleases(this.botRooms);
        break;
      case "concert_digest":
        await this.concertsPlugin.postWeeklyDigest(this.botRooms);
        break;
      default:
        logger.warn(`Unknown scheduled job: ${jobName}`);
    }
  }

  /**
   * Prefetch API data for holidays and WOTD so the post step is network-free.
   * Runs early (default 00:05 UTC). Each prefetch is individually error-handled
   * so one failure doesn't block the others.
   */
  private async runPrefetch(): Promise<void> {
    const results: string[] = [];

    try {
      await this.holidaysPlugin.prefetch();
      results.push("holidays");
    } catch (err) {
      logger.error(`Holiday prefetch failed: ${err}`);
    }

    try {
      await this.wotdPlugin.prefetch();
      results.push("wotd");
    } catch (err) {
      logger.error(`WOTD prefetch failed: ${err}`);
    }

    logger.info(`Prefetch complete: ${results.join(", ") || "none succeeded"}`);
  }

  /**
   * Nightly maintenance: prune old data to prevent DB bloat.
   * Runs after prefetch (default 00:15 UTC).
   */
  private runMaintenance(): void {
    const db = getDb();
    let totalDeleted = 0;

    const prune = (label: string, sql: string, ...params: any[]) => {
      try {
        const result = db.prepare(sql).run(...params);
        if (result.changes > 0) {
          logger.info(`Maintenance: pruned ${result.changes} rows from ${label}`);
          totalDeleted += result.changes;
        }
      } catch (err) {
        logger.error(`Maintenance: failed to prune ${label}: ${err}`);
      }
    };

    // llm_classifications — aggregates already in sentiment_stats, potty_mouth, insult_log
    prune("llm_classifications", `DELETE FROM llm_classifications WHERE classified_at < unixepoch() - ?`, 30 * 86400);

    // xp_log — audit trail, not queried by any command
    prune("xp_log", `DELETE FROM xp_log WHERE granted_at < datetime('now', '-30 days')`);

    // command_usage — rate limit tracking, old entries useless
    prune("command_usage", `DELETE FROM command_usage WHERE date < date('now', '-7 days')`);

    // rep_cooldowns — only 24h cooldown, stale entries accumulate
    prune("rep_cooldowns", `DELETE FROM rep_cooldowns WHERE granted_at < datetime('now', '-2 days')`);

    // daily_prefetch — only today's matters
    prune("daily_prefetch", `DELETE FROM daily_prefetch WHERE date < date('now', '-3 days')`);

    // Cache tables — stale entries never deleted by app code
    prune("url_cache", `DELETE FROM url_cache WHERE cached_at < unixepoch() - ?`, 7 * 86400);
    prune("retro_cache", `DELETE FROM retro_cache WHERE cached_at < unixepoch() - ?`, 14 * 86400);
    prune("urban_cache", `DELETE FROM urban_cache WHERE cached_at < unixepoch() - ?`, 7 * 86400);
    prune("stocks_cache", `DELETE FROM stocks_cache WHERE cached_at < unixepoch() - ?`, 1 * 86400);
    prune("hltb_cache", `DELETE FROM hltb_cache WHERE fetched_at < datetime('now', '-30 days')`);
    prune("releases_cache", `DELETE FROM releases_cache WHERE fetched_at < datetime('now', '-30 days')`);
    prune("anime_cache", `DELETE FROM anime_cache WHERE cached_at < unixepoch() - ?`, 30 * 86400);
    prune("movie_cache", `DELETE FROM movie_cache WHERE cached_at < unixepoch() - ?`, 7 * 86400);
    prune("concerts_cache", `DELETE FROM concerts_cache WHERE cached_at < unixepoch() - ?`, 14 * 86400);

    // shade_log — if shade feature is disabled, this is dead weight
    prune("shade_log", `DELETE FROM shade_log WHERE classified_at < datetime('now', '-30 days')`);

    logger.info(`Maintenance complete: ${totalDeleted} total rows pruned`);
  }

  /**
   * Check if a job actually produced output by looking at its DB log.
   * Jobs without a log table are assumed to have completed.
   */
  private didJobComplete(jobName: string, today: string): boolean {
    try {
      const db = getDb();
      switch (jobName) {
        case "prefetch": {
          // Prefetch is done if at least one of the caches was populated
          const row = db.prepare(`SELECT job_name FROM daily_prefetch WHERE date = ? LIMIT 1`).get(today);
          return !!row;
        }
        case "holidays": {
          const row = db.prepare(`SELECT id FROM holidays_log WHERE date = ? LIMIT 1`).get(today);
          return !!row;
        }
        case "wotd": {
          const row = db.prepare(`SELECT id FROM wotd_log WHERE date = ? LIMIT 1`).get(today);
          return !!row;
        }
        default:
          return true;
      }
    } catch {
      return true;
    }
  }

  private async handleSchedule(ctx: MessageContext): Promise<void> {
    if (!this.isAdmin(ctx.sender)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "This command is admin-only.");
      return;
    }

    const args = this.getArgs(ctx.body, "schedule");
    const match = args.match(/^(\w+)\s+(\d{1,2}):(\d{2})$/);

    if (!match) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !schedule <job> <HH:MM> (UTC)\nJobs: prefetch, maintenance, wotd, holidays, releases, birthday_check, anime_releases, movie_releases, concert_digest");
      return;
    }

    const [, jobName, hourStr, minuteStr] = match;
    const hour = parseInt(hourStr);
    const minute = parseInt(minuteStr);

    if (hour < 0 || hour > 23 || minute < 0 || minute > 59) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Invalid time. Use HH:MM in 24-hour UTC format.");
      return;
    }

    const db = getDb();
    const result = db
      .prepare(`UPDATE scheduler_config SET hour = ?, minute = ? WHERE job_name = ?`)
      .run(hour, minute, jobName);

    if (result.changes === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Unknown job "${jobName}". Valid jobs: prefetch, maintenance, wotd, holidays, releases, birthday_check, anime_releases, movie_releases, concert_digest`);
      return;
    }

    // Clear today's fired status so it can re-fire at the new time
    const today = new Date().toISOString().slice(0, 10);
    this.firedToday.delete(`${jobName}:${today}`);

    await this.sendReply(
      ctx.roomId,
      ctx.eventId,
      `Scheduled "${jobName}" updated to ${hour.toString().padStart(2, "0")}:${minute.toString().padStart(2, "0")} UTC.`
    );
  }
}
