import axios, { AxiosInstance } from "axios";
import { TTSProvider } from "./tts-provider";
import { Voice, TTSOptions, TTSResult } from "../types";

const BARK_API_URL = process.env.BARK_API_URL || "";

export class BarkProvider implements TTSProvider {
  readonly name = "bark";
  readonly displayName = "Bark/Coqui (Self-hosted)";
  private client: AxiosInstance;
  private apiUrl: string;

  constructor() {
    this.apiUrl = BARK_API_URL;
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
    const response = await this.client.get("/api/v1/voices");
    const voices = Array.isArray(response.data) ? response.data : response.data.voices || [];
    return voices.map((v: any) => ({
      id: v.id || v.voice_id || v.name,
      name: v.name || v.id,
      provider: this.name,
      language: v.language || "en",
      gender: v.gender,
      description: "Self-hosted TTS voice",
    }));
  }

  async synthesize(options: TTSOptions): Promise<TTSResult> {
    const response = await this.client.post(
      "/api/v1/synthesize",
      {
        voice: options.voiceId,
        text: options.text,
        output_format: "wav",
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
