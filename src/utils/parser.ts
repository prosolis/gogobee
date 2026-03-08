export interface ParsedMessage {
  wordCount: number;
  charCount: number;
  linkCount: number;
  imageCount: number;
  questionCount: number;
  exclamationCount: number;
  emojiCount: number;
  longestWord: string;
  hasLongWord: boolean; // word > 15 characters
}

const URL_REGEX = /https?:\/\/[^\s]+/gi;
const EMOJI_REGEX = /[\p{Emoji_Presentation}\p{Extended_Pictographic}]/gu;
const IMAGE_EXTENSIONS = /\.(png|jpg|jpeg|gif|webp|svg|bmp)(\?[^\s]*)?$/i;

export function parseMessage(body: string): ParsedMessage {
  const words = body.split(/\s+/).filter((w) => w.length > 0);
  const links = body.match(URL_REGEX) ?? [];
  const emojis = body.match(EMOJI_REGEX) ?? [];
  const questions = (body.match(/\?/g) ?? []).length;
  const exclamations = (body.match(/!/g) ?? []).length;
  const images = links.filter((l) => IMAGE_EXTENSIONS.test(l)).length;

  let longestWord = "";
  for (const word of words) {
    const cleaned = word.replace(/[^\w]/g, "");
    if (cleaned.length > longestWord.length) {
      longestWord = cleaned;
    }
  }

  return {
    wordCount: words.length,
    charCount: body.length,
    linkCount: links.length,
    imageCount: images,
    questionCount: questions,
    exclamationCount: exclamations,
    emojiCount: emojis.length,
    longestWord,
    hasLongWord: longestWord.length > 15,
  };
}

// XP curve: level N requires N*100 total XP
export function levelToXp(level: number): number {
  return level * 100;
}

export function xpToLevel(xp: number): number {
  return Math.max(1, Math.floor(xp / 100));
}

export function xpForNextLevel(xp: number): { current: number; needed: number } {
  const currentLevel = xpToLevel(xp);
  const nextLevelXp = levelToXp(currentLevel + 1);
  const currentLevelXp = levelToXp(currentLevel);
  return {
    current: xp - currentLevelXp,
    needed: nextLevelXp - currentLevelXp,
  };
}

export function progressBar(current: number, total: number, length: number = 10): string {
  const filled = Math.round((current / total) * length);
  const empty = length - filled;
  return "[" + "\u2588".repeat(filled) + "\u2591".repeat(empty) + "]";
}

const ARCHETYPES: { key: string; label: string; check: (s: ArchetypeStats) => boolean }[] = [
  { key: "chatterbox", label: "The Chatterbox", check: (s) => s.totalMessages > 1000 && s.avgWords < 10 },
  { key: "novelist", label: "The Novelist", check: (s) => s.avgWords > 40 },
  { key: "inquisitor", label: "The Inquisitor", check: (s) => s.questionRatio > 0.3 },
  { key: "linkmaster", label: "The Linkmaster", check: (s) => s.linkRatio > 0.2 },
  { key: "shutterbug", label: "The Shutterbug", check: (s) => s.imageRatio > 0.15 },
  { key: "night_owl", label: "The Night Owl", check: (s) => s.nightRatio > 0.4 },
  { key: "early_bird", label: "The Early Bird", check: (s) => s.morningRatio > 0.4 },
  { key: "enthusiast", label: "The Enthusiast", check: (s) => s.exclamationRatio > 0.3 },
  { key: "regular", label: "The Regular", check: () => true },
];

interface ArchetypeStats {
  totalMessages: number;
  avgWords: number;
  questionRatio: number;
  linkRatio: number;
  imageRatio: number;
  nightRatio: number;
  morningRatio: number;
  exclamationRatio: number;
}

export function deriveArchetype(stats: {
  total_messages: number;
  avg_words_per_message: number;
  total_questions: number;
  total_links: number;
  total_images: number;
  total_exclamations: number;
  hourly_distribution: string;
}): string {
  const hourly: Record<string, number> = JSON.parse(stats.hourly_distribution || "{}");
  const totalFromHourly = Object.values(hourly).reduce((a, b) => a + b, 0) || 1;

  const nightMessages = [0, 1, 2, 3].reduce((sum, h) => sum + (hourly[h] ?? 0), 0);
  const morningMessages = [5, 6, 7, 8, 9].reduce((sum, h) => sum + (hourly[h] ?? 0), 0);

  const s: ArchetypeStats = {
    totalMessages: stats.total_messages || 1,
    avgWords: stats.avg_words_per_message,
    questionRatio: stats.total_questions / (stats.total_messages || 1),
    linkRatio: stats.total_links / (stats.total_messages || 1),
    imageRatio: stats.total_images / (stats.total_messages || 1),
    nightRatio: nightMessages / totalFromHourly,
    morningRatio: morningMessages / totalFromHourly,
    exclamationRatio: stats.total_exclamations / (stats.total_messages || 1),
  };

  for (const archetype of ARCHETYPES) {
    if (archetype.check(s)) return archetype.label;
  }

  return "The Regular";
}
