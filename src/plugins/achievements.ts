import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { getDb } from "../db";
import logger from "../utils/logger";

interface AchievementDef {
  key: string;
  name: string;
  description: string;
  check: (stats: any, extras: AchievementExtras) => boolean;
}

interface AchievementExtras {
  userId: string;
  roomId: string;
}

const ACHIEVEMENTS: AchievementDef[] = [
  {
    key: "encyclopedist",
    name: "Encyclopedist",
    description: "Write 100,000 total words",
    check: (s) => s.total_words >= 100_000,
  },
  {
    key: "linkdump",
    name: "Linkdump",
    description: "Post 500 links",
    check: (s) => s.total_links >= 500,
  },
  {
    key: "night_shift",
    name: "Night Shift",
    description: "Send 100 messages between 00:00–04:00",
    check: (s) => {
      const hourly: Record<string, number> = JSON.parse(s.hourly_distribution || "{}");
      const nightMsgs = [0, 1, 2, 3].reduce((sum, h) => sum + (hourly[h] ?? 0), 0);
      return nightMsgs >= 100;
    },
  },
  {
    key: "riddler",
    name: "Riddler",
    description: "Ask 500 questions",
    check: (s) => s.total_questions >= 500,
  },
  {
    key: "show_dont_tell",
    name: "Show Don't Tell",
    description: "Post 200 images",
    check: (s) => s.total_images >= 200,
  },
  {
    key: "hemingway",
    name: "Hemingway",
    description: "Send 1,000 messages averaging under 5 words",
    check: (s) => s.total_messages >= 1000 && s.avg_words_per_message < 5,
  },
  {
    key: "tolstoy",
    name: "Tolstoy",
    description: "Send 1,000 messages averaging over 50 words",
    check: (s) => s.total_messages >= 1000 && s.avg_words_per_message > 50,
  },
  {
    key: "logophile",
    name: "Logophile",
    description: "Use a word over 15 characters",
    // This is checked at message time via parseMessage().hasLongWord
    // but we also check the flag here from a stored marker
    check: () => false, // handled specially below
  },
  {
    key: "omnipresent",
    name: "Omnipresent",
    description: "Active 30 unique days in a single calendar month",
    check: (_s, extras) => {
      const db = getDb();
      // Check current month
      const now = new Date();
      const monthStart = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-01`;
      const nextMonth = new Date(now.getFullYear(), now.getMonth() + 1, 1);
      const monthEnd = nextMonth.toISOString().slice(0, 10);
      const row = db
        .prepare(`SELECT COUNT(DISTINCT date) as days FROM daily_activity WHERE user_id = ? AND room_id = ? AND date >= ? AND date < ?`)
        .get(extras.userId, extras.roomId, monthStart, monthEnd) as { days: number };
      return row.days >= 30;
    },
  },
  {
    key: "early_bird_legend",
    name: "Early Bird Legend",
    description: "Held First! for 30 days total",
    check: (_s, extras) => {
      const db = getDb();
      const row = db
        .prepare(`SELECT COUNT(*) as firsts FROM daily_first WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { firsts: number };
      return row.firsts >= 30;
    },
  },
  {
    key: "streak_week",
    name: "Streak Week",
    description: "Achieve a 7-day streak",
    check: (s) => s.longest_streak >= 7,
  },
  {
    key: "streak_month",
    name: "Streak Month",
    description: "Achieve a 30-day streak",
    check: (s) => s.longest_streak >= 30,
  },
  {
    key: "beloved",
    name: "Beloved",
    description: "Earn 50 reputation points",
    check: (_s, extras) => {
      const db = getDb();
      const row = db
        .prepare(`SELECT rep FROM users WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { rep: number } | undefined;
      return (row?.rep ?? 0) >= 50;
    },
  },
  {
    key: "gamer",
    name: "Gamer",
    description: "Have 10 items in your backlog",
    check: (_s, extras) => {
      const db = getDb();
      const row = db
        .prepare(`SELECT COUNT(*) as count FROM backlog WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { count: number };
      return row.count >= 10;
    },
  },
  {
    key: "completionist",
    name: "Completionist",
    description: "Complete 10 backlog items",
    check: (_s, extras) => {
      const db = getDb();
      const row = db
        .prepare(`SELECT COUNT(*) as count FROM backlog WHERE user_id = ? AND room_id = ? AND completed = 1`)
        .get(extras.userId, extras.roomId) as { count: number };
      return row.count >= 10;
    },
  },
  // Trivia
  {
    key: "trivia_first_blood",
    name: "First Blood",
    description: "First correct trivia answer",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT total_correct FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { total_correct: number } | undefined;
      return (row?.total_correct ?? 0) >= 1;
    },
  },
  {
    key: "trivia_century",
    name: "The Scholar",
    description: "100 correct trivia answers",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT total_correct FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { total_correct: number } | undefined;
      return (row?.total_correct ?? 0) >= 100;
    },
  },
  {
    key: "trivia_speed_demon",
    name: "Speed Demon",
    description: "Correct trivia answer in under 2 seconds",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT fastest_ms FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { fastest_ms: number | null } | undefined;
      return (row?.fastest_ms ?? Infinity) < 2000;
    },
  },
  {
    key: "trivia_on_a_roll",
    name: "On a Roll",
    description: "10 correct trivia answers in a row",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT best_streak FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { best_streak: number } | undefined;
      return (row?.best_streak ?? 0) >= 10;
    },
  },
  // Birthday
  {
    key: "birthday_celebrated",
    name: "Birthday Bee",
    description: "Had your birthday celebrated by Freebee",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT user_id FROM birthday_fired WHERE user_id = ?`)
        .get(extras.userId) as any;
      return !!row;
    },
  },
  // LLM passive (only earn if LLM is enabled)
  {
    key: "wotd_scholar",
    name: "Word Nerd",
    description: "Used the WOTD correctly 10 times",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM wotd_usage WHERE user_id = ? AND room_id = ? AND xp_awarded > 0`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 10;
    },
  },
  {
    key: "wotd_cheater",
    name: "Nice Try",
    description: "Attempted to game the WOTD 5 times",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM wotd_usage WHERE user_id = ? AND room_id = ? AND xp_awarded = 0`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 5;
    },
  },
  {
    key: "potty_bronze",
    name: "Needs Soap",
    description: "50 profanity detections",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT total FROM potty_mouth WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { total: number } | undefined;
      return (row?.total ?? 0) >= 50;
    },
  },
  {
    key: "potty_gold",
    name: "Sailor Mouth",
    description: "500 profanity detections",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT total FROM potty_mouth WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { total: number } | undefined;
      return (row?.total ?? 0) >= 500;
    },
  },
  {
    key: "roaster",
    name: "The Roaster",
    description: "50 insults thrown",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT total_thrown FROM insult_log WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { total_thrown: number } | undefined;
      return (row?.total_thrown ?? 0) >= 50;
    },
  },
  {
    key: "punching_bag",
    name: "Punching Bag",
    description: "Targeted 50 times",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT times_targeted FROM insult_log WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { times_targeted: number } | undefined;
      return (row?.times_targeted ?? 0) >= 50;
    },
  },
  // Social / utility
  {
    key: "welcome_wagon",
    name: "Welcome Wagon",
    description: "First message ever in this room",
    check: () => false, // handled by WelcomePlugin directly
  },
  {
    key: "countdown_keeper",
    name: "Countdown Keeper",
    description: "5 active countdowns simultaneously",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM countdowns WHERE user_id = ? AND room_id = ? AND completed = 0`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 5;
    },
  },
  {
    key: "markov_victim",
    name: "Markov Victim",
    description: "Had someone run !markov on you",
    check: () => false, // handled by MarkovPlugin directly
  },
  {
    key: "stonks",
    name: "Stonks",
    description: "5 tickers on stock watchlist",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM stock_watchlist WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 5;
    },
  },
  {
    key: "weeaboo",
    name: "Certified Weeaboo",
    description: "10 anime on watchlist",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM anime_watchlist WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 10;
    },
  },
  {
    key: "cinephile",
    name: "Cinephile",
    description: "10 movies/TV shows on watchlist",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM movie_watchlist WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 10;
    },
  },
  {
    key: "concert_goer",
    name: "Concert Goer",
    description: "Watching 5 artists",
    check: (_s, extras) => {
      const db = getDb();
      const row = db.prepare(`SELECT COUNT(*) as c FROM concert_watchlist WHERE user_id = ? AND room_id = ?`)
        .get(extras.userId, extras.roomId) as { c: number };
      return row.c >= 5;
    },
  },
];

export class AchievementsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "achievements";
  }

  get commands(): CommandDef[] {
    return [
      { name: "achievements", description: "View unlocked achievements", usage: "!achievements [@user]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: evaluate achievements silently
    this.evaluateAchievements(ctx.sender, ctx.roomId, ctx.body);

    if (this.isCommand(ctx.body, "achievements")) {
      await this.handleAchievements(ctx);
    }
  }

  private evaluateAchievements(userId: string, roomId: string, messageBody: string): void {
    const db = getDb();
    const stats = db
      .prepare(`SELECT * FROM user_stats WHERE user_id = ? AND room_id = ?`)
      .get(userId, roomId) as any;

    if (!stats) return;

    const extras: AchievementExtras = { userId, roomId };

    for (const achievement of ACHIEVEMENTS) {
      // Skip if already unlocked
      const existing = db
        .prepare(`SELECT id FROM achievements WHERE user_id = ? AND room_id = ? AND achievement_key = ?`)
        .get(userId, roomId, achievement.key);
      if (existing) continue;

      let earned = false;

      // Special case: logophile checks message content directly
      if (achievement.key === "logophile") {
        const { parseMessage } = require("../utils/parser");
        const parsed = parseMessage(messageBody);
        earned = parsed.hasLongWord;
      } else {
        earned = achievement.check(stats, extras);
      }

      if (earned) {
        db.prepare(`INSERT OR IGNORE INTO achievements (user_id, room_id, achievement_key) VALUES (?, ?, ?)`).run(
          userId,
          roomId,
          achievement.key
        );
        logger.debug(`Achievement unlocked: ${userId} earned "${achievement.key}" in ${roomId}`);
      }
    }
  }

  private async handleAchievements(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "achievements");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const unlocked = db
      .prepare(`SELECT achievement_key, unlocked_at FROM achievements WHERE user_id = ? AND room_id = ? ORDER BY unlocked_at ASC`)
      .all(targetUser, ctx.roomId) as { achievement_key: string; unlocked_at: string }[];

    const unlockedKeys = new Set(unlocked.map((a) => a.achievement_key));

    const lines = [`Achievements for ${targetUser} (${unlocked.length}/${ACHIEVEMENTS.length}):`];

    for (const def of ACHIEVEMENTS) {
      if (unlockedKeys.has(def.key)) {
        lines.push(`  [x] ${def.name} — ${def.description}`);
      } else {
        lines.push(`  [ ] ${def.name} — ${def.description}`);
      }
    }

    await this.sendMessage(ctx.roomId, lines.join("\n"));
  }
}
