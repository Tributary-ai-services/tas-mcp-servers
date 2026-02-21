#!/usr/bin/env node
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import http from "http";
import * as fs from "fs";
import * as path from "path";
import { v4 as uuidv4 } from "uuid";

import {
  ParseScriptSchema,
  GenerateSpeechSchema,
  GeneratePodcastSchema,
  ListVoicesSchema,
  ListProvidersSchema,
  UploadMusicSchema,
  DEFAULT_PODCAST_CONFIG,
  PodcastConfig,
  PodcastGenerationResult,
  ProviderInfo,
} from "./types";
import { TTSProviderRegistry } from "./providers/tts-provider";
import { ElevenLabsProvider } from "./providers/elevenlabs-provider";
import { KokoroProvider } from "./providers/kokoro-provider";
import { MetaVoiceProvider } from "./providers/metavoice-provider";
import { BarkProvider } from "./providers/bark-provider";
import { parseScript } from "./script-parser";
import { assembleAudio, convertToMp3 } from "./audio-assembler";
import { MinioClient } from "./minio-client";

const HEALTH_PORT = parseInt(process.env.HEALTH_PORT || "8092", 10);
const TTS_CONCURRENCY = parseInt(process.env.TTS_CONCURRENCY || "3", 10);

// Create clients
const minioClient = new MinioClient();
const providerRegistry = new TTSProviderRegistry();

// Register providers (auto-available based on env vars)
providerRegistry.register(new KokoroProvider());
providerRegistry.register(new ElevenLabsProvider());
providerRegistry.register(new MetaVoiceProvider());
providerRegistry.register(new BarkProvider());

// Create and configure the MCP server
const server = new Server(
  {
    name: "podcast-mcp",
    version: "1.0.0",
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

server.setRequestHandler(ListToolsRequestSchema, async () => {
  return { tools: getToolsList() };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;
  return handleToolCall(name, args || {});
});

// Helper to read request body
function readBody(req: http.IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString()));
    req.on("error", reject);
  });
}

const TTS_DELAY_MS = parseInt(process.env.TTS_DELAY_MS || "1000", 10);

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Run TTS calls with limited concurrency and delay between calls
async function runWithConcurrency<T>(
  tasks: (() => Promise<T>)[],
  concurrency: number
): Promise<T[]> {
  const results: T[] = new Array(tasks.length);
  let nextIndex = 0;

  async function worker(): Promise<void> {
    while (nextIndex < tasks.length) {
      const idx = nextIndex++;
      results[idx] = await tasks[idx]();
      if (nextIndex < tasks.length && TTS_DELAY_MS > 0) {
        await delay(TTS_DELAY_MS);
      }
    }
  }

  const workers: Promise<void>[] = [];
  for (let i = 0; i < Math.min(concurrency, tasks.length); i++) {
    workers.push(worker());
  }
  await Promise.all(workers);
  return results;
}

// Shared tool handler
async function handleToolCall(
  name: string,
  args: Record<string, any>
): Promise<{ content: Array<{ type: string; text: string }>; isError?: boolean }> {
  try {
    switch (name) {
      case "parse_script": {
        const parsed = ParseScriptSchema.parse(args);
        const result = parseScript(parsed.script);
        return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
      }

      case "generate_speech": {
        const parsed = GenerateSpeechSchema.parse(args);
        const provider = providerRegistry.get(parsed.provider);
        if (!provider) {
          throw new Error(`Unknown TTS provider: ${parsed.provider}`);
        }
        if (!provider.isAvailable()) {
          throw new Error(`TTS provider ${parsed.provider} is not configured (missing API key)`);
        }

        const ttsResult = await provider.synthesize({
          provider: parsed.provider,
          voiceId: parsed.voice_id,
          text: parsed.text,
        });

        // Convert to mp3 if needed
        let audioBuffer = ttsResult.audioBuffer;
        if (ttsResult.format !== "mp3") {
          audioBuffer = await convertToMp3(audioBuffer, ttsResult.format);
        }

        const key = parsed.output_key || `clips/${uuidv4()}.mp3`;
        const uploaded = await minioClient.upload(key, audioBuffer, "audio/mpeg");

        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                url: uploaded.url,
                minio_key: uploaded.key,
                size_bytes: uploaded.size_bytes,
                format: "mp3",
              }, null, 2),
            },
          ],
        };
      }

      case "generate_podcast": {
        const parsed = GeneratePodcastSchema.parse(args);
        const startTime = Date.now();
        const provider = providerRegistry.get(parsed.provider);
        if (!provider) {
          throw new Error(`Unknown TTS provider: ${parsed.provider}`);
        }
        if (!provider.isAvailable()) {
          throw new Error(`TTS provider ${parsed.provider} is not configured (missing API key)`);
        }

        // Parse the script
        const script = parseScript(parsed.script);
        if (script.totalLines === 0) {
          throw new Error("Script contains no dialogue lines");
        }

        // Validate voice mapping
        const missingVoices = script.speakers.filter((s) => !parsed.voice_mapping[s]);
        if (missingVoices.length > 0) {
          throw new Error(`Missing voice mapping for speakers: ${missingVoices.join(", ")}`);
        }

        // Build config
        const podcastConfig: PodcastConfig = {
          ...DEFAULT_PODCAST_CONFIG,
          ...(parsed.config?.silence_gap_ms !== undefined && { silenceGapMs: parsed.config.silence_gap_ms }),
          ...(parsed.config?.intro_fade_ms !== undefined && { introFadeDurationMs: parsed.config.intro_fade_ms }),
          ...(parsed.config?.outro_fade_ms !== undefined && { outroFadeDurationMs: parsed.config.outro_fade_ms }),
          ...(parsed.config?.ambient_volume !== undefined && { ambientVolume: parsed.config.ambient_volume }),
          ...(parsed.config?.output_format !== undefined && { outputFormat: parsed.config.output_format }),
          ...(parsed.config?.output_bitrate !== undefined && { outputBitrate: parsed.config.output_bitrate }),
        };

        // Generate TTS for all lines with concurrency limit
        const tempDir = `/tmp/podcast-mcp/${uuidv4()}`;
        fs.mkdirSync(tempDir, { recursive: true });

        const ttsTasks = script.lines.map((line, idx) => {
          return async () => {
            const result = await provider.synthesize({
              provider: parsed.provider,
              voiceId: parsed.voice_mapping[line.speaker],
              text: line.dialogue,
            });

            let audioBuffer = result.audioBuffer;
            if (result.format !== "mp3") {
              audioBuffer = await convertToMp3(audioBuffer, result.format);
            }

            const clipPath = path.join(tempDir, `clip_${String(idx).padStart(4, "0")}.mp3`);
            fs.writeFileSync(clipPath, audioBuffer);
            return clipPath;
          };
        });

        console.error(`Generating ${ttsTasks.length} TTS clips with concurrency ${TTS_CONCURRENCY}...`);
        const clipPaths = await runWithConcurrency(ttsTasks, TTS_CONCURRENCY);

        // Download music assets if provided
        let introPath: string | undefined;
        let outroPath: string | undefined;
        let ambientPath: string | undefined;

        if (parsed.config?.intro_music_key) {
          const data = await minioClient.download(parsed.config.intro_music_key, minioClient.getAssetsBucket());
          introPath = path.join(tempDir, "intro.mp3");
          fs.writeFileSync(introPath, data);
        }
        if (parsed.config?.outro_music_key) {
          const data = await minioClient.download(parsed.config.outro_music_key, minioClient.getAssetsBucket());
          outroPath = path.join(tempDir, "outro.mp3");
          fs.writeFileSync(outroPath, data);
        }
        if (parsed.config?.ambient_music_key) {
          const data = await minioClient.download(parsed.config.ambient_music_key, minioClient.getAssetsBucket());
          ambientPath = path.join(tempDir, "ambient.mp3");
          fs.writeFileSync(ambientPath, data);
        }

        // Assemble the podcast
        console.error("Assembling podcast audio...");
        const finalAudio = await assembleAudio({
          clipPaths,
          introPath,
          outroPath,
          ambientPath,
          config: podcastConfig,
        });

        // Upload final podcast
        const slug = (parsed.title || "podcast").toLowerCase().replace(/[^a-z0-9]+/g, "-");
        const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
        const podcastKey = `podcasts/${slug}-${timestamp}.mp3`;
        const uploaded = await minioClient.upload(podcastKey, finalAudio, "audio/mpeg");

        // Cleanup temp
        try { fs.rmSync(tempDir, { recursive: true, force: true }); } catch (_) {}

        const result: PodcastGenerationResult = {
          podcast_url: uploaded.url,
          minio_key: uploaded.key,
          duration_estimate_ms: script.totalLines * 3000 + (script.totalLines - 1) * podcastConfig.silenceGapMs,
          total_lines: script.totalLines,
          speakers: script.speakers,
          provider: parsed.provider,
          clips_generated: clipPaths.length,
          has_intro: !!introPath,
          has_outro: !!outroPath,
          has_ambient: !!ambientPath,
          total_time_ms: Date.now() - startTime,
        };

        return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
      }

      case "list_voices": {
        const parsed = ListVoicesSchema.parse(args);
        const provider = providerRegistry.get(parsed.provider);
        if (!provider) {
          throw new Error(`Unknown TTS provider: ${parsed.provider}`);
        }
        if (!provider.isAvailable()) {
          throw new Error(`TTS provider ${parsed.provider} is not configured`);
        }
        const voices = await provider.listVoices();
        return { content: [{ type: "text", text: JSON.stringify({ provider: parsed.provider, voices, total: voices.length }, null, 2) }] };
      }

      case "list_providers": {
        const providers: ProviderInfo[] = providerRegistry.getAll().map((p) => ({
          name: p.name,
          displayName: p.displayName,
          available: p.isAvailable(),
          description: `${p.displayName} TTS provider`,
        }));
        return { content: [{ type: "text", text: JSON.stringify({ providers }, null, 2) }] };
      }

      case "upload_music": {
        const parsed = UploadMusicSchema.parse(args);
        const data = Buffer.from(parsed.data_base64, "base64");
        const ext = parsed.content_type === "audio/wav" ? "wav" : "mp3";
        const key = `${parsed.category}/${parsed.name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}.${ext}`;
        const uploaded = await minioClient.upload(key, data, parsed.content_type, minioClient.getAssetsBucket());
        return {
          content: [
            {
              type: "text",
              text: JSON.stringify({
                message: `Uploaded ${parsed.category} music: ${parsed.name}`,
                minio_key: uploaded.key,
                bucket: uploaded.bucket,
                url: uploaded.url,
                size_bytes: uploaded.size_bytes,
              }, null, 2),
            },
          ],
        };
      }

      default:
        throw new Error(`Unknown tool: ${name}`);
    }
  } catch (error: any) {
    return { content: [{ type: "text", text: `Error: ${error.message}` }], isError: true };
  }
}

function getToolsList() {
  return [
    {
      name: "parse_script",
      description: "Parse a screenplay-format podcast script into structured JSON with speakers, dialogue lines, stage directions, and ambient cues",
      inputSchema: {
        type: "object",
        properties: {
          script: { type: "string", description: "Screenplay-format script text with 'Speaker: dialogue' lines (1-100000 chars)" },
        },
        required: ["script"],
      },
    },
    {
      name: "generate_speech",
      description: "Synthesize a single line of text to speech using the selected TTS provider, upload the audio clip to MinIO",
      inputSchema: {
        type: "object",
        properties: {
          text: { type: "string", description: "Text to synthesize (1-5000 chars)" },
          voice_id: { type: "string", description: "TTS voice ID for the selected provider" },
          provider: { type: "string", description: "TTS provider name (kokoro, elevenlabs, metavoice, bark)", default: "kokoro" },
          output_key: { type: "string", description: "MinIO key for the audio clip (auto-generated if omitted)" },
        },
        required: ["text", "voice_id"],
      },
    },
    {
      name: "generate_podcast",
      description: "End-to-end podcast generation: parse script, synthesize all lines via TTS, assemble with intro/outro music and ambient audio using FFmpeg, upload final MP3 to MinIO",
      inputSchema: {
        type: "object",
        properties: {
          script: { type: "string", description: "Screenplay-format script text" },
          voice_mapping: {
            type: "object",
            description: "Map of speaker names to TTS voice IDs, e.g. {\"Alex\": \"voice_id_1\", \"Sam\": \"voice_id_2\"}",
            additionalProperties: { type: "string" },
          },
          provider: { type: "string", description: "TTS provider name", default: "elevenlabs" },
          title: { type: "string", description: "Podcast title (used for filename)" },
          config: {
            type: "object",
            description: "Production configuration",
            properties: {
              silence_gap_ms: { type: "number", description: "Silence between lines in ms (0-5000)", default: 500 },
              intro_music_key: { type: "string", description: "MinIO key for intro music in podcast-assets bucket" },
              outro_music_key: { type: "string", description: "MinIO key for outro music" },
              ambient_music_key: { type: "string", description: "MinIO key for ambient background music" },
              intro_fade_ms: { type: "number", description: "Intro music fade duration in ms", default: 3000 },
              outro_fade_ms: { type: "number", description: "Outro music fade duration in ms", default: 3000 },
              ambient_volume: { type: "number", description: "Ambient music volume (0-1)", default: 0.15 },
              output_format: { type: "string", enum: ["mp3", "wav", "ogg"], description: "Output format", default: "mp3" },
              output_bitrate: { type: "string", description: "Output bitrate", default: "192k" },
            },
          },
        },
        required: ["script", "voice_mapping"],
      },
    },
    {
      name: "list_voices",
      description: "List available TTS voices for a given provider",
      inputSchema: {
        type: "object",
        properties: {
          provider: { type: "string", description: "TTS provider name (kokoro, elevenlabs, metavoice, bark)", default: "kokoro" },
        },
      },
    },
    {
      name: "list_providers",
      description: "List all registered TTS providers with their availability status",
      inputSchema: {
        type: "object",
        properties: {},
      },
    },
    {
      name: "upload_music",
      description: "Upload intro, outro, or ambient music to the podcast-assets MinIO bucket for use in podcast generation",
      inputSchema: {
        type: "object",
        properties: {
          name: { type: "string", description: "Descriptive name for the music file" },
          category: { type: "string", enum: ["intro", "outro", "ambient"], description: "Music category" },
          data_base64: { type: "string", description: "Base64-encoded audio data" },
          content_type: { type: "string", description: "MIME type of the audio", default: "audio/mpeg" },
        },
        required: ["name", "category", "data_base64"],
      },
    },
  ];
}

// Health check and HTTP API server
function startHealthServer(): void {
  const healthServer = http.createServer(async (req, res) => {
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    res.setHeader("Access-Control-Allow-Headers", "Content-Type");

    if (req.method === "OPTIONS") {
      res.writeHead(204);
      res.end();
      return;
    }

    if (req.url === "/health" && req.method === "GET") {
      const availableProviders = providerRegistry.getAvailable().map((p) => p.name);
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          status: "healthy",
          service: "podcast-mcp",
          version: "1.0.0",
          timestamp: new Date().toISOString(),
          providers: availableProviders,
        })
      );
    } else if (req.url === "/mcp/tools/list" && req.method === "GET") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ tools: getToolsList() }));
    } else if (req.url === "/mcp/tools/call" && req.method === "POST") {
      try {
        const body = await readBody(req);
        const { name, arguments: toolArgs } = JSON.parse(body);
        const result = await handleToolCall(name, toolArgs || {});
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(result));
      } catch (error: any) {
        res.writeHead(400, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: error.message }));
      }
    } else {
      res.writeHead(404);
      res.end();
    }
  });

  healthServer.listen(HEALTH_PORT, () => {
    console.error(`Podcast MCP HTTP server listening on port ${HEALTH_PORT}`);
  });
}

async function main() {
  startHealthServer();

  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Podcast MCP server running on stdio");
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
