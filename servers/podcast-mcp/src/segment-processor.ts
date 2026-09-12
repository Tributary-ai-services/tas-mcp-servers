import * as fs from "fs";
import * as path from "path";
import { v4 as uuidv4 } from "uuid";
import { ScriptLine, PodcastConfig } from "./types";
import { TTSProvider } from "./providers/tts-provider";
import { convertToMp3 } from "./audio-assembler";
import { MinioClient } from "./minio-client";
import { ProgressReporter } from "./progress-reporter";

const TEMP_DIR = process.env.TEMP_DIR || "/tmp/podcast-mcp";
const TTS_DELAY_MS = parseInt(process.env.TTS_DELAY_MS || "1000", 10);

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export interface SegmentResult {
  segmentIndex: number;
  localPath: string;       // local assembled segment file
  clipsGenerated: number;
  clipsFailed: number;
}

export interface SegmentProcessorOptions {
  lines: ScriptLine[];
  segmentIndex: number;
  totalSegments: number;
  voiceMapping: Record<string, string>;
  provider: TTSProvider;
  providerName: string;
  config: PodcastConfig;
  progressReporter?: ProgressReporter;
  globalClipOffset: number;
  totalClipsOverall: number;
}

/**
 * Process a single segment: generate TTS for its clips, concatenate into one file,
 * then release all clip buffers. This bounds memory to ~SEGMENT_SIZE clips at a time.
 */
export async function processSegment(opts: SegmentProcessorOptions): Promise<SegmentResult> {
  const {
    lines, segmentIndex, totalSegments, voiceMapping, provider, providerName,
    config, progressReporter, globalClipOffset, totalClipsOverall,
  } = opts;

  const segDir = path.join(TEMP_DIR, "segments", uuidv4());
  fs.mkdirSync(segDir, { recursive: true });

  const clipPaths: string[] = [];
  let clipsGenerated = 0;
  let clipsFailed = 0;

  try {
    // Generate TTS clips for this segment with concurrency
    const concurrency = config.ttsConcurrency || 2;
    let nextIndex = 0;

    async function worker(): Promise<void> {
      while (nextIndex < lines.length) {
        const idx = nextIndex++;
        const line = lines[idx];
        const voiceId = voiceMapping[line.speaker];
        if (!voiceId) {
          throw new Error(`No voice mapping for speaker: ${line.speaker}`);
        }

        try {
          const ttsResult = await provider.synthesize({
            provider: providerName,
            voiceId,
            text: line.dialogue,
          });

          let audioBuffer = ttsResult.audioBuffer;
          if (ttsResult.format !== "mp3") {
            audioBuffer = await convertToMp3(audioBuffer, ttsResult.format);
          }

          const clipPath = path.join(segDir, `clip_${idx}.mp3`);
          fs.writeFileSync(clipPath, audioBuffer);
          clipPaths[idx] = clipPath;
          clipsGenerated++;

          // Report per-clip progress
          if (progressReporter) {
            const globalCompleted = globalClipOffset + clipsGenerated;
            await progressReporter.updateProgress({
              phase: "tts_generating",
              clips_total: totalClipsOverall,
              clips_completed: globalCompleted,
              clips_failed: clipsFailed,
              message: `Segment ${segmentIndex + 1}/${totalSegments}: clip ${clipsGenerated}/${lines.length} (${line.speaker})`,
            });
          }

          // Delay between TTS calls to avoid overloading Kokoro
          if (nextIndex < lines.length && TTS_DELAY_MS > 0) {
            await delay(TTS_DELAY_MS);
          }
        } catch (err: any) {
          clipsFailed++;
          throw err; // Let the segment fail — will be retried at segment level
        }
      }
    }

    const workers: Promise<void>[] = [];
    for (let i = 0; i < Math.min(concurrency, lines.length); i++) {
      workers.push(worker());
    }
    await Promise.all(workers);

    // Assemble segment clips into one file using FFmpeg concat demuxer
    const segmentOutputPath = path.join(segDir, `segment_${segmentIndex}.mp3`);

    if (clipPaths.filter(Boolean).length === 0) {
      throw new Error(`No clips generated for segment ${segmentIndex}`);
    }

    // Generate silence for between clips
    if (config.silenceGapMs > 0) {
      const silencePath = path.join(segDir, "silence.mp3");
      await generateSegmentSilence(config.silenceGapMs, silencePath);

      // Interleave clips with silence
      const filesToConcat: string[] = [];
      for (let i = 0; i < clipPaths.length; i++) {
        if (clipPaths[i]) {
          filesToConcat.push(clipPaths[i]);
          if (i < clipPaths.length - 1 && clipPaths[i + 1]) {
            filesToConcat.push(silencePath);
          }
        }
      }
      await concatFiles(filesToConcat, segmentOutputPath);
    } else {
      await concatFiles(clipPaths.filter(Boolean), segmentOutputPath);
    }

    return {
      segmentIndex,
      localPath: segmentOutputPath,
      clipsGenerated,
      clipsFailed,
    };
  } finally {
    // Cleanup individual clip files to free disk space (keep segment output)
    for (const clipPath of clipPaths) {
      if (clipPath) {
        try { fs.unlinkSync(clipPath); } catch (_) {}
      }
    }
    // Force GC if available to reclaim Buffer memory between segments
    if (typeof global.gc === "function") {
      global.gc();
    }
  }
}

function generateSegmentSilence(durationMs: number, outputPath: string): Promise<void> {
  const ffmpeg = require("fluent-ffmpeg");
  return new Promise((resolve, reject) => {
    const sampleRate = 44100;
    const channels = 2;
    const bytesPerSample = 2;
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

function concatFiles(files: string[], outputPath: string): Promise<void> {
  const ffmpeg = require("fluent-ffmpeg");
  return new Promise((resolve, reject) => {
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
