import ffmpeg from "fluent-ffmpeg";
import * as fs from "fs";
import * as path from "path";
import { v4 as uuidv4 } from "uuid";
import { PodcastConfig, DEFAULT_PODCAST_CONFIG } from "./types";

const TEMP_DIR = process.env.TEMP_DIR || "/tmp/podcast-mcp";

function ensureTempDir(): string {
  const dir = path.join(TEMP_DIR, uuidv4());
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

function cleanupDir(dir: string): void {
  try {
    fs.rmSync(dir, { recursive: true, force: true });
  } catch (err) {
    console.error(`Failed to cleanup temp dir ${dir}:`, err);
  }
}

function generateSilence(durationMs: number, outputPath: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const sampleRate = 44100;
    const channels = 2;
    const bytesPerSample = 2; // 16-bit
    const totalSamples = Math.floor((durationMs / 1000) * sampleRate * channels);
    const rawBuffer = Buffer.alloc(totalSamples * bytesPerSample, 0);
    const rawPath = outputPath + ".raw";
    fs.writeFileSync(rawPath, rawBuffer);

    ffmpeg()
      .input(rawPath)
      .inputOptions(["-f", "s16le", "-ar", "44100", "-ac", "2"])
      .audioCodec("libmp3lame")
      .audioBitrate("128k")
      .audioFrequency(44100)
      .audioChannels(2)
      .output(outputPath)
      .on("end", () => {
        try { fs.unlinkSync(rawPath); } catch (_) {}
        resolve();
      })
      .on("error", (err: Error) => {
        try { fs.unlinkSync(rawPath); } catch (_) {}
        reject(err);
      })
      .run();
  });
}

function concatAudioFiles(files: string[], outputPath: string): Promise<void> {
  return new Promise((resolve, reject) => {
    // Use the concat demuxer (file-list based) instead of filter_complex.
    // filter_complex with 100+ inputs causes ffmpeg to hang or OOM.
    const listPath = outputPath + ".list.txt";
    const listContent = files.map((f) => `file '${f.replace(/'/g, "'\\''")}'`).join("\n");
    fs.writeFileSync(listPath, listContent);

    ffmpeg()
      .input(listPath)
      .inputOptions(["-f", "concat", "-safe", "0"])
      .audioCodec("libmp3lame")
      .audioBitrate("192k")
      .audioFrequency(44100)
      .audioChannels(2)
      .output(outputPath)
      .on("end", () => {
        try { fs.unlinkSync(listPath); } catch (_) {}
        resolve();
      })
      .on("error", (err: Error) => {
        try { fs.unlinkSync(listPath); } catch (_) {}
        reject(err);
      })
      .run();
  });
}

function mixWithBackground(
  mainPath: string,
  backgroundPath: string,
  outputPath: string,
  backgroundVolume: number
): Promise<void> {
  return new Promise((resolve, reject) => {
    ffmpeg()
      .input(mainPath)
      .input(backgroundPath)
      .complexFilter(
        `[1:a]volume=${backgroundVolume}[bg];[0:a][bg]amix=inputs=2:duration=first:dropout_transition=3[out]`
      )
      .outputOptions(["-map", "[out]"])
      .audioCodec("libmp3lame")
      .audioBitrate("192k")
      .output(outputPath)
      .on("end", () => resolve())
      .on("error", (err: Error) => reject(err))
      .run();
  });
}

function fadeAudio(
  inputPath: string,
  outputPath: string,
  fadeInMs: number,
  fadeOutMs: number
): Promise<void> {
  return new Promise((resolve, reject) => {
    const filters: string[] = [];
    if (fadeInMs > 0) {
      filters.push(`afade=t=in:st=0:d=${fadeInMs / 1000}`);
    }
    if (fadeOutMs > 0) {
      // Fade out applied relative to end; use a large start time, ffmpeg handles it
      filters.push(`areverse,afade=t=in:st=0:d=${fadeOutMs / 1000},areverse`);
    }

    if (filters.length === 0) {
      fs.copyFileSync(inputPath, outputPath);
      resolve();
      return;
    }

    ffmpeg()
      .input(inputPath)
      .audioFilters(filters)
      .audioCodec("libmp3lame")
      .audioBitrate("192k")
      .output(outputPath)
      .on("end", () => resolve())
      .on("error", (err: Error) => reject(err))
      .run();
  });
}

export interface AssembleOptions {
  clipPaths: string[];
  introPath?: string;
  outroPath?: string;
  ambientPath?: string;
  config: PodcastConfig;
}

export async function assembleAudio(options: AssembleOptions): Promise<Buffer> {
  const workDir = ensureTempDir();
  const config = { ...DEFAULT_PODCAST_CONFIG, ...options.config };

  try {
    const filesToConcat: string[] = [];

    // Generate silence gap
    const silencePath = path.join(workDir, "silence.mp3");
    if (config.silenceGapMs > 0) {
      await generateSilence(config.silenceGapMs, silencePath);
    }

    // Add intro with fade if provided
    if (options.introPath) {
      const fadedIntro = path.join(workDir, "intro_faded.mp3");
      await fadeAudio(options.introPath, fadedIntro, 0, config.introFadeDurationMs);
      filesToConcat.push(fadedIntro);
      if (config.silenceGapMs > 0) {
        filesToConcat.push(silencePath);
      }
    }

    // Interleave clips with silence
    for (let i = 0; i < options.clipPaths.length; i++) {
      filesToConcat.push(options.clipPaths[i]);
      if (i < options.clipPaths.length - 1 && config.silenceGapMs > 0) {
        filesToConcat.push(silencePath);
      }
    }

    // Add outro with fade if provided
    if (options.outroPath) {
      if (config.silenceGapMs > 0) {
        filesToConcat.push(silencePath);
      }
      const fadedOutro = path.join(workDir, "outro_faded.mp3");
      await fadeAudio(options.outroPath, fadedOutro, config.outroFadeDurationMs, 0);
      filesToConcat.push(fadedOutro);
    }

    // Concatenate all clips
    const mainAudioPath = path.join(workDir, "main.mp3");
    if (filesToConcat.length === 1) {
      fs.copyFileSync(filesToConcat[0], mainAudioPath);
    } else {
      await concatAudioFiles(filesToConcat, mainAudioPath);
    }

    // Mix with ambient background if provided
    let finalPath = mainAudioPath;
    if (options.ambientPath) {
      const mixedPath = path.join(workDir, "mixed.mp3");
      await mixWithBackground(mainAudioPath, options.ambientPath, mixedPath, config.ambientVolume);
      finalPath = mixedPath;
    }

    // Read final output
    const result = fs.readFileSync(finalPath);
    return result;
  } finally {
    cleanupDir(workDir);
  }
}

export function convertToMp3(inputBuffer: Buffer, inputFormat: string): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const workDir = ensureTempDir();
    const inputPath = path.join(workDir, `input.${inputFormat}`);
    const outputPath = path.join(workDir, "output.mp3");

    fs.writeFileSync(inputPath, inputBuffer);

    ffmpeg()
      .input(inputPath)
      .audioCodec("libmp3lame")
      .audioBitrate("192k")
      .audioFrequency(44100)
      .audioChannels(2)
      .output(outputPath)
      .on("end", () => {
        const result = fs.readFileSync(outputPath);
        cleanupDir(workDir);
        resolve(result);
      })
      .on("error", (err: Error) => {
        cleanupDir(workDir);
        reject(err);
      })
      .run();
  });
}
