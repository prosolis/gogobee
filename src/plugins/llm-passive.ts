import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

const RAW_OLLAMA_HOST = process.env.OLLAMA_HOST ?? "";
const OLLAMA_HOST = RAW_OLLAMA_HOST && !RAW_OLLAMA_HOST.startsWith("http") ? `http://${RAW_OLLAMA_HOST}` : RAW_OLLAMA_HOST;
const OLLAMA_MODEL = process.env.OLLAMA_MODEL ?? "";
const LLM_ENABLED = OLLAMA_HOST !== "" && OLLAMA_MODEL !== "";
const NOT_CONFIGURED = "LLM features are not configured.";
const WOTD_XP = 50;
const REP_XP_BONUS = 5;
const REP_COOLDOWN_HOURS = 24;
const OLLAMA_TIMEOUT_MS = 30_000;
const MAX_QUEUE_SIZE = 50;
const OLLAMA_NUM_CTX = 16384;
const SAMPLE_RATE = parseFloat(process.env.LLM_SAMPLE_RATE ?? "0.15");

const PROFANITY_KEYWORDS = [
  "fuck", "shit", "damn", "hell", "ass", "bitch", "bastard", "crap",
  "dick", "piss", "cock", "cunt", "twat", "wank", "bollocks", "arse",
  "motherfucker", "bullshit", "horseshit", "goddamn", "dammit", "asshole",
  "shitty", "bitchy", "fucked", "fucking", "fucker", "dumbass", "jackass",
  "dipshit", "wtf", "stfu", "lmfao",
];

type Sentiment = "neutral" | "happy" | "sad" | "angry" | "excited" | "funny" | "love" | "scared";

interface ClassificationResult {
  profanity: boolean;
  profanity_severity: number;
  insult: boolean;
  insult_type: "direct" | "indirect" | null;
  insult_target: string | null;
  insult_severity: number;
  sentiment: Sentiment;
  gratitude: boolean;
  gratitude_toward: string | null;
  wotd_used: boolean;
  wotd_correct: boolean;
}

interface QueueItem {
  ctx: MessageContext;
  wotd: { word: string; definition: string | null; example: string | null; part_of_speech: string | null } | null;
}

const BACKOFF_INITIAL_MS = 5_000;
const BACKOFF_MAX_MS = 5 * 60 * 1000; // 5 minutes

export class LlmPassivePlugin extends Plugin {
  private xpPlugin: XpPlugin;
  private botUserId: string = "";
  private queue: QueueItem[] = [];
  private processing = false;
  private backoffMs = 0;
  private backoffUntil = 0;
  private consecutiveFailures = 0;

  constructor(client: IMatrixClient, xpPlugin: XpPlugin) {
    super(client);
    this.xpPlugin = xpPlugin;
    const uid = client.getUserId();
    if (typeof uid === "string") {
      this.botUserId = uid;
    } else if (uid) {
      (uid as Promise<string>).then((id: string) => { this.botUserId = id; }).catch(() => {});
    }
  }

  get name() {
    return "llm-passive";
  }

  get commands(): CommandDef[] {
    return [
      { name: "potty", description: "Potty mouth stats", usage: "!potty [@user]" },
      { name: "pottyboard", description: "Potty mouth leaderboard" },
      { name: "insults", description: "Insult stats", usage: "!insults [@user]" },
      { name: "insultboard", description: "Most prolific insulters leaderboard" },
      { name: "wotd attempts", description: "Who tried today's WOTD", usage: "!wotd attempts" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (!LLM_ENABLED) {
      // Still handle commands to return the not-configured message
      const cmds = ["potty", "pottyboard", "insults", "insultboard"];
      for (const cmd of cmds) {
        if (this.isCommand(ctx.body, cmd)) {
          await this.sendReply(ctx.roomId, ctx.eventId, NOT_CONFIGURED);
          return;
        }
      }
      if (this.isCommand(ctx.body, "wotd attempts")) {
        await this.sendReply(ctx.roomId, ctx.eventId, NOT_CONFIGURED);
        return;
      }
      return;
    }

    // Handle commands
    if (this.isCommand(ctx.body, "wotd attempts")) {
      await this.handleWotdAttempts(ctx);
      return;
    }
    if (this.isCommand(ctx.body, "pottyboard")) {
      await this.handlePottyboard(ctx);
      return;
    }
    if (this.isCommand(ctx.body, "potty")) {
      await this.handlePotty(ctx);
      return;
    }
    if (this.isCommand(ctx.body, "insultboard")) {
      await this.handleInsultboard(ctx);
      return;
    }
    if (this.isCommand(ctx.body, "insults")) {
      await this.handleInsults(ctx);
      return;
    }

    // Passive classification — pre-filter to avoid sending every message to Ollama
    // Always classify: keyword matches, @mentions, WOTD usage, non-ASCII text
    // Otherwise: sample a percentage of remaining messages to catch what keywords miss
    const bodyLower = ctx.body.toLowerCase();
    const words = bodyLower.split(/\s+/);

    const wotd = this.getTodaysWotd(ctx.roomId);

    const hasProfanity = words.some((w) => {
      const cleaned = w.replace(/[^a-z]/g, "");
      return PROFANITY_KEYWORDS.includes(cleaned);
    });
    const hasMention = ctx.body.includes("@");
    const hasWotd = wotd ? new RegExp(`\\b${wotd.word.toLowerCase().replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`).test(bodyLower) : false;
    const hasNonAscii = /[^\x00-\x7F]/.test(ctx.body);
    const hasThanks = /\b(?:thanks?|thank\s*you|thx|ty|tysm|tyvm|cheers|kudos|props)\b/i.test(ctx.body);

    if (!hasProfanity && !hasMention && !hasWotd && !hasNonAscii && !hasThanks) {
      // Random sample of remaining messages so the LLM can catch
      // profanity/insults in any language or phrasing the keyword list misses
      if (Math.random() >= SAMPLE_RATE) return;
    }

    // Enqueue for classification (drop if queue is full to avoid unbounded growth)
    if (this.queue.length >= MAX_QUEUE_SIZE) {
      logger.warn(`LLM classification queue full (${MAX_QUEUE_SIZE}), dropping message`);
      return;
    }
    this.queue.push({ ctx, wotd });
    this.processQueue();
  }

  private getTodaysWotd(roomId: string): { word: string; definition: string | null; example: string | null; part_of_speech: string | null } | null {
    try {
      const db = getDb();
      const today = new Date().toISOString().slice(0, 10);
      // Try today first, fall back to the most recent word (covers the gap before the new word is posted)
      let row = db
        .prepare(`SELECT word, definition, example, part_of_speech FROM wotd_log WHERE date = ? AND room_id = ? LIMIT 1`)
        .get(today, roomId) as { word: string; definition: string | null; example: string | null; part_of_speech: string | null } | undefined;
      if (!row) {
        row = db
          .prepare(`SELECT word, definition, example, part_of_speech FROM wotd_log WHERE room_id = ? ORDER BY date DESC LIMIT 1`)
          .get(roomId) as typeof row;
      }
      return row ?? null;
    } catch {
      return null;
    }
  }

  private async processQueue(): Promise<void> {
    if (this.processing) return;
    this.processing = true;

    try {
      while (this.queue.length > 0) {
        // If in backoff, wait then retry
        const now = Date.now();
        if (this.backoffUntil > now) {
          const wait = this.backoffUntil - now;
          logger.info(`LLM backoff: waiting ${Math.ceil(wait / 1000)}s before retry (${this.queue.length} queued)`);
          await new Promise((r) => setTimeout(r, wait));
        }

        const item = this.queue.shift()!;
        try {
          await this.classify(item);
          // Success — reset backoff
          if (this.consecutiveFailures > 0) {
            logger.info("LLM back online — resuming classification");
          }
          this.consecutiveFailures = 0;
          this.backoffMs = 0;
          this.backoffUntil = 0;
        } catch (err) {
          this.consecutiveFailures++;
          this.backoffMs = Math.min(
            this.backoffMs === 0 ? BACKOFF_INITIAL_MS : this.backoffMs * 2,
            BACKOFF_MAX_MS
          );
          this.backoffUntil = Date.now() + this.backoffMs;
          logger.error(`LLM classification failed (${this.consecutiveFailures}x, next retry in ${this.backoffMs / 1000}s): ${err}`);
        }
      }
    } finally {
      this.processing = false;
    }
  }

  private async classify(item: QueueItem): Promise<void> {
    const { ctx, wotd } = item;

    const wotdWord = wotd?.word ?? "none";
    const wotdDef = wotd?.definition ?? "N/A";
    const wotdPos = wotd?.part_of_speech ?? "N/A";
    const wotdExample = wotd?.example ?? "N/A";

    const prompt = `You are a passive classifier for a private progressive friend group's chat.
Affectionate trash talk between friends is normal here — do NOT classify casual banter, questions, or playful teasing as insults.
insult should ONLY be true for messages with clear hostile intent to demean or offend someone. Asking questions, using slang, or joking around is NOT an insult.
The bot's user ID is ${this.botUserId}. If someone directly insults the bot (e.g. "stupid bot", "you suck"), set insult_target to "${this.botUserId}".
Analyze the message and respond ONLY with valid JSON, no other text.

Today's word of the day: ${wotdWord}
Definition: ${wotdDef}
Part of speech: ${wotdPos}
Example: ${wotdExample}

Message author: ${ctx.sender}
Message: ${ctx.body}

{"profanity":boolean,"profanity_severity":0|1|2|3,"insult":boolean,"insult_type":"direct"|"indirect"|null,"insult_target":"@mxid or null","insult_severity":0|1|2|3,"sentiment":"neutral"|"happy"|"sad"|"angry"|"excited"|"funny"|"love"|"scared","gratitude":boolean,"gratitude_toward":"@mxid or null","wotd_used":boolean,"wotd_correct":boolean}

insult_target should be the @mxid of the person being insulted, or null if no specific target. If the bot itself is insulted, use "${this.botUserId}".
gratitude is TRUE for genuine thanks/appreciation ("thank you for helping", "thanks for the useful feedback", "ty that worked!"). Sarcastic thanks ("thanks for nothing", "gee thanks for the unhelpful advice") is NOT gratitude. When in doubt, lean toward TRUE if the message tone is positive. gratitude_toward should be the @mxid of the person being thanked, or null if no specific person is mentioned.
wotd_used is TRUE only when the word appears as a STANDALONE word in the message (not as a substring of another word). wotd_correct is TRUE only when the standalone word is used correctly per its definition and part of speech. Typing the word out of context, referencing it meta, or awkward forced insertion does NOT qualify.`;

    const res = await fetch(`${OLLAMA_HOST}/api/generate`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: OLLAMA_MODEL,
        prompt,
        stream: false,
        options: { num_ctx: OLLAMA_NUM_CTX },
      }),
      signal: AbortSignal.timeout(OLLAMA_TIMEOUT_MS),
    });

    if (!res.ok) {
      logger.warn(`Ollama API returned ${res.status}`);
      return;
    }

    const data = (await res.json()) as { response: string };
    const result = this.parseClassification(data.response);
    if (!result) {
      logger.warn(`Failed to parse Ollama response: ${data.response}`);
      return;
    }

    logger.info(`LLM classified "${ctx.body.substring(0, 50)}": profanity=${result.profanity}, insult=${result.insult}${result.insult ? ` (target=${result.insult_target}, type=${result.insult_type})` : ""}, sentiment=${result.sentiment}, gratitude=${result.gratitude}${result.gratitude ? ` (toward=${result.gratitude_toward})` : ""}, wotd=${result.wotd_used}`);
    this.storeResult(ctx, result);
  }

  /**
   * Attempt to parse the LLM response as a ClassificationResult.
   * Small models frequently produce malformed JSON — this handles:
   *  - Extra text / markdown fences around the JSON object
   *  - Single quotes instead of double quotes
   *  - Trailing commas before closing braces/brackets
   *  - Unquoted keys
   *  - Boolean strings ("true"/"false") instead of literal booleans
   *  - "none" / "null" strings instead of null
   *  - Missing or extra fields
   */
  private parseClassification(raw: string): ClassificationResult | null {
    // Step 1: Try direct parse first (fast path)
    try {
      const parsed = JSON.parse(raw);
      return this.normalizeResult(parsed);
    } catch { /* fall through to repair */ }

    // Step 2: Extract JSON object from surrounding text / markdown fences
    let cleaned = raw;

    // Strip markdown code fences
    cleaned = cleaned.replace(/```(?:json)?\s*/gi, "").replace(/```/g, "");

    // Find the outermost { ... } block
    const firstBrace = cleaned.indexOf("{");
    const lastBrace = cleaned.lastIndexOf("}");
    if (firstBrace === -1 || lastBrace === -1 || lastBrace <= firstBrace) return null;
    cleaned = cleaned.slice(firstBrace, lastBrace + 1);

    // Step 3: Repair common issues
    // Replace single quotes with double quotes (but not apostrophes inside words)
    cleaned = cleaned.replace(/'/g, '"');

    // Remove trailing commas before } or ]
    cleaned = cleaned.replace(/,\s*([}\]])/g, "$1");

    // Quote unquoted keys: word-chars followed by a colon
    cleaned = cleaned.replace(/(?<=[\s,{])(\w+)\s*:/g, '"$1":');

    // Try parsing again
    try {
      const parsed = JSON.parse(cleaned);
      return this.normalizeResult(parsed);
    } catch { /* fall through to field-level extraction */ }

    // Step 4: Last resort — regex extraction of individual fields
    try {
      const extract = (key: string): string | undefined => {
        const m = cleaned.match(new RegExp(`"${key}"\\s*:\\s*("(?:[^"\\\\]|\\\\.)*"|[^,}\\]]+)`, "i"));
        return m?.[1]?.trim();
      };

      const toBool = (v: string | undefined): boolean => {
        if (!v) return false;
        const s = v.replace(/"/g, "").toLowerCase();
        return s === "true" || s === "1";
      };

      const toNum = (v: string | undefined): number => {
        if (!v) return 0;
        const n = parseInt(v.replace(/"/g, ""), 10);
        return isNaN(n) ? 0 : n;
      };

      const toNullableStr = (v: string | undefined): string | null => {
        if (!v) return null;
        const s = v.replace(/^"|"$/g, "").trim();
        if (!s || s === "null" || s === "none" || s === "N/A") return null;
        return s;
      };

      const VALID_SENTIMENTS: Sentiment[] = ["neutral", "happy", "sad", "angry", "excited", "funny", "love", "scared"];
      const toSentiment = (v: string | undefined): Sentiment => {
        const s = (v ?? "neutral").replace(/^"|"$/g, "").trim().toLowerCase() as Sentiment;
        return VALID_SENTIMENTS.includes(s) ? s : "neutral";
      };

      return this.normalizeResult({
        profanity: toBool(extract("profanity")),
        profanity_severity: toNum(extract("profanity_severity")),
        insult: toBool(extract("insult")),
        insult_type: toNullableStr(extract("insult_type")),
        insult_target: toNullableStr(extract("insult_target")),
        insult_severity: toNum(extract("insult_severity")),
        sentiment: toSentiment(extract("sentiment")),
        gratitude: toBool(extract("gratitude")),
        gratitude_toward: toNullableStr(extract("gratitude_toward")),
        wotd_used: toBool(extract("wotd_used")),
        wotd_correct: toBool(extract("wotd_correct")),
      });
    } catch {
      return null;
    }
  }

  /**
   * Coerce a parsed object into a valid ClassificationResult,
   * fixing type mismatches (e.g. "true" vs true, "none" vs null).
   */
  private normalizeResult(obj: any): ClassificationResult | null {
    if (!obj || typeof obj !== "object") return null;

    const toBool = (v: any): boolean => {
      if (typeof v === "boolean") return v;
      if (typeof v === "string") return v.toLowerCase() === "true" || v === "1";
      return !!v;
    };

    const toSeverity = (v: any): number => {
      const n = typeof v === "number" ? v : parseInt(String(v), 10);
      if (isNaN(n) || n < 0) return 0;
      if (n > 3) return 3;
      return n;
    };

    const toNullableStr = (v: any): string | null => {
      if (v == null) return null;
      const s = String(v).trim();
      if (s === "" || s === "null" || s === "none" || s === "N/A") return null;
      return s;
    };

    const VALID_SENTIMENTS: Sentiment[] = ["neutral", "happy", "sad", "angry", "excited", "funny", "love", "scared"];
    const toSentiment = (v: any): Sentiment => {
      const s = String(v ?? "neutral").toLowerCase().trim() as Sentiment;
      return VALID_SENTIMENTS.includes(s) ? s : "neutral";
    };

    const insultType = toNullableStr(obj.insult_type);

    return {
      profanity: toBool(obj.profanity),
      profanity_severity: toSeverity(obj.profanity_severity),
      insult: toBool(obj.insult),
      insult_type: insultType === "direct" || insultType === "indirect" ? insultType : null,
      insult_target: toNullableStr(obj.insult_target),
      insult_severity: toSeverity(obj.insult_severity),
      sentiment: toSentiment(obj.sentiment),
      gratitude: toBool(obj.gratitude),
      gratitude_toward: toNullableStr(obj.gratitude_toward),
      wotd_used: toBool(obj.wotd_used),
      wotd_correct: toBool(obj.wotd_correct),
    };
  }

  private storeResult(ctx: MessageContext, r: ClassificationResult): void {
    const db = getDb();

    // Always insert classification
    db.prepare(`
      INSERT INTO llm_classifications (user_id, room_id, event_id, profanity, profanity_severity, insult, insult_type, insult_target, insult_severity, sentiment, gratitude, gratitude_toward, wotd_used, wotd_correct)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      ctx.sender, ctx.roomId, ctx.eventId,
      r.profanity ? 1 : 0, r.profanity_severity,
      r.insult ? 1 : 0, r.insult_type, r.insult_target, r.insult_severity, r.sentiment,
      r.gratitude ? 1 : 0, r.gratitude_toward,
      r.wotd_used ? 1 : 0, r.wotd_correct ? 1 : 0
    );

    // Sentiment aggregate
    const sentCol = r.sentiment as string;
    const validSentiments = ["neutral", "happy", "sad", "angry", "excited", "funny", "love", "scared"];
    if (validSentiments.includes(sentCol)) {
      db.prepare(`
        INSERT INTO sentiment_stats (user_id, room_id, ${sentCol}, total_classified)
        VALUES (?, ?, 1, 1)
        ON CONFLICT(user_id, room_id) DO UPDATE SET
          ${sentCol} = ${sentCol} + 1,
          total_classified = total_classified + 1
      `).run(ctx.sender, ctx.roomId);
    }

    // Profanity tracking
    if (r.profanity) {
      const sevCol = `severity_${r.profanity_severity}` as "severity_1" | "severity_2" | "severity_3";
      if (r.profanity_severity >= 1 && r.profanity_severity <= 3) {
        db.prepare(`
          INSERT INTO potty_mouth (user_id, room_id, total, severity_1, severity_2, severity_3)
          VALUES (?, ?, 1, ?, ?, ?)
          ON CONFLICT(user_id, room_id) DO UPDATE SET
            total = total + 1,
            ${sevCol} = ${sevCol} + 1
        `).run(
          ctx.sender, ctx.roomId,
          r.profanity_severity === 1 ? 1 : 0,
          r.profanity_severity === 2 ? 1 : 0,
          r.profanity_severity === 3 ? 1 : 0
        );

        const pottyEmoji = ["\uD83E\uDEE3", "\uD83D\uDE32", "\uD83E\uDD2C"][r.profanity_severity - 1]; // 🫣 😲 🤬
        this.sendReact(ctx.roomId, ctx.eventId, pottyEmoji);
      }
    }

    // Insult tracking
    if (r.insult) {
      const isDirect = r.insult_type === "direct";
      db.prepare(`
        INSERT INTO insult_log (user_id, room_id, total_thrown, direct_thrown, indirect_thrown, times_targeted)
        VALUES (?, ?, 1, ?, ?, 0)
        ON CONFLICT(user_id, room_id) DO UPDATE SET
          total_thrown = total_thrown + 1,
          direct_thrown = direct_thrown + ?,
          indirect_thrown = indirect_thrown + ?
      `).run(
        ctx.sender, ctx.roomId,
        isDirect ? 1 : 0,
        isDirect ? 0 : 1,
        isDirect ? 1 : 0,
        isDirect ? 0 : 1
      );

      const insultEmoji = r.insult_target === this.botUserId ? "\uD83D\uDD95" // 🖕 insulted the bot
        : r.insult_type === "direct" ? "\uD83C\uDFAF" : "\uD83D\uDCA8"; // 🎯 direct, 💨 indirect
      this.sendReact(ctx.roomId, ctx.eventId, insultEmoji);

      // Track target
      if (r.insult_target) {
        db.prepare(`
          INSERT INTO insult_log (user_id, room_id, total_thrown, direct_thrown, indirect_thrown, times_targeted)
          VALUES (?, ?, 0, 0, 0, 1)
          ON CONFLICT(user_id, room_id) DO UPDATE SET
            times_targeted = times_targeted + 1
        `).run(r.insult_target, ctx.roomId);
      }
    }

    // WOTD tracking
    if (r.wotd_used) {
      const today = new Date().toISOString().slice(0, 10);
      const xpAwarded = r.wotd_correct ? WOTD_XP : 0;

      if (r.wotd_correct) {
        this.xpPlugin.grantXp(ctx.sender, ctx.roomId, WOTD_XP, "wotd_correct");
      }

      db.prepare(`
        INSERT INTO wotd_usage (user_id, room_id, date, xp_awarded)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(user_id, room_id, date) DO UPDATE SET
          xp_awarded = MAX(xp_awarded, ?)
      `).run(ctx.sender, ctx.roomId, today, xpAwarded, xpAwarded);

      // React so the user knows their WOTD attempt was detected
      const emoji = r.wotd_correct ? "\uD83D\uDCD6" : "\uD83E\uDD14"; // 📖 if correct, 🤔 if not
      this.sendReact(ctx.roomId, ctx.eventId, emoji);
    }

    // Gratitude — grant rep when the LLM detects genuine thanks
    if (r.gratitude && r.gratitude_toward && r.gratitude_toward !== ctx.sender) {
      const receiverId = r.gratitude_toward;

      // Check 24h cooldown
      const cooldown = db
        .prepare(
          `SELECT granted_at FROM rep_cooldowns
           WHERE giver_id = ? AND receiver_id = ? AND room_id = ?`
        )
        .get(ctx.sender, receiverId, ctx.roomId) as { granted_at: string } | undefined;

      let onCooldown = false;
      if (cooldown) {
        const grantedAt = new Date(cooldown.granted_at + "Z").getTime();
        onCooldown = Date.now() - grantedAt < REP_COOLDOWN_HOURS * 60 * 60 * 1000;
      }

      if (!onCooldown) {
        db.prepare(
          `INSERT INTO rep_cooldowns (giver_id, receiver_id, room_id, granted_at)
           VALUES (?, ?, ?, datetime('now'))
           ON CONFLICT(giver_id, receiver_id, room_id) DO UPDATE SET granted_at = datetime('now')`
        ).run(ctx.sender, receiverId, ctx.roomId);

        db.prepare(
          `INSERT INTO users (user_id, room_id, rep)
           VALUES (?, ?, 1)
           ON CONFLICT(user_id, room_id) DO UPDATE SET rep = rep + 1`
        ).run(receiverId, ctx.roomId);

        this.xpPlugin.grantXp(receiverId, ctx.roomId, REP_XP_BONUS, "reputation");

        this.sendReact(ctx.roomId, ctx.eventId, "\u2705");

        logger.debug(`LLM: ${ctx.sender} gave rep to ${receiverId} in ${ctx.roomId}`);
      }
    }

    // Sentiment reaction (only when no other reaction was sent)
    if (!r.profanity && !r.insult && !r.wotd_used && r.sentiment !== "neutral") {
      const sentimentEmojis: Record<Sentiment, string[]> = {
        neutral: [],
        sad: ["\uD83E\uDEE2", "\uD83D\uDE22", "\uD83E\uDD79", "\uD83D\uDC94", "\uD83E\uDD17"], // 🫢 😢 🥹 💔 🤗
        happy: ["\uD83D\uDE04", "\uD83C\uDF89", "\uD83D\uDE0A", "\u2728", "\uD83D\uDC4F"],       // 😄 🎉 😊 ✨ 👏
        angry: ["\uD83D\uDE24", "\uD83D\uDCA2", "\uD83D\uDE21"],                                   // 😤 💢 😡
        excited: ["\uD83D\uDE06", "\uD83D\uDD25", "\uD83C\uDF89", "\uD83D\uDE4C", "\u26A1"],      // 😆 🔥 🎉 🙌 ⚡
        funny: ["\uD83D\uDE02", "\uD83D\uDC80", "\uD83E\uDD23", "\uD83D\uDE06"],                  // 😂 💀 🤣 😆
        love: ["\u2764\uFE0F", "\uD83E\uDD70", "\uD83D\uDE0D", "\uD83D\uDC96", "\uD83D\uDC9E"],  // ❤️ 🥰 😍 💖 💞
        scared: ["\uD83D\uDE28", "\uD83D\uDE31", "\uD83D\uDE30"],                                  // 😨 😱 😰
      };

      const choices = sentimentEmojis[r.sentiment];
      if (choices.length > 0) {
        const pick = choices[Math.floor(Math.random() * choices.length)];
        this.sendReact(ctx.roomId, ctx.eventId, pick);
      }
    }
  }

  // --- Commands ---

  private async handlePotty(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "potty");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT total, severity_1, severity_2, severity_3 FROM potty_mouth WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { total: number; severity_1: number; severity_2: number; severity_3: number } | undefined;

    if (!row || row.total === 0) {
      await this.sendMessage(ctx.roomId, `${targetUser} has a clean mouth. For now.`);
      return;
    }

    await this.sendMessage(
      ctx.roomId,
      `${targetUser} potty mouth: Total: ${row.total} (mild: ${row.severity_1}, moderate: ${row.severity_2}, scorched: ${row.severity_3})`
    );
  }

  private async handlePottyboard(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, total, severity_1, severity_2, severity_3 FROM potty_mouth WHERE room_id = ? ORDER BY total DESC LIMIT 10`)
      .all(ctx.roomId) as { user_id: string; total: number; severity_1: number; severity_2: number; severity_3: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No potty mouth data yet.");
      return;
    }

    const lines = rows.map(
      (r, i) => `${i + 1}. ${r.user_id} — ${r.total} (mild: ${r.severity_1}, moderate: ${r.severity_2}, scorched: ${r.severity_3})`
    );
    await this.sendMessage(ctx.roomId, `Potty Mouth Leaderboard:\n${lines.join("\n")}`);
  }

  private async handleInsults(ctx: MessageContext): Promise<void> {
    const args = this.getArgs(ctx.body, "insults");
    const targetUser = args.startsWith("@") ? args.split(/\s/)[0] : ctx.sender;

    const db = getDb();
    const row = db
      .prepare(`SELECT total_thrown, direct_thrown, indirect_thrown, times_targeted FROM insult_log WHERE user_id = ? AND room_id = ?`)
      .get(targetUser, ctx.roomId) as { total_thrown: number; direct_thrown: number; indirect_thrown: number; times_targeted: number } | undefined;

    if (!row) {
      await this.sendMessage(ctx.roomId, `${targetUser} has no insult data. A saint, apparently.`);
      return;
    }

    await this.sendMessage(
      ctx.roomId,
      `${targetUser} insults: Thrown: ${row.total_thrown} (direct: ${row.direct_thrown}, indirect: ${row.indirect_thrown}) | Received: ${row.times_targeted} times`
    );
  }

  private async handleInsultboard(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const rows = db
      .prepare(`SELECT user_id, total_thrown, direct_thrown, indirect_thrown FROM insult_log WHERE room_id = ? ORDER BY total_thrown DESC LIMIT 10`)
      .all(ctx.roomId) as { user_id: string; total_thrown: number; direct_thrown: number; indirect_thrown: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No insult data yet.");
      return;
    }

    const lines = rows.map(
      (r, i) => `${i + 1}. ${r.user_id} — ${r.total_thrown} thrown (direct: ${r.direct_thrown}, indirect: ${r.indirect_thrown})`
    );
    await this.sendMessage(ctx.roomId, `Insult Leaderboard:\n${lines.join("\n")}`);
  }

  private async handleWotdAttempts(ctx: MessageContext): Promise<void> {
    const db = getDb();
    const today = new Date().toISOString().slice(0, 10);

    const rows = db
      .prepare(`SELECT user_id, xp_awarded FROM wotd_usage WHERE room_id = ? AND date = ?`)
      .all(ctx.roomId, today) as { user_id: string; xp_awarded: number }[];

    if (rows.length === 0) {
      await this.sendReply(ctx.roomId, ctx.eventId, "No one has attempted today's WOTD yet.");
      return;
    }

    const lines = rows.map(
      (r) => `${r.user_id} — ${r.xp_awarded > 0 ? "credited" : "no credit"}`
    );
    await this.sendMessage(ctx.roomId, `Today's WOTD attempts:\n${lines.join("\n")}`);
  }
}
