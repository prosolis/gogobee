import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";

const COMING_SOON = "This feature is coming soon.";

export class ShadePlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "shade";
  }

  get commands(): CommandDef[] {
    return [
      { name: "shadecheck", description: "(stub) Coming soon", usage: "!shadecheck [@user]" },
      { name: "shadeboard", description: "(stub) Coming soon" },
      { name: "perpetrators", description: "(stub) Coming soon" },
      { name: "receipts", description: "(stub) Coming soon" },
      { name: "shadewar", description: "(stub) Coming soon" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    const stubCommands = ["shadecheck", "shadeboard", "perpetrators", "receipts", "shadewar"];
    for (const cmd of stubCommands) {
      if (this.isCommand(ctx.body, cmd)) {
        await this.sendReply(ctx.roomId, ctx.eventId, COMING_SOON);
        return;
      }
    }
  }
}
