import logger from "../utils/logger";

/** Common client interface — works with both matrix-bot-sdk and our BotClient wrapper */
export interface IMatrixClient {
  getUserId(): Promise<string> | string | null;
  sendText(roomId: string, text: string): Promise<string>;
  sendMessage(roomId: string, content: any): Promise<string>;
  sendEvent(roomId: string, eventType: string, content: any): Promise<string>;
  sendNotice(roomId: string, text: string): Promise<string>;
  getEvent(roomId: string, eventId: string): Promise<any>;
  getJoinedRooms(): Promise<string[]>;
  getJoinedRoomMembers(roomId: string): Promise<string[]>;
  uploadContent(data: Buffer, contentType?: string, filename?: string): Promise<string>;
  dms: { getOrCreateDm(userId: string): Promise<string> };
}

export interface CommandDef {
  name: string;
  description: string;
  usage?: string;
  adminOnly?: boolean;
}

export interface MessageContext {
  roomId: string;
  sender: string;
  body: string;
  eventId: string;
  event: any;
}

export interface ReactionContext {
  roomId: string;
  sender: string;
  eventId: string;
  reactionKey: string;
  targetEventId: string;
  event: any;
}

export abstract class Plugin {
  protected client: IMatrixClient;
  protected prefix: string;

  constructor(client: IMatrixClient) {
    this.client = client;
    this.prefix = process.env.BOT_PREFIX ?? "!";
  }

  abstract get name(): string;
  abstract get commands(): CommandDef[];

  abstract onMessage(ctx: MessageContext): Promise<void>;

  async onReaction(_ctx: ReactionContext): Promise<void> {
    // Default no-op; plugins override if they care about reactions
  }

  protected async sendMessage(roomId: string, text: string): Promise<string> {
    try {
      return await this.client.sendText(roomId, text);
    } catch (err) {
      logger.error(`Failed to send message to ${roomId}: ${err}`);
      throw err;
    }
  }

  protected async sendHtml(roomId: string, html: string, plain?: string): Promise<string> {
    try {
      return await this.client.sendMessage(roomId, {
        msgtype: "m.text",
        body: plain ?? html.replace(/<[^>]+>/g, ""),
        format: "org.matrix.custom.html",
        formatted_body: html,
      });
    } catch (err) {
      logger.error(`Failed to send HTML message to ${roomId}: ${err}`);
      throw err;
    }
  }

  protected async sendReply(roomId: string, eventId: string, text: string): Promise<string> {
    try {
      return await this.client.sendMessage(roomId, {
        msgtype: "m.text",
        body: text,
        "m.relates_to": {
          "m.in_reply_to": { event_id: eventId },
        },
      });
    } catch (err) {
      logger.error(`Failed to send reply in ${roomId}: ${err}`);
      throw err;
    }
  }

  protected async sendDm(userId: string, text: string): Promise<void> {
    try {
      const dmRoomId = await this.client.dms.getOrCreateDm(userId);
      await this.client.sendText(dmRoomId, text);
    } catch (err) {
      logger.error(`Failed to send DM to ${userId}: ${err}`);
    }
  }

  protected isCommand(body: string, command: string): boolean {
    return body.startsWith(this.prefix + command);
  }

  protected getArgs(body: string, command: string): string {
    return body.slice(this.prefix.length + command.length).trim();
  }

  protected isAdmin(userId: string): boolean {
    const admins = (process.env.BOT_ADMIN_USERS ?? "").split(",").map((s) => s.trim());
    return admins.includes(userId);
  }
}

export class PluginRegistry {
  private plugins: Plugin[] = [];
  private botUserId: string;

  constructor(botUserId: string) {
    this.botUserId = botUserId;
  }

  register(plugin: Plugin): void {
    this.plugins.push(plugin);
    logger.info(`Registered plugin: ${plugin.name} (${plugin.commands.map((c) => c.name).join(", ") || "passive"})`);
  }

  getCommands(): { plugin: string; commands: CommandDef[] }[] {
    return this.plugins
      .filter((p) => p.commands.length > 0)
      .map((p) => ({ plugin: p.name, commands: p.commands }));
  }

  async dispatch(roomId: string, event: any): Promise<void> {
    if (event.type !== "m.room.message") return;
    const content = event.content;
    if (!content || content.msgtype !== "m.text") return;

    const sender: string = event.sender;
    if (sender === this.botUserId) return;

    const body: string = content.body ?? "";
    const eventId: string = event.event_id;

    const ctx: MessageContext = { roomId, sender, body, eventId, event };

    for (const plugin of this.plugins) {
      try {
        await plugin.onMessage(ctx);
      } catch (err) {
        logger.error(`Plugin ${plugin.name} error on message: ${err}`);
      }
    }
  }

  async dispatchReaction(roomId: string, event: any): Promise<void> {
    if (event.type !== "m.reaction") return;

    const sender: string = event.sender;
    if (sender === this.botUserId) return;

    const relatesTo = event.content?.["m.relates_to"];
    if (!relatesTo) return;

    const ctx: ReactionContext = {
      roomId,
      sender,
      eventId: event.event_id,
      reactionKey: relatesTo.key ?? "",
      targetEventId: relatesTo.event_id ?? "",
      event,
    };

    for (const plugin of this.plugins) {
      try {
        await plugin.onReaction(ctx);
      } catch (err) {
        logger.error(`Plugin ${plugin.name} error on reaction: ${err}`);
      }
    }
  }
}
