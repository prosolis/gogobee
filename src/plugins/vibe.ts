import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import logger from "../utils/logger";

const OLLAMA_HOST = (() => {
  const raw = process.env.OLLAMA_HOST ?? "";
  return raw && !raw.startsWith("http") ? `http://${raw}` : raw;
})();
const OLLAMA_MODEL = process.env.OLLAMA_MODEL ?? "";
const LLM_ENABLED = OLLAMA_HOST !== "" && OLLAMA_MODEL !== "";
const OLLAMA_TIMEOUT_MS = 45_000;
const OLLAMA_NUM_CTX = 16384;

const BUFFER_CAP = 50;
const MIN_MESSAGES = 10;
const COOLDOWN_MS = 5 * 60 * 1000; // 5 minutes per room per command

interface BufferedMessage {
  sender: string;
  body: string;
}

export class VibePlugin extends Plugin {
  private buffers = new Map<string, BufferedMessage[]>();
  private vibeCooldowns = new Map<string, number>();
  private tldrCooldowns = new Map<string, number>();

  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "vibe";
  }

  get commands(): CommandDef[] {
    return [
      { name: "vibe", description: "Describe the current room energy", usage: "!vibe" },
      { name: "tldr", description: "Summarize recent conversation", usage: "!tldr [n]" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Buffer only plain text messages (skip commands for vibe/tldr)
    const msgtype = ctx.event?.content?.msgtype;
    if (msgtype === "m.text" && !this.isCommand(ctx.body, "vibe") && !this.isCommand(ctx.body, "tldr")) {
      this.pushMessage(ctx.roomId, ctx.sender, ctx.body);
    }

    if (this.isCommand(ctx.body, "vibe")) {
      await this.handleVibe(ctx);
    } else if (this.isCommand(ctx.body, "tldr")) {
      await this.handleTldr(ctx);
    }
  }

  private pushMessage(roomId: string, sender: string, body: string): void {
    let buf = this.buffers.get(roomId);
    if (!buf) {
      buf = [];
      this.buffers.set(roomId, buf);
    }
    buf.push({ sender, body });
    if (buf.length > BUFFER_CAP) {
      buf.shift();
    }
  }

  private checkCooldown(cooldowns: Map<string, number>, roomId: string): number | null {
    const lastUsed = cooldowns.get(roomId) ?? 0;
    const remaining = lastUsed + COOLDOWN_MS - Date.now();
    if (remaining > 0) {
      return Math.ceil(remaining / 1000);
    }
    return null;
  }

  // --- !vibe ---

  private async handleVibe(ctx: MessageContext): Promise<void> {
    if (!LLM_ENABLED) {
      await this.sendReply(ctx.roomId, ctx.eventId, "LLM features are not configured.");
      return;
    }

    const cooldownSecs = this.checkCooldown(this.vibeCooldowns, ctx.roomId);
    if (cooldownSecs) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Vibe check on cooldown. Try again in ${cooldownSecs}s.`);
      return;
    }

    const buf = this.buffers.get(ctx.roomId);
    if (!buf || buf.length < MIN_MESSAGES) {
      const have = buf?.length ?? 0;
      await this.sendReply(ctx.roomId, ctx.eventId, `Not enough recent context yet (${have}/${MIN_MESSAGES} messages). Keep chatting.`);
      return;
    }

    this.vibeCooldowns.set(ctx.roomId, Date.now());

    try {
      const vibe = await this.generateVibe(buf);
      if (vibe) {
        await this.sendMessage(ctx.roomId, vibe);
      } else {
        await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't read the room. Try again later.");
      }
    } catch (err) {
      logger.error(`Vibe generation failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't read the room. Try again later.");
    }
  }

  private async generateVibe(messages: BufferedMessage[]): Promise<string | null> {
    const transcript = messages.map((m) => `${m.sender}: ${m.body}`).join("\n");

    const prompt = `You are Freebee, a snarky but affectionate chat bot in a private friend group. Someone just asked you to read the room and describe the current vibe.

Below is a transcript of the last ${messages.length} messages. Describe the room's current energy in 1-2 sentences. Be creative, funny, and specific to what's actually being discussed. Think of it like a weather report for the chat's mood.

Examples of the tone we want:
- "Chaotic gremlin energy with undertones of unresolved technical debt."
- "Three people passionately disagreeing about something none of them will remember tomorrow."
- "Suspiciously wholesome. Someone's about to ruin it."
- "One person carrying the entire conversation while everyone else lurks in silence."

Rules:
- 1-2 sentences max. Short and punchy.
- Reference actual topics or dynamics from the transcript, not generic vibes.
- Do NOT name specific users. Describe roles/dynamics instead ("someone", "one person", "half the room").
- Do NOT use bullet points or formatting. Just flowing text.
- Do NOT start with "The vibe is" or "Current vibe:". Just describe it directly.

Transcript:
${transcript}

Describe the vibe now. Raw text only.`;

    return this.callOllama(prompt, "vibe");
  }

  // --- !tldr ---

  private async handleTldr(ctx: MessageContext): Promise<void> {
    if (!LLM_ENABLED) {
      await this.sendReply(ctx.roomId, ctx.eventId, "LLM features are not configured.");
      return;
    }

    const cooldownSecs = this.checkCooldown(this.tldrCooldowns, ctx.roomId);
    if (cooldownSecs) {
      await this.sendReply(ctx.roomId, ctx.eventId, `TLDR on cooldown. Try again in ${cooldownSecs}s.`);
      return;
    }

    const args = this.getArgs(ctx.body, "tldr").trim();
    let count = BUFFER_CAP;
    if (args) {
      const parsed = parseInt(args, 10);
      if (!isNaN(parsed) && parsed > 0) {
        count = Math.min(parsed, BUFFER_CAP);
      }
    }

    const buf = this.buffers.get(ctx.roomId);
    if (!buf || buf.length < MIN_MESSAGES) {
      const have = buf?.length ?? 0;
      await this.sendReply(ctx.roomId, ctx.eventId, `Not enough recent context yet (${have}/${MIN_MESSAGES} messages). Keep chatting.`);
      return;
    }

    const messages = buf.slice(-count);
    this.tldrCooldowns.set(ctx.roomId, Date.now());

    try {
      const summary = await this.generateTldr(messages);
      if (summary) {
        await this.sendMessage(ctx.roomId, summary);
      } else {
        await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't summarize. Try again later.");
      }
    } catch (err) {
      logger.error(`TLDR generation failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Couldn't summarize. Try again later.");
    }
  }

  private async generateTldr(messages: BufferedMessage[]): Promise<string | null> {
    const transcript = messages.map((m) => `${m.sender}: ${m.body}`).join("\n");

    const prompt = `You are Freebee, a helpful chat bot in a private friend group. Someone just asked for a summary of the recent conversation so they can catch up.

Below is a transcript of the last ${messages.length} messages. Summarize what happened in a concise, easy-to-scan format.

Rules:
- Start with "TLDR:" followed by the summary.
- Group by topic/thread if the conversation covered multiple subjects.
- Keep each topic to 1 sentence.
- Use casual, natural language — not corporate meeting notes.
- You MAY reference usernames since this is a summary, not a vibe check.
- Keep the entire summary under 500 characters.
- No bullet points — use short flowing sentences separated by line breaks.

Transcript:
${transcript}

Write the summary now. Raw text only.`;

    return this.callOllama(prompt, "tldr");
  }

  // --- shared ---

  private async callOllama(prompt: string, label: string): Promise<string | null> {
    const res = await fetch(`${OLLAMA_HOST}/api/generate`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: OLLAMA_MODEL,
        prompt,
        stream: false,
        options: { num_ctx: OLLAMA_NUM_CTX, temperature: label === "tldr" ? 0.3 : 0.9 },
      }),
      signal: AbortSignal.timeout(OLLAMA_TIMEOUT_MS),
    });

    if (!res.ok) {
      logger.warn(`Ollama API returned ${res.status} for ${label}`);
      return null;
    }

    const data = (await res.json()) as { response: string };
    let text = data.response?.trim();
    if (!text) return null;

    // Clean up LLM quirks
    text = text.replace(/^["']|["']$/g, "");
    text = text.replace(/^#+\s*/gm, "");
    text = text.replace(/\*\*/g, "");

    if (label === "vibe") {
      text = `Vibe check: ${text}`;
    }

    return text;
  }
}
