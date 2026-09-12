import axios, { AxiosInstance } from "axios";
import { TTSProvider } from "./tts-provider";
import { Voice, TTSOptions, TTSResult } from "../types";

// MetaVoice-1B: 1.2B param model, Apache 2.0 license
// Requires GPU (12GB+ VRAM) — not suitable for CPU-only nodes
// Server: python fam/llm/serving.py --huggingface_repo_id="metavoiceio/metavoice-1B-v0.1"
// API: POST /tts on port 58003
const METAVOICE_API_URL = process.env.METAVOICE_API_URL || "";

// Built-in speaker presets from MetaVoice-1B
const DEFAULT_VOICES: Voice[] = [
  { id: "default", name: "Default", provider: "metavoice", language: "en", description: "MetaVoice default voice" },
  { id: "bria", name: "Bria", provider: "metavoice", language: "en", gender: "female", description: "Female voice" },
  { id: "alex", name: "Alex", provider: "metavoice", language: "en", gender: "male", description: "Male voice" },
];

export class MetaVoiceProvider implements TTSProvider {
  readonly name = "metavoice";
  readonly displayName = "MetaVoice-1B (Self-hosted, GPU)";
  private client: AxiosInstance;
  private apiUrl: string;

  constructor() {
    this.apiUrl = METAVOICE_API_URL;
    this.client = axios.create({
      baseURL: this.apiUrl,
      headers: {
        "Content-Type": "application/json",
      },
      timeout: 180000, // 3 minutes — 1B model is slower than Kokoro
    });
  }

  isAvailable(): boolean {
    return this.apiUrl.length > 0;
  }

  async listVoices(): Promise<Voice[]> {
    try {
      // MetaVoice serving.py exposes /docs but no standard voices endpoint
      // Try common patterns; fall back to built-in presets
      const response = await this.client.get("/voices");
      const voices = Array.isArray(response.data) ? response.data : response.data.voices || [];
      return voices.map((v: any) => ({
        id: v.id || v.voice_id || v.name,
        name: v.name || v.id,
        provider: this.name,
        language: v.language || "en",
        gender: v.gender,
        description: v.description || "MetaVoice-1B voice",
      }));
    } catch {
      return DEFAULT_VOICES;
    }
  }

  async synthesize(options: TTSOptions): Promise<TTSResult> {
    // MetaVoice serving.py API: POST /tts
    // Body: { text, guidance (3.0), top_p (0.95), speaker_ref_path (optional), top_k (200) }
    const response = await this.client.post(
      "/tts",
      {
        text: options.text,
        speaker_ref_path: options.voiceId !== "default" ? options.voiceId : undefined,
        guidance: 3.0,
        top_p: 0.95,
        top_k: 200,
      },
      {
        responseType: "arraybuffer",
        headers: {
          Accept: "audio/wav",
        },
      }
    );

    return {
      audioBuffer: Buffer.from(response.data),
      format: "wav",
    };
  }
}
