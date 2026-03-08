import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

const TRIVIA_TIMEOUT_SECONDS = parseInt(process.env.TRIVIA_TIMEOUT_SECONDS ?? "20", 10);
const FULL_POINTS = 100;
const FULL_POINTS_WINDOW_MS = 3000;

interface OpenTDBQuestion {
  category: string;
  type: string;
  difficulty: string;
  question: string;
  correct_answer: string;
  incorrect_answers: string[];
}

interface ActiveQuestion {
  sessionId: number;
  answer: string;
  answerIndex: number;
  options: string[];
  timeout: ReturnType<typeof setTimeout>;
  askedAt: number;
  startedBy: string;
  threadEventId: string;
}

const CATEGORY_MAP: Record<number, string> = {
  9: "General Knowledge",
  10: "Books",
  11: "Film",
  12: "Music",
  13: "Musicals & Theatre",
  14: "Television",
  15: "Video Games",
  16: "Board Games",
  17: "Science & Nature",
  18: "Computers",
  19: "Mathematics",
  20: "Mythology",
  21: "Sports",
  22: "Geography",
  23: "History",
  24: "Politics",
  25: "Art",
  26: "Celebrities",
  27: "Animals",
  28: "Vehicles",
  29: "Comics",
  30: "Gadgets",
  31: "Anime & Manga",
  32: "Cartoons",
};

function decodeHtml(html: string): string {
  return html
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#039;/g, "'");
}

function shuffleArray<T>(arr: T[]): T[] {
  const shuffled = [...arr];
  for (let i = shuffled.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
  }
  return shuffled;
}

function calculatePoints(elapsedMs: number): number {
  if (elapsedMs <= FULL_POINTS_WINDOW_MS) return FULL_POINTS;
  const timeoutMs = TRIVIA_TIMEOUT_SECONDS * 1000;
  if (elapsedMs >= timeoutMs) return 0;
  const decayRange = timeoutMs - FULL_POINTS_WINDOW_MS;
  const elapsed = elapsedMs - FULL_POINTS_WINDOW_MS;
  return Math.round(FULL_POINTS * (1 - elapsed / decayRange));
}

function fuzzyMatchCategory(input: string): number | null {
  const lower = input.toLowerCase();
  for (const [id, name] of Object.entries(CATEGORY_MAP)) {
    if (name.toLowerCase().includes(lower) || lower.includes(name.toLowerCase())) {
      return parseInt(id);
    }
  }
  return null;
}

export class TriviaPlugin extends Plugin {
  private xpPlugin: XpPlugin;
  private questionCache = new Map<string, OpenTDBQuestion[]>(); // cacheKey -> questions
  private activeQuestions = new Map<string, ActiveQuestion>();
  private triviaThreads = new Map<string, { eventId: string; categoryId?: number; difficulty?: string }>(); // roomId -> thread info

  constructor(client: IMatrixClient, xpPlugin: XpPlugin) {
    super(client);
    this.xpPlugin = xpPlugin;
    this.initDb();
  }

  get name() {
    return "trivia";
  }

  get commands(): CommandDef[] {
    return [
      { name: "trivia", description: "Start a trivia question", usage: "!trivia [category] [easy|medium|hard]" },
      { name: "trivia stop", description: "Cancel the active question" },
      { name: "trivia scores", description: "Trivia leaderboard", usage: "!trivia scores [@user|month]" },
      { name: "trivia categories", description: "List available trivia categories" },
      { name: "trivia fastest", description: "Top 10 fastest correct answers" },
    ];
  }

  private initDb(): void {
    const db = getDb();
    db.exec(`
      CREATE TABLE IF NOT EXISTS trivia_sessions (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        room_id TEXT,
        question TEXT,
        answer TEXT,
        category TEXT,
        difficulty TEXT,
        asked_at INTEGER DEFAULT (unixepoch()),
        answered_by TEXT,
        answered_at INTEGER,
        correct INTEGER DEFAULT 0
      )
    `);
    db.exec(`
      CREATE TABLE IF NOT EXISTS trivia_scores (
        user_id TEXT,
        room_id TEXT,
        total_correct INTEGER DEFAULT 0,
        total_points INTEGER DEFAULT 0,
        total_answered INTEGER DEFAULT 0,
        current_streak INTEGER DEFAULT 0,
        best_streak INTEGER DEFAULT 0,
        fastest_ms INTEGER,
        PRIMARY KEY(user_id, room_id)
      )
    `);
  }

  private cacheKey(roomId: string, categoryId?: number, difficulty?: string): string {
    return `${roomId}:${categoryId ?? "any"}:${difficulty ?? "any"}`;
  }

  private async fetchQuestions(roomId: string, categoryId?: number, difficulty?: string): Promise<void> {
    const key = this.cacheKey(roomId, categoryId, difficulty);
    try {
      let url = "https://opentdb.com/api.php?amount=50&type=multiple";
      if (categoryId) url += `&category=${categoryId}`;
      if (difficulty) url += `&difficulty=${difficulty}`;

      const res = await fetch(url);
      if (!res.ok) {
        logger.error(`OpenTDB API returned ${res.status}`);
        return;
      }

      const data = (await res.json()) as { response_code: number; results: OpenTDBQuestion[] };
      if (data.response_code !== 0 || !data.results.length) {
        // Try boolean type as fallback
        let boolUrl = "https://opentdb.com/api.php?amount=50&type=boolean";
        if (categoryId) boolUrl += `&category=${categoryId}`;
        if (difficulty) boolUrl += `&difficulty=${difficulty}`;

        const boolRes = await fetch(boolUrl);
        if (boolRes.ok) {
          const boolData = (await boolRes.json()) as { response_code: number; results: OpenTDBQuestion[] };
          if (boolData.response_code === 0) {
            const cache = this.questionCache.get(key) ?? [];
            cache.push(...boolData.results);
            this.questionCache.set(key, cache);
          }
        }
        return;
      }

      const cache = this.questionCache.get(key) ?? [];
      cache.push(...data.results);
      this.questionCache.set(key, cache);
    } catch (err) {
      logger.error(`Failed to fetch trivia questions: ${err}`);
    }
  }

  private getMessageThreadId(ctx: MessageContext): string | undefined {
    const relatesTo = ctx.event.content?.["m.relates_to"];
    if (relatesTo?.rel_type === "m.thread") {
      return relatesTo.event_id;
    }
    return undefined;
  }

  private async sendThreadMessage(roomId: string, threadEventId: string, text: string): Promise<string> {
    return await this.client.sendMessage(roomId, {
      msgtype: "m.text",
      body: text,
      "m.relates_to": {
        rel_type: "m.thread",
        event_id: threadEventId,
      },
    });
  }

  private async sendThreadReply(roomId: string, threadEventId: string, replyToEventId: string, text: string): Promise<string> {
    return await this.client.sendMessage(roomId, {
      msgtype: "m.text",
      body: text,
      "m.relates_to": {
        rel_type: "m.thread",
        event_id: threadEventId,
        "is_falling_back": false,
        "m.in_reply_to": { event_id: replyToEventId },
      },
    });
  }

  private async getOrCreateThread(roomId: string, categoryId?: number, difficulty?: string): Promise<string> {
    const existing = this.triviaThreads.get(roomId);
    // Reuse existing thread if category matches (or no category specified)
    if (existing) {
      const categoryChanged = categoryId !== undefined && categoryId !== existing.categoryId;
      if (!categoryChanged) return existing.eventId;
    }

    const catName = categoryId ? CATEGORY_MAP[categoryId] : null;
    const label = catName ? `Trivia Time: ${catName}!` : "Trivia Time!";
    const threadRootId = await this.sendMessage(roomId, `${label} Answer trivia questions in this thread.`);
    this.triviaThreads.set(roomId, { eventId: threadRootId, categoryId, difficulty });
    return threadRootId;
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    const inThreadId = this.getMessageThreadId(ctx);
    const roomThread = this.triviaThreads.get(ctx.roomId);
    const roomThreadId = roomThread?.eventId;
    const isInTriviaThread = !!(inThreadId && inThreadId === roomThreadId);
    const active = this.activeQuestions.get(ctx.roomId);

    // Check for answer attempts — only accept from within the trivia thread
    if (active && isInTriviaThread) {
      const trimmed = ctx.body.trim().toLowerCase();
      const isMultipleChoice = active.options.length === 4;
      const validLetters = isMultipleChoice ? ["a", "b", "c", "d"] : ["true", "false"];

      if (validLetters.includes(trimmed)) {
        await this.handleAnswer(ctx, active, trimmed);
        return;
      }
    }

    if (!this.isCommand(ctx.body, "trivia")) return;

    const args = this.getArgs(ctx.body, "trivia").trim();

    // If a thread exists and the user is NOT in it, redirect them
    // Exception: allow starting a new question from the main room (with optional category/difficulty)
    const threadOnlySubcommands = ["stop", "scores", "fastest"];
    const isThreadOnly = threadOnlySubcommands.some((sc) => args === sc || args.startsWith(sc + " "));
    if (roomThreadId && !isInTriviaThread) {
      if (isThreadOnly) {
        await this.sendReply(ctx.roomId, ctx.eventId, "Please use the trivia thread for trivia commands and answers.");
        return;
      }
      // Allow categories and starting new questions from the main room
    }

    if (args === "stop") {
      await this.handleStop(ctx);
    } else if (args.startsWith("scores")) {
      await this.handleScores(ctx, args.slice(6).trim());
    } else if (args === "categories") {
      await this.handleCategories(ctx);
    } else if (args === "fastest") {
      await this.handleFastest(ctx);
    } else {
      await this.handleTrivia(ctx, args);
    }
  }

  private async handleTrivia(ctx: MessageContext, args: string): Promise<void> {
    if (this.activeQuestions.has(ctx.roomId)) {
      const thread = this.triviaThreads.get(ctx.roomId);
      if (thread) {
        await this.sendThreadReply(ctx.roomId, thread.eventId, ctx.eventId, "A question is already active! Answer it first.");
      } else {
        await this.sendReply(ctx.roomId, ctx.eventId, "A question is already active! Answer it first.");
      }
      return;
    }

    // Parse category and difficulty from args
    let categoryId: number | undefined;
    let difficulty: string | undefined;
    const difficulties = ["easy", "medium", "hard"];

    if (args) {
      const parts = args.split(/\s+/);
      const diffPart = parts.find((p) => difficulties.includes(p.toLowerCase()));
      if (diffPart) {
        difficulty = diffPart.toLowerCase();
        parts.splice(parts.indexOf(diffPart), 1);
      }
      if (parts.length > 0) {
        const catInput = parts.join(" ");
        const matched = fuzzyMatchCategory(catInput);
        if (matched) {
          categoryId = matched;
        }
      }
    }

    // If no category/difficulty specified and we're in an existing thread, inherit its settings
    const existingThread = this.triviaThreads.get(ctx.roomId);
    if (existingThread) {
      if (categoryId === undefined) categoryId = existingThread.categoryId;
      if (difficulty === undefined) difficulty = existingThread.difficulty;
    }

    // Fetch questions if cache is empty for this room + category + difficulty
    const key = this.cacheKey(ctx.roomId, categoryId, difficulty);
    const roomCache = this.questionCache.get(key);
    if (!roomCache || roomCache.length === 0) {
      await this.fetchQuestions(ctx.roomId, categoryId, difficulty);
    }

    const questions = this.questionCache.get(key);
    if (!questions || questions.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to fetch trivia questions. Try again later.");
      return;
    }

    // Create or get the trivia thread
    const threadEventId = await this.getOrCreateThread(ctx.roomId, categoryId, difficulty);

    const question = questions.shift()!;
    const decodedQuestion = decodeHtml(question.question);
    const correctAnswer = decodeHtml(question.correct_answer);
    const incorrectAnswers = question.incorrect_answers.map(decodeHtml);

    const isBoolean = question.type === "boolean";
    let options: string[];
    let answerIndex: number;
    let answerLetter: string;

    if (isBoolean) {
      options = ["True", "False"];
      answerIndex = correctAnswer === "True" ? 0 : 1;
      answerLetter = correctAnswer.toLowerCase();
    } else {
      options = shuffleArray([correctAnswer, ...incorrectAnswers]);
      answerIndex = options.indexOf(correctAnswer);
      answerLetter = ["a", "b", "c", "d"][answerIndex];
    }

    const db = getDb();
    const now = Math.floor(Date.now() / 1000);
    const result = db.prepare(`
      INSERT INTO trivia_sessions (room_id, question, answer, category, difficulty, asked_at)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(ctx.roomId, decodedQuestion, correctAnswer, question.category, question.difficulty, now);

    const sessionId = Number(result.lastInsertRowid);

    const timeout = setTimeout(() => {
      this.handleTimeout(ctx.roomId, sessionId, correctAnswer);
    }, TRIVIA_TIMEOUT_SECONDS * 1000);

    this.activeQuestions.set(ctx.roomId, {
      sessionId,
      answer: answerLetter,
      answerIndex,
      options,
      timeout,
      askedAt: Date.now(),
      startedBy: ctx.sender,
      threadEventId,
    });

    const diffLabel = question.difficulty.charAt(0).toUpperCase() + question.difficulty.slice(1);
    const lines = [`[${question.category}] (${diffLabel})`, decodedQuestion, ""];

    if (isBoolean) {
      lines.push("True or False?");
    } else {
      const labels = ["A", "B", "C", "D"];
      for (let i = 0; i < options.length; i++) {
        lines.push(`${labels[i]}) ${options[i]}`);
      }
    }

    lines.push("", `You have ${TRIVIA_TIMEOUT_SECONDS} seconds to answer!`);
    await this.sendThreadMessage(ctx.roomId, threadEventId, lines.join("\n"));
  }

  private async handleAnswer(ctx: MessageContext, active: ActiveQuestion, answer: string): Promise<void> {
    const isBoolean = active.options.length === 2;
    let isCorrect: boolean;

    if (isBoolean) {
      isCorrect = answer === active.answer;
    } else {
      const letterIndex = ["a", "b", "c", "d"].indexOf(answer);
      isCorrect = letterIndex === active.answerIndex;
    }

    const elapsedMs = Date.now() - active.askedAt;
    const db = getDb();

    if (isCorrect) {
      clearTimeout(active.timeout);
      this.activeQuestions.delete(ctx.roomId);

      const points = calculatePoints(elapsedMs);
      const elapsedSec = (elapsedMs / 1000).toFixed(2);

      // Update session
      db.prepare(`
        UPDATE trivia_sessions SET answered_by = ?, answered_at = ?, correct = 1
        WHERE id = ?
      `).run(ctx.sender, Math.floor(Date.now() / 1000), active.sessionId);

      // Update scores
      db.prepare(`
        INSERT INTO trivia_scores (user_id, room_id, total_correct, total_points, total_answered, current_streak, best_streak, fastest_ms)
        VALUES (?, ?, 1, ?, 1, 1, 1, ?)
        ON CONFLICT(user_id, room_id) DO UPDATE SET
          total_correct = total_correct + 1,
          total_points = total_points + ?,
          total_answered = total_answered + 1,
          current_streak = current_streak + 1,
          best_streak = MAX(best_streak, current_streak + 1),
          fastest_ms = MIN(COALESCE(fastest_ms, ?), ?)
      `).run(ctx.sender, ctx.roomId, points, elapsedMs, points, elapsedMs, elapsedMs);

      // Grant XP based on points
      this.xpPlugin.grantXp(ctx.sender, ctx.roomId, points, "trivia correct answer");

      // React with checkmark
      this.client.sendEvent(ctx.roomId, "m.reaction", {
        "m.relates_to": {
          rel_type: "m.annotation",
          event_id: ctx.eventId,
          key: "\u2705",
        },
      }).catch((err) => logger.error(`Failed to react: ${err}`));

      await this.sendThreadMessage(
        ctx.roomId,
        active.threadEventId,
        `Correct! ${ctx.sender} answered in ${elapsedSec}s for ${points} points!`
      );
    } else {
      // Update total_answered and reset streak
      db.prepare(`
        INSERT INTO trivia_scores (user_id, room_id, total_correct, total_points, total_answered, current_streak, best_streak)
        VALUES (?, ?, 0, 0, 1, 0, 0)
        ON CONFLICT(user_id, room_id) DO UPDATE SET
          total_answered = total_answered + 1,
          current_streak = 0
      `).run(ctx.sender, ctx.roomId);

      // React with X
      this.client.sendEvent(ctx.roomId, "m.reaction", {
        "m.relates_to": {
          rel_type: "m.annotation",
          event_id: ctx.eventId,
          key: "\u274C",
        },
      }).catch((err) => logger.error(`Failed to react: ${err}`));
    }
  }

  private handleTimeout(roomId: string, sessionId: number, correctAnswer: string): void {
    const active = this.activeQuestions.get(roomId);
    if (!active || active.sessionId !== sessionId) return;

    this.activeQuestions.delete(roomId);

    // Reset streaks for all participants who answered wrong
    const db = getDb();
    db.prepare(`
      UPDATE trivia_scores SET current_streak = 0 WHERE room_id = ?
    `).run(roomId);

    const threadEventId = active.threadEventId;
    this.sendThreadMessage(roomId, threadEventId, `Time's up! The correct answer was: ${correctAnswer}`).catch((err) =>
      logger.error(`Failed to send timeout message: ${err}`)
    );
  }

  private async handleStop(ctx: MessageContext): Promise<void> {
    const active = this.activeQuestions.get(ctx.roomId);
    const threadEventId = this.triviaThreads.get(ctx.roomId)?.eventId;
    if (!active) {
      if (threadEventId) {
        await this.sendThreadReply(ctx.roomId, threadEventId, ctx.eventId, "No active trivia question to stop.");
      } else {
        await this.sendReply(ctx.roomId, ctx.eventId, "No active trivia question to stop.");
      }
      return;
    }

    if (!this.isAdmin(ctx.sender) && ctx.sender !== active.startedBy) {
      await this.sendThreadReply(ctx.roomId, active.threadEventId, ctx.eventId, "Only an admin or the question starter can stop an active question.");
      return;
    }

    clearTimeout(active.timeout);
    this.activeQuestions.delete(ctx.roomId);
    await this.sendThreadMessage(ctx.roomId, active.threadEventId, "Trivia question cancelled.");
  }

  private async handleScores(ctx: MessageContext, args: string): Promise<void> {
    const db = getDb();
    const threadEventId = this.triviaThreads.get(ctx.roomId)!.eventId;

    if (args === "month") {
      // Current month scores
      const now = new Date();
      const monthStart = Math.floor(new Date(now.getFullYear(), now.getMonth(), 1).getTime() / 1000);

      const rows = db.prepare(`
        SELECT answered_by AS user_id,
               COUNT(*) AS total_correct,
               SUM(CASE WHEN correct = 1 THEN 1 ELSE 0 END) AS wins
        FROM trivia_sessions
        WHERE room_id = ? AND correct = 1 AND asked_at >= ?
        GROUP BY answered_by
        ORDER BY wins DESC
        LIMIT 10
      `).all(ctx.roomId, monthStart) as { user_id: string; total_correct: number; wins: number }[];

      if (rows.length === 0) {
        await this.sendThreadReply(ctx.roomId, threadEventId, ctx.eventId, "No trivia scores this month yet.");
        return;
      }

      const lines = rows.map((r, i) => `${i + 1}. ${r.user_id} — ${r.wins} correct`);
      await this.sendThreadMessage(ctx.roomId, threadEventId, `Trivia Leaderboard (This Month):\n${lines.join("\n")}`);
      return;
    }

    if (args.startsWith("@")) {
      // Individual user scores
      const targetUser = args.split(/\s/)[0];
      const row = db
        .prepare(`SELECT * FROM trivia_scores WHERE user_id = ? AND room_id = ?`)
        .get(targetUser, ctx.roomId) as {
          total_correct: number;
          total_points: number;
          total_answered: number;
          current_streak: number;
          best_streak: number;
          fastest_ms: number | null;
        } | undefined;

      if (!row) {
        await this.sendThreadReply(ctx.roomId, threadEventId, ctx.eventId, `No trivia data found for ${targetUser}.`);
        return;
      }

      const fastestSec = row.fastest_ms != null ? (row.fastest_ms / 1000).toFixed(2) + "s" : "N/A";
      const accuracy = row.total_answered > 0
        ? ((row.total_correct / row.total_answered) * 100).toFixed(1) + "%"
        : "N/A";

      await this.sendThreadMessage(
        ctx.roomId,
        threadEventId,
        `Trivia Stats for ${targetUser}:\n` +
          `Correct: ${row.total_correct}/${row.total_answered} (${accuracy})\n` +
          `Points: ${row.total_points}\n` +
          `Streak: ${row.current_streak} (Best: ${row.best_streak})\n` +
          `Fastest: ${fastestSec}`
      );
      return;
    }

    // General leaderboard
    const rows = db
      .prepare(
        `SELECT user_id, total_correct, total_points, best_streak
         FROM trivia_scores WHERE room_id = ?
         ORDER BY total_points DESC LIMIT 10`
      )
      .all(ctx.roomId) as { user_id: string; total_correct: number; total_points: number; best_streak: number }[];

    if (rows.length === 0) {
      await this.sendThreadReply(ctx.roomId, threadEventId, ctx.eventId, "No trivia scores yet.");
      return;
    }

    const lines = rows.map(
      (r, i) => `${i + 1}. ${r.user_id} — ${r.total_points} pts (${r.total_correct} correct, best streak: ${r.best_streak})`
    );
    await this.sendThreadMessage(ctx.roomId, threadEventId, `Trivia Leaderboard:\n${lines.join("\n")}`);
  }

  private async handleCategories(ctx: MessageContext): Promise<void> {
    const threadEventId = this.triviaThreads.get(ctx.roomId)!.eventId;
    const lines = ["Available Trivia Categories:"];
    for (const [id, name] of Object.entries(CATEGORY_MAP)) {
      lines.push(`  ${id}. ${name}`);
    }
    await this.sendThreadMessage(ctx.roomId, threadEventId, lines.join("\n"));
  }

  private async handleFastest(ctx: MessageContext): Promise<void> {
    const threadEventId = this.triviaThreads.get(ctx.roomId)!.eventId;
    const db = getDb();
    const rows = db
      .prepare(
        `SELECT user_id, fastest_ms
         FROM trivia_scores WHERE room_id = ? AND fastest_ms IS NOT NULL
         ORDER BY fastest_ms ASC LIMIT 10`
      )
      .all(ctx.roomId) as { user_id: string; fastest_ms: number }[];

    if (rows.length === 0) {
      await this.sendThreadReply(ctx.roomId, threadEventId, ctx.eventId, "No fastest answer data yet.");
      return;
    }

    const lines = rows.map(
      (r, i) => `${i + 1}. ${r.user_id} — ${(r.fastest_ms / 1000).toFixed(2)}s`
    );
    await this.sendThreadMessage(ctx.roomId, threadEventId, `Fastest Trivia Answers:\n${lines.join("\n")}`);
  }
}
