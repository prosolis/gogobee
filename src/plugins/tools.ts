import { IMatrixClient, Plugin, CommandDef, MessageContext } from "./base";
import { evaluate } from "mathjs";
import QRCode from "qrcode";
import logger from "../utils/logger";

export class ToolsPlugin extends Plugin {
  constructor(client: IMatrixClient) {
    super(client);
  }

  get name() {
    return "tools";
  }

  get commands(): CommandDef[] {
    return [
      { name: "calc", description: "Inline calculator", usage: "!calc <expression>" },
      { name: "qr", description: "Generate a QR code image", usage: "!qr <text or URL>" },
    ];
  }

  async onMessage(ctx: MessageContext): Promise<void> {
    if (this.isCommand(ctx.body, "calc")) {
      await this.handleCalc(ctx);
    } else if (this.isCommand(ctx.body, "qr")) {
      await this.handleQr(ctx);
    }
  }

  private async handleCalc(ctx: MessageContext): Promise<void> {
    const expr = this.getArgs(ctx.body, "calc");
    if (!expr) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !calc <expression>");
      return;
    }

    // Normalize natural language patterns
    let normalized = expr
      .replace(/(\d),(\d)/g, "$1$2") // strip thousands commas: 450,000 -> 450000
      .replace(/(\d+(?:\.\d+)?)\s*%\s*of\s*(\d+(?:\.\d+)?)/gi, "($1 / 100) * $2")
      .replace(/(\d+(?:\.\d+)?)\s*%/g, "($1 / 100)");

    try {
      const result = evaluate(normalized);
      const display = typeof result === "number"
        ? Number.isInteger(result) ? result.toLocaleString("en-US") : parseFloat(result.toFixed(10)).toLocaleString("en-US")
        : String(result);
      await this.sendReply(ctx.roomId, ctx.eventId, `${expr} = ${display}`);
    } catch (err) {
      await this.sendReply(ctx.roomId, ctx.eventId, `Could not evaluate: ${expr}`);
    }
  }

  private async handleQr(ctx: MessageContext): Promise<void> {
    const text = this.getArgs(ctx.body, "qr");
    if (!text) {
      await this.sendReply(ctx.roomId, ctx.eventId, "Usage: !qr <text or URL>");
      return;
    }

    try {
      const pngBuffer = await QRCode.toBuffer(text, { type: "png", width: 300, margin: 2 });
      const mxcUrl = await this.client.uploadContent(pngBuffer, "image/png", "qrcode.png");

      await this.client.sendMessage(ctx.roomId, {
        msgtype: "m.image",
        body: "qrcode.png",
        url: mxcUrl,
        info: {
          mimetype: "image/png",
          w: 300,
          h: 300,
          size: pngBuffer.length,
        },
        "m.relates_to": {
          "m.in_reply_to": { event_id: ctx.eventId },
        },
      });
    } catch (err) {
      logger.error(`QR generation failed: ${err}`);
      await this.sendReply(ctx.roomId, ctx.eventId, "Failed to generate QR code.");
    }
  }
}
