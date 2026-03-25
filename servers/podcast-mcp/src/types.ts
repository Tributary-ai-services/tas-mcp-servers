import { z } from "zod";

// --- Script Types ---

export const ScriptLineSchema = z.object({
  speaker: z.string().describe("Speaker name"),
  dialogue: z.string().describe("Spoken dialogue text"),
  direction: z.string().optional().describe("Stage direction, e.g. (laughs), (excited)"),
  ambient: z.string().optional().describe("Ambient cue, e.g. [ambient: coffee shop]"),
  lineNumber: z.number().int().describe("Line number in the script"),
});

export type ScriptLine = z.infer<typeof ScriptLineSchema>;

export interface ParsedScript {
  lines: ScriptLine[];
  speakers: string[];
  totalLines: number;
  ambientCues: string[];
}

// --- Voice & TTS Types ---

export interface Voice {
  id: string;
  name: string;
  provider: string;
  language?: string;
  gender?: string;
  preview_url?: string;
  description?: string;
}

export interface TTSOptions {
  provider: string;
  voiceId: string;
  text: string;
  stability?: number;
  similarityBoost?: number;
  speed?: number;
  outputFormat?: string;
}

export interface TTSResult {
  audioBuffer: Buffer;
  durationMs?: number;
  format: string;
}

// --- Podcast Config ---

export interface PodcastConfig {
  silenceGapMs: number;
  introFadeDurationMs: number;
  outroFadeDurationMs: number;
  ambientVolume: number;
  outputFormat: string;
  outputBitrate: string;
  ttsConcurrency: number;
}

export const DEFAULT_PODCAST_CONFIG: PodcastConfig = {
  silenceGapMs: 500,
  introFadeDurationMs: 3000,
  outroFadeDurationMs: 3000,
  ambientVolume: 0.15,
  outputFormat: "mp3",
  outputBitrate: "192k",
  ttsConcurrency: 3,
};

// --- MCP Tool Input Schemas ---

export const ParseScriptSchema = z.object({
  script: z.string().min(1).max(100000).describe("Screenplay-format script text with 'Speaker: dialogue' lines"),
});

export const GenerateSpeechSchema = z.object({
  text: z.string().min(1).max(5000).describe("Text to synthesize"),
  voice_id: z.string().describe("TTS voice ID for the selected provider"),
  provider: z.string().default("kokoro").describe("TTS provider name (kokoro, elevenlabs, bark)"),
  output_key: z.string().optional().describe("MinIO key for the audio clip (auto-generated if omitted)"),
});

export const GeneratePodcastSchema = z.object({
  script: z.string().min(1).max(100000).describe("Screenplay-format script text"),
  voice_mapping: z.record(z.string(), z.string()).describe("Map of speaker names to TTS voice IDs"),
  provider: z.string().default("kokoro").describe("TTS provider name (kokoro, elevenlabs, bark)"),
  title: z.string().optional().describe("Podcast title (used for filename)"),
  config: z.object({
    silence_gap_ms: z.number().int().min(0).max(5000).optional().describe("Silence between lines in ms"),
    intro_music_key: z.string().optional().describe("MinIO key for intro music in podcast-assets bucket"),
    outro_music_key: z.string().optional().describe("MinIO key for outro music in podcast-assets bucket"),
    ambient_music_key: z.string().optional().describe("MinIO key for ambient background music"),
    intro_fade_ms: z.number().int().optional().describe("Intro music fade duration in ms"),
    outro_fade_ms: z.number().int().optional().describe("Outro music fade duration in ms"),
    ambient_volume: z.number().min(0).max(1).optional().describe("Ambient music volume (0-1)"),
    output_format: z.enum(["mp3", "wav", "ogg"]).optional().describe("Output audio format"),
    output_bitrate: z.string().optional().describe("Output bitrate e.g. 192k"),
  }).optional().describe("Podcast production configuration"),
});

export const ListVoicesSchema = z.object({
  provider: z.string().default("kokoro").describe("TTS provider name (kokoro, elevenlabs, bark)"),
});

export const ListProvidersSchema = z.object({});

export const UploadMusicSchema = z.object({
  name: z.string().describe("Descriptive name for the music file"),
  category: z.enum(["intro", "outro", "ambient"]).describe("Music category"),
  data_base64: z.string().describe("Base64-encoded audio data"),
  content_type: z.string().default("audio/mpeg").describe("MIME type of the audio"),
});

// --- MinIO Types ---

export interface MinioUploadResult {
  bucket: string;
  key: string;
  url: string;
  size_bytes: number;
  etag?: string;
}

export interface MinioListResult {
  key: string;
  size: number;
  last_modified: string;
  etag?: string;
}

// --- Podcast Generation Result ---

export interface PodcastGenerationResult {
  podcast_url: string;
  minio_key: string;
  duration_estimate_ms: number;
  total_lines: number;
  speakers: string[];
  provider: string;
  clips_generated: number;
  has_intro: boolean;
  has_outro: boolean;
  has_ambient: boolean;
  total_time_ms: number;
}

// --- Provider Info ---

export interface ProviderInfo {
  name: string;
  displayName: string;
  available: boolean;
  voiceCount?: number;
  description: string;
}
