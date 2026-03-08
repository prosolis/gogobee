/**
 * Compatibility wrapper around matrix-js-sdk that exposes the same interface
 * our plugins expect from matrix-bot-sdk. This lets us swap the underlying
 * SDK without changing any plugin code.
 */
import * as sdk from "matrix-js-sdk";
import { RoomEvent } from "matrix-js-sdk";
import type { MatrixEvent } from "matrix-js-sdk";
import type { Room } from "matrix-js-sdk";
// Suppress matrix-js-sdk's verbose HTTP logging
// eslint-disable-next-line @typescript-eslint/no-require-imports
try { (require("matrix-js-sdk/lib/logger").logger as any).setLevel("warn"); } catch { /* ignore */ }
import fs from "fs";
import path from "path";
import logger from "./utils/logger";

export interface BotClientOptions {
  homeserverUrl: string;
  accessToken: string;
  userId: string;
  dataDir: string;
}

/**
 * Wraps matrix-js-sdk's MatrixClient with the API surface our plugins use.
 * Crypto is handled automatically by matrix-js-sdk — no manual key management.
 */
export class BotClient {
  private client: sdk.MatrixClient;
  private opts: BotClientOptions;
  private _userId: string;
  private started = false;

  // Event callbacks
  private messageHandlers: ((roomId: string, event: any) => Promise<void>)[] = [];
  private reactionHandlers: ((roomId: string, event: any) => Promise<void>)[] = [];

  private constructor(opts: BotClientOptions, deviceId: string) {
    this.opts = opts;
    this._userId = opts.userId;

    const storeDir = path.join(opts.dataDir, "store");
    if (!fs.existsSync(storeDir)) fs.mkdirSync(storeDir, { recursive: true });

    this.client = sdk.createClient({
      baseUrl: opts.homeserverUrl,
      accessToken: opts.accessToken,
      userId: opts.userId,
      deviceId,
      store: new sdk.MemoryStore(),
    });
  }

  /**
   * Create a BotClient. Reads device ID from device.json (must exist).
   */
  static create(opts: BotClientOptions): BotClient {
    const deviceFile = path.join(opts.dataDir, "device.json");
    let deviceId: string | undefined;
    try {
      if (fs.existsSync(deviceFile)) {
        const data = JSON.parse(fs.readFileSync(deviceFile, "utf-8"));
        deviceId = data.deviceId;
      }
    } catch { /* ignore */ }

    if (!deviceId) {
      throw new Error(
        "No device.json found. The access token must be obtained via password login " +
        "to create a device identity. Set MATRIX_BOT_PASSWORD."
      );
    }

    logger.info(`Using device ID: ${deviceId}`);
    return new BotClient(opts, deviceId);
  }

  private saveDeviceId(): void {
    const deviceId = this.client.getDeviceId();
    if (deviceId) {
      const deviceFile = path.join(this.opts.dataDir, "device.json");
      fs.writeFileSync(deviceFile, JSON.stringify({ deviceId }), "utf-8");
    }
  }

  /**
   * Initialize crypto and start syncing.
   */
  async start(): Promise<void> {
    // Initialize rust crypto — this handles everything:
    // key management, session rotation, key gossiping, device tracking
    const cryptoDir = path.join(this.opts.dataDir, "crypto-js");
    if (!fs.existsSync(cryptoDir)) fs.mkdirSync(cryptoDir, { recursive: true });

    await this.client.initRustCrypto({
      cryptoDatabasePrefix: cryptoDir + "/",
    });

    // Save device ID for persistence across restarts
    this.saveDeviceId();

    logger.info(`E2EE initialized with device ${this.client.getDeviceId()}`);

    // Wire up event listeners
    this.client.on(RoomEvent.Timeline, (event: MatrixEvent, room: Room | undefined) => {
      if (!room) return;
      const roomId = room.roomId;

      if (event.isBeingDecrypted() || event.getType() === "m.room.encrypted") {
        // Wait for decryption to complete (or fail)
        event.once("Event.decrypted" as any, () => {
          if (event.getType() === "m.room.encrypted") {
            // Decryption failed — still encrypted
            logger.warn(
              `Unable to decrypt event ${event.getId()} from ${event.getSender()} ` +
              `in ${roomId} (session: ${(event.getContent() as any)?.session_id?.substring(0, 8)}...)`
            );
          } else {
            this.handleTimelineEvent(roomId, event);
          }
        });

        // Timeout: if decryption doesn't happen within 10s, the event is likely undecryptable
        setTimeout(() => {
          if (event.getType() === "m.room.encrypted") {
            logger.warn(
              `Decryption timeout for ${event.getId()} from ${event.getSender()} in ${roomId}`
            );
          }
        }, 10_000);
      } else {
        this.handleTimelineEvent(roomId, event);
      }
    });

    // Auto-join on invite
    this.client.on("RoomMember.membership" as any, (_event: any, member: any) => {
      if (member.membership === "invite" && member.userId === this._userId) {
        this.client.joinRoom(member.roomId).catch((err: any) => {
          logger.error(`Failed to auto-join ${member.roomId}: ${err}`);
        });
      }
    });

    // Register sync listener BEFORE starting client to avoid race condition
    const syncReady = new Promise<void>((resolve) => {
      const onSync = (state: string) => {
        if (state === "PREPARED" || state === "SYNCING") {
          logger.info(`Initial sync complete (state: ${state})`);
          this.started = true;
          this.client.removeListener("sync" as any, onSync);
          resolve();
        }
      };
      this.client.on("sync" as any, onSync);
    });

    // Start syncing
    await this.client.startClient({ initialSyncLimit: 10 });

    // Wait for first sync to complete
    await syncReady;
  }

  private handleTimelineEvent(roomId: string, event: MatrixEvent): void {
    const type = event.getType();
    const sender = event.getSender();
    const age = Date.now() - event.getTs();

    logger.debug(`Timeline event: type=${type} sender=${sender} age=${Math.round(age / 1000)}s room=${roomId}`);

    // Skip own messages
    if (sender === this._userId) return;
    // Skip non-live events (historical)
    if (age > 60_000) return;

    const rawEvent = {
      type: event.getType(),
      event_id: event.getId(),
      sender: event.getSender(),
      room_id: roomId,
      content: event.getContent(),
      origin_server_ts: event.getTs(),
    };

    if (event.getType() === "m.room.message") {
      for (const handler of this.messageHandlers) {
        handler(roomId, rawEvent).catch((err) =>
          logger.error(`Message handler error: ${err}`)
        );
      }
    } else if (event.getType() === "m.reaction") {
      for (const handler of this.reactionHandlers) {
        handler(roomId, rawEvent).catch((err) =>
          logger.error(`Reaction handler error: ${err}`)
        );
      }
    }
  }

  // --- Event registration (matching matrix-bot-sdk pattern) ---

  onMessage(handler: (roomId: string, event: any) => Promise<void>): void {
    this.messageHandlers.push(handler);
  }

  onReaction(handler: (roomId: string, event: any) => Promise<void>): void {
    this.reactionHandlers.push(handler);
  }

  // --- Methods used by plugins (same signatures as matrix-bot-sdk) ---

  async getUserId(): Promise<string> {
    return this._userId;
  }

  getDeviceId(): string | null {
    return this.client.getDeviceId();
  }

  async sendText(roomId: string, text: string): Promise<string> {
    const res = await this.client.sendTextMessage(roomId, text);
    return res.event_id;
  }

  async sendNotice(roomId: string, text: string): Promise<string> {
    const res = await this.client.sendNotice(roomId, text);
    return res.event_id;
  }

  async sendMessage(roomId: string, content: any): Promise<string> {
    const res = await this.client.sendMessage(roomId, content);
    return res.event_id;
  }

  async sendEvent(roomId: string, eventType: string, content: any): Promise<string> {
    const res = await this.client.sendEvent(roomId, eventType as any, content);
    return res.event_id;
  }

  async getEvent(roomId: string, eventId: string): Promise<any> {
    return await this.client.fetchRoomEvent(roomId, eventId);
  }

  async getJoinedRooms(): Promise<string[]> {
    const res = await this.client.getJoinedRooms();
    return res.joined_rooms;
  }

  async getJoinedRoomMembers(roomId: string): Promise<string[]> {
    const room = this.client.getRoom(roomId);
    if (!room) return [];
    return room.getJoinedMembers().map((m) => m.userId);
  }

  async joinRoom(roomId: string): Promise<void> {
    await this.client.joinRoom(roomId);
  }

  async uploadContent(data: Buffer, contentType?: string, filename?: string): Promise<string> {
    const res = await this.client.uploadContent(data, {
      type: contentType,
      name: filename,
    });
    return res.content_uri;
  }

  async sendToDevices(type: string, messages: Record<string, Record<string, any>>): Promise<void> {
    // matrix-js-sdk expects Map<string, Map<string, object>>
    const contentMap = new Map<string, Map<string, Record<string, any>>>();
    for (const [userId, devices] of Object.entries(messages)) {
      const deviceMap = new Map<string, Record<string, any>>();
      for (const [deviceId, content] of Object.entries(devices)) {
        deviceMap.set(deviceId, content);
      }
      contentMap.set(userId, deviceMap);
    }
    await this.client.sendToDevice(type, contentMap);
  }

  async getOwnDevices(): Promise<any[]> {
    const res = await this.client.getDevices();
    return res.devices;
  }

  isRoomEncrypted(roomId: string): boolean {
    return this.client.isRoomEncrypted(roomId);
  }

  // DM support
  dms = {
    getOrCreateDm: async (userId: string): Promise<string> => {
      // Check for existing DM
      const rooms = this.client.getRooms();
      for (const room of rooms) {
        const members = room.getJoinedMembers();
        if (members.length === 2 && members.some((m) => m.userId === userId)) {
          return room.roomId;
        }
      }

      // Create new DM
      const res = await this.client.createRoom({
        is_direct: true,
        invite: [userId],
        preset: "trusted_private_chat" as any,
      });
      return res.room_id;
    },
  };

  // Crypto accessors for compatibility
  crypto = {
    isRoomEncrypted: (roomId: string): boolean => {
      return this.client.isRoomEncrypted(roomId);
    },
    clientDeviceId: "" as string,
  };

  async stop(): Promise<void> {
    this.client.stopClient();
  }

  /** Access the underlying matrix-js-sdk client for advanced operations */
  get raw(): sdk.MatrixClient {
    return this.client;
  }
}
