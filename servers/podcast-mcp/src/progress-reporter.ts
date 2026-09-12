import { Kafka, Producer } from "kafkajs";
import Redis from "ioredis";

const KAFKA_BROKERS = (process.env.KAFKA_BROKERS || "tas-kafka-shared:9092").split(",");
const REDIS_URL = process.env.REDIS_URL || "redis://tas-redis-shared:6379/0";
const KAFKA_TOPIC_PREFIX = process.env.KAFKA_TOPIC_PREFIX || "aether";
const PROGRESS_TOPIC = KAFKA_TOPIC_PREFIX ? `${KAFKA_TOPIC_PREFIX}.podcast.progress` : "podcast.progress";
const WORKER_POD = process.env.HOSTNAME || process.env.POD_NAME || "unknown";

export interface ProgressUpdate {
  phase: string; // tts_generating, assembling, uploading, completed, failed
  clips_total: number;
  clips_completed: number;
  clips_failed?: number;
  message: string;
}

export class ProgressReporter {
  private producer: Producer | null = null;
  private redis: Redis | null = null;
  private productionId: string;
  private kafkaReady = false;

  constructor(productionId: string) {
    this.productionId = productionId;
  }

  async init(): Promise<void> {
    // Initialize Redis
    try {
      this.redis = new Redis(REDIS_URL, {
        maxRetriesPerRequest: 3,
        lazyConnect: true,
        connectTimeout: 5000,
      });
      await this.redis.connect();
      console.error(`[ProgressReporter] Redis connected for production ${this.productionId}`);
    } catch (err: any) {
      console.error(`[ProgressReporter] Redis connection failed: ${err.message}`);
      this.redis = null;
    }

    // Initialize Kafka producer
    try {
      const kafka = new Kafka({
        clientId: `podcast-worker-${WORKER_POD}`,
        brokers: KAFKA_BROKERS,
        connectionTimeout: 5000,
        requestTimeout: 10000,
      });
      this.producer = kafka.producer();
      await this.producer.connect();
      this.kafkaReady = true;
      console.error(`[ProgressReporter] Kafka producer connected`);
    } catch (err: any) {
      console.error(`[ProgressReporter] Kafka producer connection failed: ${err.message}`);
      this.producer = null;
    }
  }

  async updateProgress(update: ProgressUpdate): Promise<void> {
    const now = new Date().toISOString();

    // Update Redis hash
    if (this.redis) {
      try {
        const key = `podcast:progress:${this.productionId}`;
        await this.redis.hmset(key, {
          phase: update.phase,
          clips_total: String(update.clips_total),
          clips_completed: String(update.clips_completed),
          clips_failed: String(update.clips_failed || 0),
          message: update.message,
          worker_pod: WORKER_POD,
          updated_at: now,
        });
        await this.redis.expire(key, 7200); // 2 hour TTL
      } catch (err: any) {
        console.error(`[ProgressReporter] Redis update failed: ${err.message}`);
      }
    }

    // Publish to Kafka
    if (this.kafkaReady && this.producer) {
      try {
        const event = {
          id: `evt_${Date.now()}`,
          type: "podcast.progress.update",
          source: "podcast-mcp",
          subject: this.productionId,
          data: {
            ...update,
            worker_pod: WORKER_POD,
          },
          timestamp: now,
          version: "1.0",
        };

        await this.producer.send({
          topic: PROGRESS_TOPIC,
          messages: [
            {
              key: this.productionId,
              value: JSON.stringify(event),
              headers: {
                "event-type": "podcast.progress.update",
                source: "podcast-mcp",
              },
            },
          ],
        });
      } catch (err: any) {
        console.error(`[ProgressReporter] Kafka publish failed: ${err.message}`);
      }
    }
  }

  async reportCompleted(result: Record<string, any>): Promise<void> {
    // Update Redis
    if (this.redis) {
      try {
        const key = `podcast:progress:${this.productionId}`;
        await this.redis.hmset(key, {
          phase: "completed",
          message: "Podcast generation completed",
          updated_at: new Date().toISOString(),
        });
        await this.redis.expire(key, 7200);
      } catch (err: any) {
        console.error(`[ProgressReporter] Redis completion update failed: ${err.message}`);
      }
    }

    // Publish completion to Kafka
    if (this.kafkaReady && this.producer) {
      try {
        const event = {
          id: `evt_${Date.now()}`,
          type: "podcast.completed",
          source: "podcast-mcp",
          subject: this.productionId,
          data: {
            media_url: result.podcast_url,
            duration: result.duration_estimate_ms,
            segments: result.total_lines,
            provider: result.provider,
            worker_pod: WORKER_POD,
          },
          timestamp: new Date().toISOString(),
          version: "1.0",
        };

        await this.producer.send({
          topic: PROGRESS_TOPIC,
          messages: [
            {
              key: this.productionId,
              value: JSON.stringify(event),
              headers: {
                "event-type": "podcast.completed",
                source: "podcast-mcp",
              },
            },
          ],
        });
      } catch (err: any) {
        console.error(`[ProgressReporter] Kafka completion publish failed: ${err.message}`);
      }
    }
  }

  async reportFailed(error: string, retryCount: number): Promise<void> {
    // Update Redis
    if (this.redis) {
      try {
        const key = `podcast:progress:${this.productionId}`;
        await this.redis.hmset(key, {
          phase: "failed",
          message: error,
          updated_at: new Date().toISOString(),
        });
        await this.redis.expire(key, 7200);
      } catch (err: any) {
        console.error(`[ProgressReporter] Redis failure update failed: ${err.message}`);
      }
    }

    // Publish failure to Kafka
    if (this.kafkaReady && this.producer) {
      try {
        const event = {
          id: `evt_${Date.now()}`,
          type: "podcast.failed",
          source: "podcast-mcp",
          subject: this.productionId,
          data: {
            error,
            retry_count: retryCount,
            worker_pod: WORKER_POD,
          },
          timestamp: new Date().toISOString(),
          version: "1.0",
        };

        await this.producer.send({
          topic: PROGRESS_TOPIC,
          messages: [
            {
              key: this.productionId,
              value: JSON.stringify(event),
              headers: {
                "event-type": "podcast.failed",
                source: "podcast-mcp",
              },
            },
          ],
        });
      } catch (err: any) {
        console.error(`[ProgressReporter] Kafka failure publish failed: ${err.message}`);
      }
    }
  }

  async close(): Promise<void> {
    if (this.producer) {
      try { await this.producer.disconnect(); } catch (_) {}
    }
    if (this.redis) {
      try { this.redis.disconnect(); } catch (_) {}
    }
  }
}
