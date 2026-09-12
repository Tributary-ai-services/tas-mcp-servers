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
import { ProgressReporter } from "./progress-reporter";
import { JobConsumer } from "./job-consumer";
import { processSegment, SegmentResult } from "./segment-processor";

const HEALTH_PORT = parseInt(process.env.HEALTH_PORT || "8092", 10);
const TTS_CONCURRENCY = parseInt(process.env.TTS_CONCURRENCY || "2", 10);
const SEGMENT_SIZE = parseInt(process.env.SEGMENT_SIZE || "12", 10);

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

// Reusable podcast generation function (called by both HTTP handler and Kafka consumer)
// Uses segmented processing: script is split into segments of SEGMENT_SIZE clips,
// each segment is TTS-generated + assembled independently, then stitched at the end.
// This bounds memory to ~SEGMENT_SIZE clips at a time instead of all clips.
async function generatePodcast(params: {
  script: string;
  voice_mapping: Record<string, string>;
  provider?: string;
  title?: string;
  config?: Record<string, any>;
  progressReporter?: ProgressReporter;
  heartbeat?: () => Promise<void>;
}): Promise<PodcastGenerationResult> {
  const startTime = Date.now();
  const providerName = params.provider || "kokoro";
  const provider = providerRegistry.get(providerName);
  if (!provider) {
    throw new Error(`Unknown TTS provider: ${providerName}`);
  }
  if (!provider.isAvailable()) {
    throw new Error(`TTS provider ${providerName} is not configured`);
  }

  // Parse script
  const parsed = parseScript(params.script);
  if (parsed.lines.length === 0) {
    throw new Error("Script contains no dialogue lines");
  }

  const cfg: PodcastConfig = {
    ...DEFAULT_PODCAST_CONFIG,
    silenceGapMs: params.config?.silence_gap_ms ?? DEFAULT_PODCAST_CONFIG.silenceGapMs,
    introFadeDurationMs: params.config?.intro_fade_ms ?? DEFAULT_PODCAST_CONFIG.introFadeDurationMs,
    outroFadeDurationMs: params.config?.outro_fade_ms ?? DEFAULT_PODCAST_CONFIG.outroFadeDurationMs,
    ambientVolume: params.config?.ambient_volume ?? DEFAULT_PODCAST_CONFIG.ambientVolume,
    outputFormat: params.config?.output_format ?? DEFAULT_PODCAST_CONFIG.outputFormat,
    outputBitrate: params.config?.output_bitrate ?? DEFAULT_PODCAST_CONFIG.outputBitrate,
    ttsConcurrency: TTS_CONCURRENCY,
  };

  const totalClips = parsed.lines.length;

  // Split script lines into segments
  const segments: typeof parsed.lines[] = [];
  for (let i = 0; i < parsed.lines.length; i += SEGMENT_SIZE) {
    segments.push(parsed.lines.slice(i, i + SEGMENT_SIZE));
  }

  console.error(`[generatePodcast] ${totalClips} clips split into ${segments.length} segments of ~${SEGMENT_SIZE}`);

  // Report initial progress
  if (params.progressReporter) {
    await params.progressReporter.updateProgress({
      phase: "tts_generating",
      clips_total: totalClips,
      clips_completed: 0,
      message: `Starting: ${totalClips} clips in ${segments.length} segments`,
    });
  }

  // Process segments sequentially — each segment generates TTS, assembles, then frees memory
  const segmentResults: SegmentResult[] = [];
  let globalClipsCompleted = 0;

  for (let i = 0; i < segments.length; i++) {
    const result = await processSegment({
      lines: segments[i],
      segmentIndex: i,
      totalSegments: segments.length,
      voiceMapping: params.voice_mapping,
      provider,
      providerName,
      config: cfg,
      progressReporter: params.progressReporter,
      globalClipOffset: globalClipsCompleted,
      totalClipsOverall: totalClips,
    });

    globalClipsCompleted += result.clipsGenerated;
    segmentResults.push(result);

    console.error(`[generatePodcast] Segment ${i + 1}/${segments.length} done (${globalClipsCompleted}/${totalClips} clips)`);

    // Send Kafka heartbeat between segments to prevent consumer group eviction
    if (params.heartbeat) {
      try { await params.heartbeat(); } catch (_) {}
    }
  }

  // Report stitching phase
  if (params.progressReporter) {
    await params.progressReporter.updateProgress({
      phase: "assembling",
      clips_total: totalClips,
      clips_completed: globalClipsCompleted,
      message: `Stitching ${segments.length} segments with intro/outro...`,
    });
  }

  // Download music assets from MinIO if specified
  let introPath: string | undefined;
  let outroPath: string | undefined;
  let ambientPath: string | undefined;
  const musicDir = path.join(process.env.TEMP_DIR || "/tmp/podcast-mcp", "music", uuidv4());
  fs.mkdirSync(musicDir, { recursive: true });

  if (params.config?.intro_music_key) {
    const buf = await minioClient.download(params.config.intro_music_key, minioClient.getAssetsBucket());
    introPath = path.join(musicDir, "intro.mp3");
    fs.writeFileSync(introPath, buf);
  }
  if (params.config?.outro_music_key) {
    const buf = await minioClient.download(params.config.outro_music_key, minioClient.getAssetsBucket());
    outroPath = path.join(musicDir, "outro.mp3");
    fs.writeFileSync(outroPath, buf);
  }
  if (params.config?.ambient_music_key) {
    const buf = await minioClient.download(params.config.ambient_music_key, minioClient.getAssetsBucket());
    ambientPath = path.join(musicDir, "ambient.mp3");
    fs.writeFileSync(ambientPath, buf);
  }

  // Final stitch: assemble segment files (no silence between them — already baked in)
  const finalBuffer = await assembleAudio({
    clipPaths: segmentResults.map((s) => s.localPath),
    introPath,
    outroPath,
    ambientPath,
    config: { ...cfg, silenceGapMs: 0 }, // segments already have internal silence
  });

  // Report uploading phase
  if (params.progressReporter) {
    await params.progressReporter.updateProgress({
      phase: "uploading",
      clips_total: totalClips,
      clips_completed: globalClipsCompleted,
      message: "Uploading final podcast to MinIO...",
    });
  }

  // Upload final podcast
  const titleSlug = (params.title || "podcast").toLowerCase().replace(/[^a-z0-9]+/g, "-");
  const outputKey = `podcasts/${titleSlug}-${uuidv4()}.mp3`;
  const uploaded = await minioClient.upload(outputKey, finalBuffer, "audio/mpeg");

  // Cleanup temp files
  for (const seg of segmentResults) {
    try { fs.unlinkSync(seg.localPath); } catch (_) {}
    // Also cleanup the segment directory
    try { fs.rmSync(path.dirname(seg.localPath), { recursive: true, force: true }); } catch (_) {}
  }
  try { fs.rmSync(musicDir, { recursive: true, force: true }); } catch (_) {}

  const totalTimeMs = Date.now() - startTime;
  const durationEstimateMs = Math.round((finalBuffer.length / (192000 / 8)) * 1000);

  return {
    podcast_url: uploaded.url,
    minio_key: uploaded.key,
    duration_estimate_ms: durationEstimateMs,
    total_lines: totalClips,
    speakers: parsed.speakers,
    provider: providerName,
    clips_generated: globalClipsCompleted,
    has_intro: !!introPath,
    has_outro: !!outroPath,
    has_ambient: !!ambientPath,
    total_time_ms: totalTimeMs,
  };
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
        const result = await generatePodcast({
          script: parsed.script,
          voice_mapping: parsed.voice_mapping,
          provider: parsed.provider,
          title: parsed.title,
          config: parsed.config,
        });
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

  // Start Kafka job consumer (non-blocking — logs warning if Kafka unavailable)
  const jobConsumer = new JobConsumer(generatePodcast);
  jobConsumer.start().catch((err) => {
    console.error(`JobConsumer failed to start: ${err.message}`);
  });

  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Podcast MCP server running on stdio");

  // Graceful shutdown
  const shutdown = async () => {
    console.error("Shutting down...");
    await jobConsumer.stop();
    process.exit(0);
  };
  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
