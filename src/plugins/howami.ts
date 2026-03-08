import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

const OLLAMA_HOST = (() => {
  const raw = process.env.OLLAMA_HOST ?? "";
  return raw && !raw.startsWith("http") ? `http://${raw}` : raw;
})();
const OLLAMA_MODEL = process.env.OLLAMA_MODEL ?? "";
const LLM_ENABLED = OLLAMA_HOST !== "" && OLLAMA_MODEL !== "";
const OLLAMA_TIMEOUT_MS = 45_000;
const OLLAMA_NUM_CTX = 16384;

interface UserProfile {
  // user_stats
  totalMessages: number;
  totalWords: number;
  totalQuestions: number;
  totalExclamations: number;
  totalEmojis: number;
  totalLinks: number;
  totalImages: number;
  avgWordsPerMessage: number;
  longestMessage: number;
  currentStreak: number;
  longestStreak: number;
  hourlyDistribution: Record<string, number>;
  dailyDistribution: Record<string, number>;
  // users
  xp: number;
  level: number;
  rep: number;
  // potty_mouth
  profanityTotal: number;
  profanitySeverity: { mild: number; moderate: number; scorched: number };
  // insult_log
  insultsThrown: number;
  insultsDirect: number;
  insultsIndirect: number;
  timesTargeted: number;
  // trivia
  triviaCorrect: number;
  triviaPoints: number;
  triviaStreak: number;
  triviaFastestMs: number | null;
  // achievements
  achievements: string[];
  totalAchievements: number;
  // sentiment breakdown
  sentiments: Record<string, number>;
  // wotd
  wotdCorrect: number;
  wotdAttempts: number;
  // activity
  peakHour: number | null;
  peakDay: string | null;
  firstSeenDate: string | null;
}

const DAY_NAMES = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export class HowAmIPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "howami";
  }

  get commands(): CommandDef[] {
    return [
      { name: "howami", description: "Get roasted based on your actual stats", usage: "!howami [@user]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (!this.isCommand(ctx.body, "howami")) return;
    await this.handleHowAmI(ctx);
  }

  private async handleHowAmI(ctx: MessageContext): Promise<void> {
    if (!LLM_ENABLED) {
      await this.sendReply(ctx.roomId, ctx.eventId, "LLM features are not configured.");
      return;
    }

    const args = this.getArgs(ctx.body, "howami");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;
    const isSelf = targetUser === ctx.sender;

    const profile = this.gatherProfile(targetUser, ctx.roomId);
    if (!profile || profile.totalMessages === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, `No data found for ${targetUser}. They need to chat more.`);
      return;
    }

    try {
      const roast = await this.generateRoast(targetUser, profile, isSelf);
      if (roast) {
        await this.sendMessage(ctx.roomId, roast);
      } else {
        await this.sendReply(ctx.roomId, ctx.eventId, "The roast machine broke. Try again later.");
      }
    } catch (err) {
      logger.error(`howami roast generation failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "The roast machine broke. Try again later.");
    }
  }

  private gatherProfile(userId: string, roomId: string): UserProfile | null {
    const db = getDb();

    // user_stats
    const stats = db
      .prepare(`SELECT * FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as any;
    if (!stats) return null;

    // users (xp/level/rep)
    const user = db
      .prepare(`SELECT xp, level, rep FROM users WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { xp: number; level: number; rep: number } | undefined;

    // potty_mouth
    const potty = db
      .prepare(`SELECT total, severity_1, severity_2, severity_3 FROM potty_mouth WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { total: number; severity_1: number; severity_2: number; severity_3: number } | undefined;

    // insult_log
    const insults = db
      .prepare(`SELECT total_thrown, direct_thrown, indirect_thrown, times_targeted FROM insult_log WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { total_thrown: number; direct_thrown: number; indirect_thrown: number; times_targeted: number } | undefined;

    // trivia
    const trivia = db
      .prepare(`SELECT total_correct, total_points, best_streak, fastest_ms FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { total_correct: number; total_points: number; best_streak: number; fastest_ms: number | null } | undefined;

    // achievements
    const achievementRows = db
      .prepare(`SELECT achievement_key FROM achievements WHERE user_id = ? AND room_id = ?`)
      .all(userId, roomId) as { achievement_key: string }[];

    // sentiment breakdown from aggregate table
    const sentimentRow = db
      .prepare(`SELECT * FROM sentiment_stats WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { neutral: number; happy: number; sad: number; angry: number; excited: number; funny: number; love: number; scared: number; total_classified: number } | undefined;
    const sentiments: Record<string, number> = {};
    if (sentimentRow) {
      for (const s of ["neutral", "happy", "sad", "angry", "excited", "funny", "love", "scared"] as const) {
        if (sentimentRow[s] > 0) sentiments[s] = sentimentRow[s];
      }
    }

    // wotd
    const wotdRow = db
      .prepare(`SELECT COUNT(*) as attempts, COALESCE(SUM(CASE WHEN xp_awarded > 0 THEN 1 ELSE 0 END), 0) as correct FROM wotd_usage WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { attempts: number; correct: number };

    // first seen
    const firstSeen = db
      .prepare(`SELECT MIN(date) as first_date FROM daily_activity WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as { first_date: string | null };

    // hourly/daily distributions
    const hourly: Record<string, number> = JSON.parse(stats.hourly_distribution || "{}");
    const daily: Record<string, number> = JSON.parse(stats.daily_distribution || "{}");

    // peak hour
    let peakHour: number | null = null;
    let peakHourCount = 0;
    for (const [h, count] of Object.entries(hourly)) {
      if (count > peakHourCount) {
        peakHour = parseInt(h);
        peakHourCount = count;
      }
    }

    // peak day
    let peakDay: string | null = null;
    let peakDayCount = 0;
    for (const [d, count] of Object.entries(daily)) {
      if (count > peakDayCount) {
        peakDay = DAY_NAMES[parseInt(d)] ?? null;
        peakDayCount = count;
      }
    }

    return {
      totalMessages: stats.total_messages,
      totalWords: stats.total_words,
      totalQuestions: stats.total_questions,
      totalExclamations: stats.total_exclamations,
      totalEmojis: stats.total_emojis,
      totalLinks: stats.total_links,
      totalImages: stats.total_images,
      avgWordsPerMessage: stats.avg_words_per_message,
      longestMessage: stats.longest_message,
      currentStreak: stats.current_streak,
      longestStreak: stats.longest_streak,
      hourlyDistribution: hourly,
      dailyDistribution: daily,
      xp: user?.xp ?? 0,
      level: user?.level ?? 1,
      rep: user?.rep ?? 0,
      profanityTotal: potty?.total ?? 0,
      profanitySeverity: {
        mild: potty?.severity_1 ?? 0,
        moderate: potty?.severity_2 ?? 0,
        scorched: potty?.severity_3 ?? 0,
      },
      insultsThrown: insults?.total_thrown ?? 0,
      insultsDirect: insults?.direct_thrown ?? 0,
      insultsIndirect: insults?.indirect_thrown ?? 0,
      timesTargeted: insults?.times_targeted ?? 0,
      triviaCorrect: trivia?.total_correct ?? 0,
      triviaPoints: trivia?.total_points ?? 0,
      triviaStreak: trivia?.best_streak ?? 0,
      triviaFastestMs: trivia?.fastest_ms ?? null,
      achievements: achievementRows.map((a) => a.achievement_key),
      totalAchievements: achievementRows.length,
      sentiments,
      wotdCorrect: wotdRow?.correct ?? 0,
      wotdAttempts: wotdRow?.attempts ?? 0,
      peakHour,
      peakDay,
      firstSeenDate: firstSeen?.first_date ?? null,
    };
  }

  private buildStatsSummary(userId: string, p: UserProfile): string {
    const lines: string[] = [];

    lines.push(`User: ${userId}`);
    if (p.firstSeenDate) lines.push(`First seen: ${p.firstSeenDate}`);
    lines.push(`Level ${p.level} | ${p.xp} XP | ${p.rep} reputation`);
    lines.push(`Messages: ${p.totalMessages} | Words: ${p.totalWords} | Avg words/msg: ${p.avgWordsPerMessage.toFixed(1)}`);

    const questionPct = p.totalMessages > 0 ? ((p.totalQuestions / p.totalMessages) * 100).toFixed(1) : "0";
    const exclamationPct = p.totalMessages > 0 ? ((p.totalExclamations / p.totalMessages) * 100).toFixed(1) : "0";
    lines.push(`Questions: ${p.totalQuestions} (${questionPct}% of messages) | Exclamations: ${p.totalExclamations} (${exclamationPct}%)`);

    lines.push(`Emojis: ${p.totalEmojis} | Links: ${p.totalLinks} | Images: ${p.totalImages}`);
    lines.push(`Longest message: ${p.longestMessage} chars`);
    lines.push(`Activity streak: ${p.currentStreak}d current, ${p.longestStreak}d record`);

    if (p.peakHour !== null) lines.push(`Most active hour: ${p.peakHour}:00 UTC`);
    if (p.peakDay) lines.push(`Most active day: ${p.peakDay}`);

    if (p.profanityTotal > 0) {
      lines.push(`Profanity: ${p.profanityTotal} total (mild: ${p.profanitySeverity.mild}, moderate: ${p.profanitySeverity.moderate}, scorched: ${p.profanitySeverity.scorched})`);
    }

    if (p.insultsThrown > 0 || p.timesTargeted > 0) {
      lines.push(`Insults thrown: ${p.insultsThrown} (direct: ${p.insultsDirect}, indirect: ${p.insultsIndirect}) | Times targeted: ${p.timesTargeted}`);
    }

    if (p.triviaCorrect > 0) {
      const fastestStr = p.triviaFastestMs != null ? `${(p.triviaFastestMs / 1000).toFixed(2)}s` : "N/A";
      lines.push(`Trivia: ${p.triviaCorrect} correct, ${p.triviaPoints} pts, best streak: ${p.triviaStreak}, fastest: ${fastestStr}`);
    }

    if (p.wotdAttempts > 0) {
      lines.push(`WOTD: ${p.wotdCorrect}/${p.wotdAttempts} correct attempts`);
    }

    // Sentiment breakdown
    const totalClassified = Object.values(p.sentiments).reduce((a, b) => a + b, 0);
    if (totalClassified > 0) {
      const topSentiments = Object.entries(p.sentiments)
        .filter(([s]) => s !== "neutral")
        .sort((a, b) => b[1] - a[1])
        .slice(0, 3)
        .map(([s, c]) => `${s}: ${c}`);
      if (topSentiments.length > 0) {
        lines.push(`Top sentiments: ${topSentiments.join(", ")} (out of ${totalClassified} classified)`);
      }
    }

    if (p.achievements.length > 0) {
      lines.push(`Achievements (${p.totalAchievements}): ${p.achievements.join(", ")}`);
    }

    return lines.join("\n");
  }

  private async generateRoast(userId: string, profile: UserProfile, isSelf: boolean): Promise<string | null> {
    const statsSummary = this.buildStatsSummary(userId, profile);
    const target = isSelf ? "the user themselves (they asked for it)" : `${userId} (someone else asked about them)`;

    const prompt = `You are Freebee, a snarky but affectionate chat bot in a private friend group. Someone just used !howami, and you need to roast ${target} based on their ACTUAL stats below.

Rules:
- Write a single paragraph roast, 2-4 sentences max
- Be witty and specific — reference their actual numbers and patterns
- Tone: playful roast between friends, not mean-spirited. Think comedy roast, not bullying
- Find the funny angle in their data (night owl? question machine? potty mouth? lurker? emoji addict?)
- If they have notable achievements, work those in
- If their stats reveal an obvious personality type, call it out
- Do NOT use bullet points, headers, or formatting. Just flowing text
- Do NOT start with "Well," or "Oh," or greetings. Jump straight into the roast
- Keep it under 500 characters

Stats:
${statsSummary}

Write the roast now. Raw text only, no quotes or markdown.`;

    const res = await fetch(`${OLLAMA_HOST}/api/generate`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: OLLAMA_MODEL,
        prompt,
        stream: false,
        options: { num_ctx: OLLAMA_NUM_CTX, temperature: 0.9 },
      }),
      signal: AbortSignal.timeout(OLLAMA_TIMEOUT_MS),
    });

    if (!res.ok) {
      logger.warn(`Ollama API returned ${res.status} for howami`);
      return null;
    }

    const data = (await res.json()) as { response: string };
    let roast = data.response?.trim();
    if (!roast) return null;

    // Clean up common LLM quirks
    roast = roast.replace(/^["']|["']$/g, ""); // strip wrapping quotes
    roast = roast.replace(/^#+\s*/gm, ""); // strip markdown headers
    roast = roast.replace(/\*\*/g, ""); // strip bold markers

    return roast;
  }
}
