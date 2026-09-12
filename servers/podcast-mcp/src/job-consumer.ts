import { Kafka, Consumer, EachMessagePayload } from "kafkajs";
import { ProgressReporter } from "./progress-reporter";

const KAFKA_BROKERS = (process.env.KAFKA_BROKERS || "tas-kafka-shared:9092").split(",");
const CONSUMER_GROUP = process.env.KAFKA_CONSUMER_GROUP || "podcast-workers";
const KAFKA_TOPIC_PREFIX = process.env.KAFKA_TOPIC_PREFIX || "aether";
const JOBS_TOPIC = KAFKA_TOPIC_PREFIX ? `${KAFKA_TOPIC_PREFIX}.podcast.jobs` : "podcast.jobs";
const WORKER_POD = process.env.HOSTNAME || process.env.POD_NAME || "unknown";

// Session timeout must exceed the longest possible job duration.
// kafkajs sends heartbeats on its internal timer, but eachMessage blocks
// the consumer run-loop. We call heartbeat() manually between segments
// so the broker knows we're alive. 5 minutes gives margin for segment gaps.
const SESSION_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes
const HEARTBEAT_INTERVAL_MS = 10_000;      // 10 seconds

export type GeneratePodcastFn = (params: {
  script: string;
  voice_mapping: Record<string, string>;
  provider: string;
  title: string;
  config?: Record<string, any>;
  progressReporter?: ProgressReporter;
  heartbeat?: () => Promise<void>;
}) => Promise<Record<string, any>>;

export class JobConsumer {
  private consumer: Consumer | null = null;
  private generatePodcast: GeneratePodcastFn;
  private running = false;

  constructor(generatePodcast: GeneratePodcastFn) {
    this.generatePodcast = generatePodcast;
  }

  async start(): Promise<void> {
    try {
      const kafka = new Kafka({
        clientId: `podcast-worker-${WORKER_POD}`,
        brokers: KAFKA_BROKERS,
        connectionTimeout: 10000,
        requestTimeout: 30000,
        retry: {
          initialRetryTime: 1000,
          retries: 5,
        },
      });

      this.consumer = kafka.consumer({
        groupId: CONSUMER_GROUP,
        sessionTimeout: SESSION_TIMEOUT_MS,
        heartbeatInterval: HEARTBEAT_INTERVAL_MS,
      });

      await this.consumer.connect();
      await this.consumer.subscribe({ topic: JOBS_TOPIC, fromBeginning: false });

      this.running = true;
      console.error(`[JobConsumer] Connected to Kafka, consuming from ${JOBS_TOPIC} (group: ${CONSUMER_GROUP})`);

      await this.consumer.run({
        eachMessage: async (payload: EachMessagePayload) => {
          await this.handleMessage(payload);
        },
      });
    } catch (err: any) {
      console.error(`[JobConsumer] Failed to start Kafka consumer: ${err.message}`);
      console.error(`[JobConsumer] Podcast generation will only work via HTTP (no job queue)`);
    }
  }

  private async handleMessage(payload: EachMessagePayload): Promise<void> {
    const { message, heartbeat } = payload;
    const value = message.value?.toString();
    if (!value) return;

    let event: any;
    try {
      event = JSON.parse(value);
    } catch (err) {
      console.error(`[JobConsumer] Failed to parse message: ${err}`);
      return;
    }

    const productionId = event.subject || event.data?.production_id;
    const data = event.data || {};

    console.error(`[JobConsumer] Received job for production ${productionId}`);

    const reporter = new ProgressReporter(productionId);
    await reporter.init();

    try {
      // Report initial progress
      await reporter.updateProgress({
        phase: "tts_generating",
        clips_total: 0,
        clips_completed: 0,
        message: "Starting podcast generation...",
      });

      const result = await this.generatePodcast({
        script: data.script,
        voice_mapping: data.voice_mapping || {},
        provider: data.provider || "kokoro",
        title: data.title || "podcast",
        config: data.config || {},
        progressReporter: reporter,
        heartbeat,  // Pass Kafka heartbeat so generatePodcast can call it between segments
      });

      // Report completion
      await reporter.reportCompleted(result);
      console.error(`[JobConsumer] Job completed for production ${productionId}`);
    } catch (err: any) {
      console.error(`[JobConsumer] Job failed for production ${productionId}: ${err.message}`);
      const retryAttempt = data.retry_attempt || 0;
      await reporter.reportFailed(err.message, retryAttempt);
    } finally {
      await reporter.close();
    }
  }

  async stop(): Promise<void> {
    this.running = false;
    if (this.consumer) {
      try {
        await this.consumer.disconnect();
      } catch (_) {}
    }
  }
}
