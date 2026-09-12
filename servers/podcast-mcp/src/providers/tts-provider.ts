import { Voice, TTSOptions, TTSResult } from "../types";

export interface TTSProvider {
  readonly name: string;
  readonly displayName: string;

  isAvailable(): boolean;
  listVoices(): Promise<Voice[]>;
  synthesize(options: TTSOptions): Promise<TTSResult>;
}

export class TTSProviderRegistry {
  private providers: Map<string, TTSProvider> = new Map();

  register(provider: TTSProvider): void {
    this.providers.set(provider.name, provider);
    console.error(`Registered TTS provider: ${provider.displayName} (available: ${provider.isAvailable()})`);
  }

  get(name: string): TTSProvider | undefined {
    return this.providers.get(name);
  }

  getAvailable(): TTSProvider[] {
    return Array.from(this.providers.values()).filter((p) => p.isAvailable());
  }

  getAll(): TTSProvider[] {
    return Array.from(this.providers.values());
  }

  has(name: string): boolean {
    return this.providers.has(name);
  }
}
