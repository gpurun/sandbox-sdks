export type JsonRecord = Record<string, unknown>;

export interface OverlaybdRootfs {
  provider: 'overlaybd';
  image: string;
  pullMode?: string;
  [key: string]: unknown;
}

export interface RootfsInfo {
  provider: string;
  image?: string;
  pullMode?: string;
  path?: string;
  [key: string]: unknown;
}

export interface CreateSandboxRequest {
  templateID?: string;
  imageID?: string;
  snapshotID?: string;
  runtimeType?: string;
  rootfs?: OverlaybdRootfs;
  timeout?: number;
  metadata?: Record<string, string>;
  envVars?: Record<string, string>;
  [key: string]: unknown;
}

export interface Sandbox {
  alias: string | null;
  clientID: string;
  domain: string | null;
  envdAccessToken?: string;
  envdVersion: string;
  sandboxID: string;
  templateID: string;
  trafficAccessToken: string | null;
  [key: string]: unknown;
}

export interface SandboxInfo {
  sandboxID: string;
  templateID?: string;
  imageID?: string;
  snapshotID?: string;
  state: string;
  runtimeType: string;
  metadata?: Record<string, string>;
  rootfs?: RootfsInfo;
  createdAtUnix?: number;
  updatedAtUnix?: number;
  [key: string]: unknown;
}

export interface SandboxListItem {
  alias?: string | null;
  clientID: string;
  cpuCount: number;
  diskSizeMB: number;
  endAt: string;
  envdVersion: string;
  memoryMB: number;
  metadata: Record<string, string>;
  sandboxID: string;
  startedAt: string;
  state: string;
  templateID: string;
  volumeMounts: unknown[];
  [key: string]: unknown;
}

export interface ListSandboxesV2Options {
  state?: string[];
  nextToken?: number;
  limit?: number;
}

export interface ListSandboxesV2Result {
  items: SandboxListItem[];
  nextToken?: string;
}

export interface Snapshot {
  snapshotID: string;
  sandboxID: string;
  rootfsPath?: string;
  memfilePath?: string;
  snapfilePath?: string;
  createdAtUnix?: number;
  [key: string]: unknown;
}

export interface BalloonConfig {
  amountMiB: number;
  deflateOnOOM: boolean;
  statsPollingIntervalS: number;
  freePageHinting: boolean;
  freePageReporting: boolean;
  [key: string]: unknown;
}

export interface BalloonStats {
  targetMiB: number;
  actualMiB: number;
  swapIn: number;
  swapOut: number;
  majorFaults: number;
  minorFaults: number;
  freeMemory: number;
  totalMemory: number;
  availableMemory: number;
  diskCaches: number;
  hugetlbAllocations: number;
  hugetlbFailures: number;
  sharedMemory: number;
  unevictableMemory: number;
  oomKill: number;
  allocStall: number;
  asyncScan: number;
  directScan: number;
  asyncReclaim: number;
  directReclaim: number;
  [key: string]: unknown;
}

export interface BalloonHinting {
  state: string;
  hostCmd: number;
  guestCmd?: number;
  [key: string]: unknown;
}

export interface UpdateBalloonRequest {
  amountMiB: number;
}

export interface UpdateBalloonStatsRequest {
  statsPollingIntervalS: number;
}

export interface StartBalloonHintingRequest {
  acknowledgeOnStop?: boolean;
}

export interface RuntimeCapabilities {
  startFromImage: boolean;
  startFromTemplate: boolean;
  startFromSnapshot: boolean;
  pause: boolean;
  resume: boolean;
  fullSnapshot: boolean;
  diffSnapshot: boolean;
  gpu: boolean;
  vsock: boolean;
  tapNetwork: boolean;
  hotplugDisk: boolean;
  hotplugNetwork: boolean;
  liveResizeCPU: boolean;
  liveResizeMemory: boolean;
  gracefulShutdown: boolean;
  serialConsole: boolean;
  jailer: boolean;
  [key: string]: unknown;
}

export interface RuntimeSummary {
  runtimeType: string;
  capabilities: RuntimeCapabilities;
  [key: string]: unknown;
}

export interface CompatibleTemplateBuild {
  buildID: string;
  cpuCount: number;
  createdAt: string;
  diskSizeMB?: number;
  envdVersion?: string;
  finishedAt?: string;
  memoryMB: number;
  status: string;
  updatedAt: string;
  [key: string]: unknown;
}

export interface CompatibleTemplate {
  templateID: string;
  buildID: string;
  buildStatus: string;
  aliases: string[];
  names: string[];
  public: boolean;
  spawnCount: number;
  cpuCount: number;
  memoryMB: number;
  diskSizeMB: number;
  envdVersion: string;
  createdAt: string;
  updatedAt: string;
  lastSpawnedAt: string;
  createdBy: string;
  buildCount: number;
  metadata: Record<string, string>;
  [key: string]: unknown;
}

export interface TemplateWithBuilds extends CompatibleTemplate {
  builds: CompatibleTemplateBuild[];
}

export interface TemplateBuildLogEntry {
  level: string;
  message: string;
  timestamp: string;
  step?: string;
  [key: string]: unknown;
}

export interface TemplateBuildInfo {
  buildID: string;
  logEntries: TemplateBuildLogEntry[];
  logs: string[];
  status: string;
  templateID: string;
  reason?: string;
  [key: string]: unknown;
}

export interface TemplateV3 {
  aliases: string[];
  buildID: string;
  metadata?: Record<string, string>;
  names: string[];
  public: boolean;
  tags: string[];
  templateID: string;
  [key: string]: unknown;
}

export interface ConvertResult {
  imageID: string;
  templateID: string;
  rootfsPath: string;
  createdAtUnix: number;
  [key: string]: unknown;
}

export interface ImageInfo {
  imageID: string;
  rootfsPath: string;
  createdAtUnix: number;
  labels?: Record<string, string>;
  [key: string]: unknown;
}

export interface CreateImageRequest {
  templateID: string;
  [key: string]: unknown;
}

export interface Strategy {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  priority: number;
  rules: unknown;
  fallbacks: unknown;
  description: string;
  createdAt: string;
  updatedAt: string;
  [key: string]: unknown;
}

export interface Quota {
  provider: string;
  region: string;
  total: number;
  used: number;
  available: number;
  updatedAt: string;
  [key: string]: unknown;
}

export interface Decision {
  provider: string;
  strategy: string;
  reason: string;
  fallbacks: unknown;
  score: number;
  [key: string]: unknown;
}

export interface DecideRequirements {
  minCPU?: number;
  minMemory?: number;
  needGPU?: boolean;
  region?: string;
  [key: string]: unknown;
}

export interface DecideRequest {
  sandboxID?: string;
  templateID?: string;
  imageID?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  region?: string;
  providerHint?: string;
  requirements?: DecideRequirements;
  [key: string]: unknown;
}

export interface ExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

export interface ProcessInfo {
  pid?: number;
  tag?: string;
  cmd?: string[];
  args?: string[];
  cwd?: string;
  tty?: boolean;
  rows?: number;
  cols?: number;
  [key: string]: unknown;
}

export interface StartProcessRequest {
  cmd: string[];
  cwd?: string;
  env?: Record<string, string>;
  tty?: boolean;
  rows?: number;
  cols?: number;
  tag?: string;
}

export interface EndEvent {
  exitCode: number;
  exited: boolean;
  status: string;
  error?: string;
}

export interface ProcessStreamConfig {
  cmd: string;
  args?: string[];
  envs?: Record<string, string>;
  cwd?: string;
  [key: string]: unknown;
}

export interface ConnectProcessEntry {
  config: ProcessStreamConfig;
  pid: number;
  tag?: string;
  [key: string]: unknown;
}

export interface ProcessSelector {
  pid?: number;
  tag?: string;
  [key: string]: unknown;
}

export interface ProcessSize {
  cols?: number;
  rows?: number;
  [key: string]: unknown;
}

export interface StartProcessStreamRequest {
  process: ProcessStreamConfig;
  pty?: { size?: ProcessSize };
  tag?: string;
  stdin?: boolean;
  [key: string]: unknown;
}

export interface ConnectProcessStreamRequest {
  process: ProcessSelector;
  [key: string]: unknown;
}

export interface UpdateProcessRequest {
  process: ProcessSelector;
  pty?: { size?: ProcessSize };
  [key: string]: unknown;
}

export interface SendInputRequest {
  process: ProcessSelector;
  input: { stdin?: string; pty?: string };
  [key: string]: unknown;
}

export interface SendSignalRequest {
  process: ProcessSelector;
  signal: number;
  [key: string]: unknown;
}

export interface ProcessEvent {
  event: {
    start?: { pid: number; [key: string]: unknown };
    /** stdout/stderr/pty are standard base64 strings (JSON envelope) — decode, e.g. with decodeDataField, before use. */
    data?: { stdout?: string; stderr?: string; pty?: string; [key: string]: unknown };
    keepalive?: JsonRecord;
    end?: EndEvent;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}

export interface ProcessStream {
  waitEnd(): Promise<EndEvent>;
  close(): void;
}

export interface ProcessSocketHandlers {
  onStart?: (process: ProcessInfo) => void;
  onData?: (data: Uint8Array) => void;
  onEnd?: (end: EndEvent) => void;
  onError?: (error: Error) => void;
}

export interface ProcessSocket {
  send(data: Uint8Array | string): void;
  close(): void;
  waitEnd(): Promise<EndEvent>;
}

export interface ClientOptions {
  baseUrl?: string;
  apiKey?: string;
  accessToken?: string;
  token?: string;
  timeoutMs?: number;
  headers?: Record<string, string>;
}

export interface BoxApiClientOptions extends ClientOptions {}

export interface BoxdClientOptions extends ClientOptions {
  sandboxId?: string;
}

export interface GpurunOptions extends BoxdClientOptions {
  dataUrl?: string;
}
