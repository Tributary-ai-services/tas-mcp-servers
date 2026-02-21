import axios, { AxiosInstance } from "axios";
import { TTSProvider } from "./tts-provider";
import { Voice, TTSOptions, TTSResult } from "../types";

const KOKORO_API_URL = process.env.KOKORO_API_URL || "http://kokoro-tts.tas-mcp-servers.svc.cluster.local:8880";

export class KokoroProvider implements TTSProvider {
  readonly name = "kokoro";
  readonly displayName = "Kokoro TTS (Self-hosted)";
  private client: AxiosInstance;
  private apiUrl: string;

  constructor() {
    this.apiUrl = KOKORO_API_URL;
    this.client = axios.create({
      baseURL: this.apiUrl,
      headers: {
        "Content-Type": "application/json",
      },
      timeout: 120000,
    });
  }

  isAvailable(): boolean {
    return this.apiUrl.length > 0;
  }

  async listVoices(): Promise<Voice[]> {
    try {
      const response = await this.client.get("/v1/audio/voices");
      const data = response.data;

      // Kokoro returns { voices: ["af_alloy", "af_bella", ...] } — a flat string array
      const voices = data.voices || data;
      if (!Array.isArray(voices)) {
        return [];
      }

      return voices.map((v: any) => {
        if (typeof v === "string") {
          // Parse voice ID convention: prefix_name
          // af_ = American Female, am_ = American Male, bf_ = British Female, etc.
          const gender = v.match(/^[a-z]([fm])_/) ? (v[1] === "f" ? "female" : "male") : undefined;
          const langPrefix = v.substring(0, 1);
          const langMap: Record<string, string> = {
            a: "en-US", b: "en-GB", e: "es", f: "fr", h: "hi", i: "it", j: "ja", p: "pt", z: "zh",
          };
          const language = langMap[langPrefix] || "en";
          const displayName = v.replace(/^[a-z]{2}_/, "").replace(/_/g, " ");

          return {
            id: v,
            name: displayName.charAt(0).toUpperCase() + displayName.slice(1),
            provider: this.name,
            language,
            gender,
            description: `Kokoro ${v}`,
          };
        }
        return {
          id: v.voice_id || v.id || v.name,
          name: v.name || v.voice_id || v.id,
          provider: this.name,
          language: v.language || "en",
          gender: v.gender,
          preview_url: v.preview_url,
          description: v.description || "Kokoro TTS voice",
        };
      });
    } catch (error: any) {
      console.error(`Kokoro list voices failed: ${error.message}`);
      return [];
    }
  }

  async synthesize(options: TTSOptions): Promise<TTSResult> {
    const response = await this.client.post(
      "/v1/audio/speech",
      {
        model: "kokoro",
        voice: options.voiceId,
        input: options.text,
        response_format: "mp3",
        speed: options.speed ?? 1.0,
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
