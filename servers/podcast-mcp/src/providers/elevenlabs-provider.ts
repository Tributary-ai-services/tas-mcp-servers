import axios, { AxiosInstance } from "axios";
import { TTSProvider } from "./tts-provider";
import { Voice, TTSOptions, TTSResult } from "../types";

const ELEVENLABS_API_URL = process.env.ELEVENLABS_API_URL || "https://api.elevenlabs.io";
const ELEVENLABS_API_KEY = process.env.ELEVENLABS_API_KEY || "";
const ELEVENLABS_MODEL = process.env.ELEVENLABS_MODEL || "eleven_multilingual_v2";

// Default voices available on ElevenLabs free tier (verified 2026-04-01)
const DEFAULT_VOICES: Voice[] = [
  { id: "EXAVITQu4vr4xnSDxMaL", name: "Bella", provider: "elevenlabs", gender: "female", description: "Soft, young, American" },
  { id: "ErXwobaYiN019PkySvjV", name: "Antoni", provider: "elevenlabs", gender: "male", description: "Well-rounded, young, American" },
  { id: "VR6AewLTigWG4xSOukaG", name: "Arnold", provider: "elevenlabs", gender: "male", description: "Crisp, middle-aged, American" },
  { id: "pNInz6obpgDQGcFmaJgB", name: "Adam", provider: "elevenlabs", gender: "male", description: "Deep, middle-aged, American" },
  { id: "onwK4e9ZLuTAKqWW03F9", name: "Daniel", provider: "elevenlabs", gender: "male", description: "Deep, authoritative, British" },
  { id: "Xb7hH8MSUJpSbSDYk0k2", name: "Alice", provider: "elevenlabs", gender: "female", description: "Confident, middle-aged, British" },
];

export class ElevenLabsProvider implements TTSProvider {
  readonly name = "elevenlabs";
  readonly displayName = "ElevenLabs";
  private client: AxiosInstance;
  private apiKey: string;

  constructor() {
    this.apiKey = ELEVENLABS_API_KEY;
    this.client = axios.create({
      baseURL: ELEVENLABS_API_URL,
      headers: {
        "xi-api-key": this.apiKey,
        "Content-Type": "application/json",
      },
      timeout: 60000,
    });
  }

  isAvailable(): boolean {
    return this.apiKey.length > 0;
  }

  async listVoices(): Promise<Voice[]> {
    try {
      const response = await this.client.get("/v1/voices");
      const voices = response.data.voices || [];
      return voices.map((v: any) => ({
        id: v.voice_id,
        name: v.name,
        provider: this.name,
        language: v.labels?.language,
        gender: v.labels?.gender,
        preview_url: v.preview_url,
        description: v.labels?.description || v.description,
      }));
    } catch (error: any) {
      // Fall back to default voices if API key lacks voices_read permission
      console.error(`ElevenLabs list voices failed (${error.response?.status}), using defaults`);
      return DEFAULT_VOICES;
    }
  }

  async synthesize(options: TTSOptions): Promise<TTSResult> {
    const response = await this.client.post(
      `/v1/text-to-speech/${options.voiceId}`,
      {
        text: options.text,
        model_id: ELEVENLABS_MODEL,
        voice_settings: {
          stability: options.stability ?? 0.5,
          similarity_boost: options.similarityBoost ?? 0.75,
          speed: options.speed ?? 1.0,
        },
      },
      {
        responseType: "arraybuffer",
        headers: {
          Accept: "audio/mpeg",
        },
      }
    );

    return {
      audioBuffer: Buffer.from(response.data),
      format: "mp3",
    };
  }
}
