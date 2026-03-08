import "fake-indexeddb/auto";
import "dotenv/config";
import path from "path";
import fs from "fs";
import { BotClient } from "./matrix-client";
import { initDb } from "./db";
import logger from "./utils/logger";
import { loginWithPassword, isTokenValid } from "./utils/auth";
import { PluginRegistry } from "./plugins/base";
import { XpPlugin } from "./plugins/xp";
import { ReputationPlugin } from "./plugins/reputation";
import { StatsPlugin } from "./plugins/stats";
import { StreaksPlugin } from "./plugins/streaks";
import { RemindersPlugin } from "./plugins/reminders";
import { UserPlugin } from "./plugins/user";
import { FunPlugin } from "./plugins/fun";
import { WotdPlugin } from "./plugins/wotd";
import { HolidaysPlugin } from "./plugins/holidays";
import { GamingPlugin } from "./plugins/gaming";
import { DailyScheduler } from "./plugins/daily";
import { AchievementsPlugin } from "./plugins/achievements";
import { ShadePlugin } from "./plugins/shade";
import { RateLimitsPlugin } from "./plugins/ratelimits";
import { BirthdayPlugin } from "./plugins/birthday";
import { TriviaPlugin } from "./plugins/trivia";
import { LlmPassivePlugin } from "./plugins/llm-passive";
import { StocksPlugin } from "./plugins/stocks";
import { ConcertsPlugin } from "./plugins/concerts";
import { AnimePlugin } from "./plugins/anime";
import { MoviesPlugin } from "./plugins/movies";
import { LookupPlugin } from "./plugins/lookup";
import { PresencePlugin } from "./plugins/presence";
import { CountdownPlugin } from "./plugins/countdown";
import { WelcomePlugin } from "./plugins/welcome";
import { MarkovPlugin } from "./plugins/markov";
import { UrlsPlugin } from "./plugins/urls";
import { ToolsPlugin } from "./plugins/tools";
import { ReactionsPlugin } from "./plugins/reactions";
import { BotInfoPlugin } from "./plugins/botinfo";
import { RetroPlugin } from "./plugins/retro";
import { HowAmIPlugin } from "./plugins/howami";
import { VibePlugin } from "./plugins/vibe";

/**
 * Ensure we have a valid access token. If the current token is invalid and a
 * password is configured, log in automatically and persist the new token.
 */
async function resolveAccessToken(
  homeserverUrl: string,
  botUserId: string,
  currentToken: string | undefined,
  password: string | undefined,
  dataDir: string
): Promise<string> {
  const deviceFile = path.join(dataDir, "device.json");
  const hasDeviceFile = fs.existsSync(deviceFile);

  // Try to load the stored token from device.json first
  let storedToken: string | undefined;
  if (hasDeviceFile) {
    try {
      const data = JSON.parse(fs.readFileSync(deviceFile, "utf-8"));
      storedToken = data.accessToken;
    } catch { /* ignore */ }
  }

  // Prefer stored token (device.json) over env var
  const effectiveToken = storedToken ?? currentToken;

  // If we have a stored device identity AND a valid token, reuse them
  if (hasDeviceFile && effectiveToken) {
    const valid = await isTokenValid(homeserverUrl, effectiveToken);
    if (valid) {
      logger.info("Access token is valid, device identity exists");
      return effectiveToken;
    }
    logger.warn("Access token is invalid or expired");
  }

  // No device.json means fresh crypto store — we MUST do a password login
  // to get a new token + device pair. Reusing an old token's device would
  // cause OTK conflicts since the server has keys we don't have locally.
  if (!hasDeviceFile && effectiveToken) {
    logger.info("No device.json found — fresh crypto store requires a new device via password login");
  }

  if (!password) {
    if (!hasDeviceFile) {
      logger.error(
        "No device.json found and no MATRIX_BOT_PASSWORD set. " +
        "A fresh crypto store requires a password login to create a new device. " +
        "Set MATRIX_BOT_PASSWORD to proceed."
      );
    } else {
      logger.error(
        "Access token is invalid and no MATRIX_BOT_PASSWORD is set. " +
        "Provide a valid MATRIX_ACCESS_TOKEN or set MATRIX_BOT_PASSWORD for automatic renewal."
      );
    }
    process.exit(1);
  }

  logger.info("Performing password login to obtain a new access token and device...");

  // Reuse existing device ID only if we have the crypto store to match
  let existingDeviceId: string | undefined;
  if (hasDeviceFile) {
    try {
      const data = JSON.parse(fs.readFileSync(deviceFile, "utf-8"));
      existingDeviceId = data.deviceId;
    } catch { /* ignore parse errors */ }
  }

  const loginResult = await loginWithPassword(homeserverUrl, botUserId, password, existingDeviceId);

  // Save the new device ID and access token together
  fs.writeFileSync(
    deviceFile,
    JSON.stringify({ deviceId: loginResult.device_id, accessToken: loginResult.access_token }),
    "utf-8"
  );
  fs.chmodSync(deviceFile, 0o600);
  logger.info(`Device identity saved: ${loginResult.device_id}`);

  return loginResult.access_token;
}

async function main(): Promise<void> {
  const dataDir = process.env.DATA_DIR ?? "./data";
  const homeserverUrl = process.env.MATRIX_HOMESERVER_URL;
  const botUserId = process.env.MATRIX_BOT_USER_ID;
  const botPassword = process.env.MATRIX_BOT_PASSWORD;

  if (!homeserverUrl || !botUserId) {
    logger.error("Missing required env vars: MATRIX_HOMESERVER_URL, MATRIX_BOT_USER_ID");
    process.exit(1);
  }

  if (!process.env.MATRIX_ACCESS_TOKEN && !botPassword) {
    logger.error("Provide either MATRIX_ACCESS_TOKEN or MATRIX_BOT_PASSWORD (or both)");
    process.exit(1);
  }

  // Ensure data directory exists
  fs.mkdirSync(dataDir, { recursive: true });

  // Crypto reset: wipe local crypto store and device identity
  const resetMarker = path.join(dataDir, ".crypto_reset_needed");
  const needsCryptoReset = process.env.CRYPTO_RESET === "true" || fs.existsSync(resetMarker);
  if (needsCryptoReset) {
    const cryptoDir = path.join(dataDir, "crypto-js");
    const deviceFile = path.join(dataDir, "device.json");

    // Try to delete the old device from the server
    let oldDeviceId: string | undefined;
    if (fs.existsSync(deviceFile)) {
      try {
        const data = JSON.parse(fs.readFileSync(deviceFile, "utf-8"));
        oldDeviceId = data.deviceId;
      } catch { /* ignore */ }
    }

    if (oldDeviceId && process.env.MATRIX_ACCESS_TOKEN) {
      try {
        const res = await fetch(`${homeserverUrl}/_matrix/client/v3/devices/${encodeURIComponent(oldDeviceId)}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${process.env.MATRIX_ACCESS_TOKEN}`, "Content-Type": "application/json" },
          body: JSON.stringify({ auth: { type: "m.login.password" } }),
        });
        if (res.ok || res.status === 401) {
          logger.info(`Deleted old device ${oldDeviceId} from server`);
        } else {
          logger.warn(`Could not delete old device ${oldDeviceId}: ${res.status}`);
        }
      } catch (err) {
        logger.warn(`Could not delete old device ${oldDeviceId}: ${err}`);
      }
    }

    if (fs.existsSync(cryptoDir)) fs.rmSync(cryptoDir, { recursive: true });
    if (fs.existsSync(deviceFile)) fs.unlinkSync(deviceFile);
    if (fs.existsSync(resetMarker)) fs.unlinkSync(resetMarker);
    logger.info("Crypto reset: wiped crypto store and device.json. Starting with fresh device identity.");
  }

  // Initialize database
  initDb(dataDir);

  // Validate / refresh the access token
  const accessToken = await resolveAccessToken(
    homeserverUrl,
    botUserId,
    process.env.MATRIX_ACCESS_TOKEN,
    botPassword,
    dataDir
  );

  // Create the BotClient (wraps matrix-js-sdk with automatic E2EE)
  const botClient = BotClient.create({
    homeserverUrl,
    accessToken,
    userId: botUserId,
    dataDir,
  });

  // Parse bot rooms
  const botRooms = (process.env.BOT_ROOMS ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  // Instantiate plugins in dependency order
  const xpPlugin = new XpPlugin(botClient);
  const repPlugin = new ReputationPlugin(botClient, xpPlugin);
  const statsPlugin = new StatsPlugin(botClient);
  const streaksPlugin = new StreaksPlugin(botClient);
  const remindersPlugin = new RemindersPlugin(botClient);
  const userPlugin = new UserPlugin(botClient);
  const funPlugin = new FunPlugin(botClient);
  const wotdPlugin = new WotdPlugin(botClient, xpPlugin);
  const holidaysPlugin = new HolidaysPlugin(botClient);
  const gamingPlugin = new GamingPlugin(botClient);
  const achievementsPlugin = new AchievementsPlugin(botClient);
  const rateLimitPlugin = new RateLimitsPlugin(botClient);
  const birthdayPlugin = new BirthdayPlugin(botClient, xpPlugin);
  const triviaPlugin = new TriviaPlugin(botClient, xpPlugin);
  const llmPassivePlugin = new LlmPassivePlugin(botClient, xpPlugin);
  const stocksPlugin = new StocksPlugin(botClient);
  const concertsPlugin = new ConcertsPlugin(botClient, rateLimitPlugin);
  const animePlugin = new AnimePlugin(botClient);
  const moviesPlugin = new MoviesPlugin(botClient);
  const lookupPlugin = new LookupPlugin(botClient, rateLimitPlugin);
  const presencePlugin = new PresencePlugin(botClient);
  const countdownPlugin = new CountdownPlugin(botClient);
  const markovPlugin = new MarkovPlugin(botClient);
  const urlsPlugin = new UrlsPlugin(botClient);
  const toolsPlugin = new ToolsPlugin(botClient);
  const reactionsPlugin = new ReactionsPlugin(botClient);
  const botInfoPlugin = new BotInfoPlugin(botClient);
  const retroPlugin = new RetroPlugin(botClient);
  const howAmIPlugin = new HowAmIPlugin(botClient);
  const vibePlugin = new VibePlugin(botClient);

  const dailyScheduler = new DailyScheduler(
    botClient,
    remindersPlugin,
    wotdPlugin,
    holidaysPlugin,
    gamingPlugin,
    birthdayPlugin,
    animePlugin,
    moviesPlugin,
    concertsPlugin,
    botRooms
  );

  // Register plugins with the registry
  const registry = new PluginRegistry(botUserId);

  // XP first (other plugins depend on it being processed first for passive XP)
  registry.register(xpPlugin);
  registry.register(repPlugin);
  registry.register(statsPlugin);
  registry.register(streaksPlugin);
  registry.register(presencePlugin);
  registry.register(remindersPlugin);
  registry.register(userPlugin);
  registry.register(funPlugin);
  registry.register(wotdPlugin);
  registry.register(holidaysPlugin);
  registry.register(gamingPlugin);
  registry.register(rateLimitPlugin);
  registry.register(birthdayPlugin);
  registry.register(triviaPlugin);
  registry.register(llmPassivePlugin);
  registry.register(stocksPlugin);
  registry.register(concertsPlugin);
  registry.register(animePlugin);
  registry.register(moviesPlugin);
  registry.register(lookupPlugin);
  registry.register(countdownPlugin);
  registry.register(markovPlugin);
  registry.register(urlsPlugin);
  registry.register(toolsPlugin);
  registry.register(reactionsPlugin);
  registry.register(botInfoPlugin);
  registry.register(retroPlugin);
  registry.register(howAmIPlugin);
  registry.register(vibePlugin);
  registry.register(achievementsPlugin);
  registry.register(dailyScheduler);

  // Welcome plugin needs registry reference for !help
  const welcomePlugin = new WelcomePlugin(botClient, xpPlugin, registry);
  registry.register(welcomePlugin);

  // Register shade plugin only if feature flag is enabled
  if (process.env.FEATURE_SHADE === "true") {
    const shadePlugin = new ShadePlugin(botClient);
    registry.register(shadePlugin);
  }

  // Wire Matrix events to plugin registry via BotClient
  botClient.onMessage(async (roomId: string, event: any) => {
    await registry.dispatch(roomId, event);
  });

  botClient.onReaction(async (roomId: string, event: any) => {
    await registry.dispatchReaction(roomId, event);
  });

  // Start the scheduler
  dailyScheduler.start();

  // Start the client (initializes crypto + starts syncing)
  await botClient.start();

  logger.info(`Freebee started as ${botUserId}`);
  logger.info(`Listening in ${botRooms.length} configured room(s)`);
  logger.info(`Registered ${registry["plugins"].length} plugin(s)`);
  logger.info(`E2EE is handled automatically by matrix-js-sdk (Rust crypto)`);

  // Kick-start key exchange in bot rooms (non-blocking).
  // Sending an encrypted message advertises our device to room members.
  if (botRooms.length > 0) {
    (async () => {
      for (const roomId of botRooms) {
        try {
          if (!botClient.isRoomEncrypted(roomId)) continue;
          if (needsCryptoReset) {
            await botClient.sendNotice(roomId, "Encryption keys have been refreshed.");
          } else {
            await botClient.sendNotice(roomId, "\u200B");
          }
          logger.info(`Key exchange ping sent to ${roomId}`);
        } catch (err) {
          logger.warn(`Key exchange message failed for ${roomId}: ${err}`);
        }
      }
    })().catch((err) => logger.error(`Key exchange loop failed: ${err}`));
  }

  // Catch up missed scheduled jobs after sync is ready
  dailyScheduler.catchUpMissedJobs().catch((err) => {
    logger.error(`Catch-up failed: ${err}`);
  });

  // Graceful shutdown
  const shutdown = () => {
    logger.info("Shutting down...");
    dailyScheduler.stop();
    botClient.stop();
    // Delete device.json so next start creates a fresh device via password login.
    // The in-memory IndexedDB crypto store doesn't persist, so reusing the same
    // device ID would cause OTK conflicts with the server.
    const deviceFile = path.join(dataDir, "device.json");
    try { if (fs.existsSync(deviceFile)) fs.unlinkSync(deviceFile); } catch { /* ignore */ }
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);
}

main().catch((err) => {
  logger.error(`Fatal startup error: ${err}`);
  process.exit(1);
});
