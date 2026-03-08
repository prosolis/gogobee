import { IMatrixClient, Plugin, CommandDef, MessageContext, PluginRegistry } from "./base";
import { XpPlugin } from "./xp";
import { getDb } from "../db";
import logger from "../utils/logger";

export class WelcomePlugin extends Plugin {
  private xpPlugin: XpPlugin;
  private registry: PluginRegistry;

  constructor(client: IMatrixClient, xpPlugin: XpPlugin, registry: PluginRegistry) {
    super(client);
    this.xpPlugin = xpPlugin;
    this.registry = registry;
  }

  get name() {
    return "welcome";
  }

  get commands(): CommandDef[] {
    return [{ name: "help", description: "Full command list (sent as DM)" }];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    // Passive: welcome new users before command handling
    this.handleWelcome(ctx);

    if (this.isCommand(ctx.body, "help")) {
      await this.handleHelp(ctx);
    }
  }

  private handleWelcome(ctx: MessageContext): void {
    const db = getDb();

    // Check if we've already welcomed this user in ANY room
    const existing = db
      .prepare(`SELECT id FROM achievements WHERE user_id = ? AND achievement_key = 'welcome_wagon'`)
      .get(ctx.sender);

    if (existing) return;

    // New user — insert welcome_wagon achievement (tied to this room for the record)
    db.prepare(`INSERT OR IGNORE INTO achievements (user_id, room_id, achievement_key) VALUES (?, ?, ?)`).run(
      ctx.sender,
      ctx.roomId,
      "welcome_wagon"
    );

    // Grant 25 XP
    this.xpPlugin.grantXp(ctx.sender, ctx.roomId, 25, "welcome");

    // Post welcome message (fire-and-forget)
    const welcomeText =
      `Welcome to the community, ${ctx.sender}!\n\n` +
      `You've got 25 XP to start. Here's a taste of what you can do:\n` +
      `!rank  !hltb <game>  !remindme 1h <msg>  !trivia  !time <city>\n\n` +
      `Type !help for the full command list (sent as a DM). Have fun!`;

    this.sendMessage(ctx.roomId, welcomeText).catch((err) => {
      logger.error(`Failed to send welcome message: ${err}`);
    });

    logger.debug(`Welcomed new user ${ctx.sender} in ${ctx.roomId}`);
  }

  private async handleHelp(ctx: MessageContext): Promise<void> {
    const allCommands = this.registry.getCommands();
    const isAdmin = this.isAdmin(ctx.sender);

    const sections: string[] = [];

    for (const group of allCommands) {
      const visibleCommands = group.commands.filter((cmd) => isAdmin || !cmd.adminOnly);
      if (visibleCommands.length === 0) continue;

      const lines = visibleCommands.map((cmd) => `  !${cmd.name} — ${cmd.description}`);
      sections.push(`[${group.plugin}]\n${lines.join("\n")}`);
    }

    const helpText = `Freebee Commands\n\n${sections.join("\n\n")}`;

    await this.sendDm(ctx.sender, helpText);
    await this.sendReply(ctx.roomId, ctx.eventId, "Command list sent to your DMs!");
  }
}
