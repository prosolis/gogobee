/**
 * LLM Classifier Worker — STUB
 *
 * This worker thread will handle shade classification via Ollama when the feature ships.
 *
 * Design notes:
 * - Runs in a separate Worker thread to avoid blocking the main event loop
 * - Communicates via MessageChannel (parentPort)
 * - Receives: { type: "classify", id: string, text: string }
 * - Returns: { type: "result", id: string, isShade: boolean, confidence: number }
 * - Uses the model specified by OLLAMA_MODEL env var
 * - Rate limited to prevent overwhelming the LLM host
 *
 * When wiring this up:
 * 1. ShadePlugin creates a Worker pointing to this file
 * 2. On each message, ShadePlugin posts a classify job
 * 3. Worker calls Ollama API and returns the classification
 * 4. ShadePlugin writes results to shade_log table
 */

// import { parentPort } from "worker_threads";
// import { OLLAMA_HOST, OLLAMA_MODEL } from env

// parentPort?.on("message", async (msg) => {
//   if (msg.type === "classify") {
//     const result = await classifyWithOllama(msg.text);
//     parentPort?.postMessage({
//       type: "result",
//       id: msg.id,
//       isShade: result.isShade,
//       confidence: result.confidence,
//     });
//   }
// });

export {};
