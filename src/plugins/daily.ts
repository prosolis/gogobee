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
        this.firedToday.add(key);
        logger.info(`Catching up missed job: ${job.job_name}`);
        await this.runJob(job.job_name);
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
      if (job.hour !== currentHour || job.minute !== currentMinute) continue;

      this.firedToday.add(key);
      this.runJob(job.job_name).catch((err) => {
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

  private async handleSchedule(ctx: MessageContext): Promise<void> {
    if (!this.isAdmin(ctx.sender)) {
      await this.sendReply(ctx.roomId, ctx.eventId, "This command is admin-only.");
      return;
    }

    const args = this.getArgs(ctx.body, "schedule");
    const match = args.match(/^(\w+)\s+(\d{1,2}):(\d{2})$/);

    if (!match) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !schedule <job> <HH:MM> (UTC)\nJobs: wotd, holidays, releases, birthday_check, anime_releases, movie_releases, concert_digest");
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
      await this.sendReply(ctx.roomId, ctx.eventId, `Unknown job "${jobName}". Valid jobs: wotd, holidays, releases, birthday_check, anime_releases, movie_releases, concert_digest`);
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
