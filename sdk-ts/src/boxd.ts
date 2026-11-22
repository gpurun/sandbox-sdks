import { createHash, randomBytes } from 'node:crypto';
import { connect as connectTcp, type Socket } from 'node:net';
import { connect as connectTls } from 'node:tls';
import { resolveBearerToken, resolveDataUrl } from './auth.js';
import { BoxdError } from './errors.js';
import type {
  BoxdClientOptions,
  ConnectProcessEntry,
  ConnectProcessStreamRequest,
  EndEvent,
  ExecResult,
  JsonRecord,
  ProcessEvent,
  ProcessInfo,
  ProcessSelector,
  ProcessSocket,
  ProcessSocketHandlers,
  ProcessStream,
  SendInputRequest,
  SendSignalRequest,
  StartProcessRequest,
  StartProcessStreamRequest,
  UpdateProcessRequest
} from './types.js';

const SANDBOX_HEADER = 'Novita-Sandbox-Id';

interface Deferred<T> {
  promise: Promise<T>;
  resolve(value: T): void;
  reject(error: unknown): void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function recordOf(value: unknown): JsonRecord | undefined {
  return value !== null && typeof value === 'object' ? (value as JsonRecord) : undefined;
}

function firstString(value: JsonRecord | undefined, keys: string[]): string | undefined {
  if (!value) return undefined;
  for (const key of keys) {
    if (typeof value[key] === 'string') return value[key] as string;
  }
  return undefined;
}

function firstNumber(value: JsonRecord | undefined, keys: string[]): number | undefined {
  if (!value) return undefined;
  for (const key of keys) {
    if (typeof value[key] === 'number') return value[key] as number;
  }
  return undefined;
}

function firstBoolean(value: JsonRecord | undefined, keys: string[]): boolean | undefined {
  if (!value) return undefined;
  for (const key of keys) {
    if (typeof value[key] === 'boolean') return value[key] as boolean;
  }
  return undefined;
}

function parseExecResult(text: string, status: number): ExecResult {
  let body: JsonRecord | undefined;
  if (text !== '') {
    try {
      body = recordOf(JSON.parse(text));
    } catch {
      body = undefined;
    }
  }
  return {
    exitCode: firstNumber(body, ['exitCode', 'exit_code', 'code']) ?? (status === 422 ? 1 : 0),
    stdout: firstString(body, ['stdout', 'output']) ?? (body === undefined ? text : ''),
    stderr: firstString(body, ['stderr', 'error']) ?? ''
  };
}

function normalizeEnd(value: Partial<EndEvent> | undefined): EndEvent {
  const end = value ?? {};
  const result: EndEvent = {
    exitCode: typeof end.exitCode === 'number' ? end.exitCode : 0,
    exited: typeof end.exited === 'boolean' ? end.exited : true,
    status: typeof end.status === 'string' ? end.status : ''
  };
  if (typeof end.error === 'string') result.error = end.error;
  return result;
}

const WS_EVENT_TYPES = new Set(['start', 'data', 'end', 'keepalive']);

function probeWsEvent(data: Uint8Array): JsonRecord | undefined {
  let parsed: unknown;
  try {
    parsed = JSON.parse(Buffer.from(data.buffer, data.byteOffset, data.byteLength).toString('utf8'));
  } catch {
    return undefined;
  }
  const record = recordOf(parsed);
  if (record && typeof record.type === 'string' && WS_EVENT_TYPES.has(record.type)) return record;
  return undefined;
}

function concatBytes(left: Uint8Array, right: Uint8Array): Uint8Array {
  const joined = new Uint8Array(left.length + right.length);
  joined.set(left, 0);
  joined.set(right, left.length);
  return joined;
}

function createProcessStream(response: Response, onEvent: (event: ProcessEvent) => void): ProcessStream {
  const end = deferred<EndEvent>();
  void end.promise.catch(() => {});
  let settled = false;
  let closed = false;
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;

  const settleEnd = (value: EndEvent): void => {
    if (settled) return;
    settled = true;
    end.resolve(value);
  };
  const failEnd = (error: unknown): void => {
    if (settled) return;
    settled = true;
    end.reject(error);
  };

  const pump = async (): Promise<void> => {
    if (!response.body) {
      failEnd(new BoxdError('missing Connect response body', 502));
      return;
    }
    reader = response.body.getReader();
    let pending: Uint8Array = new Uint8Array(0);
    try {
      while (!closed) {
        const chunk = await reader.read();
        if (chunk.done) break;
        if (chunk.value) pending = concatBytes(pending, chunk.value);

        while (pending.length >= 5) {
          const flags = pending[0];
          const length = pending[1] * 0x1000000 + pending[2] * 0x10000 + pending[3] * 0x100 + pending[4];
          if (pending.length < 5 + length) break;
          const payload = pending.slice(5, 5 + length);
          pending = pending.slice(5 + length);

          let event: ProcessEvent | undefined;
          if (length > 0) {
            try {
              event = JSON.parse(new TextDecoder().decode(payload)) as ProcessEvent;
            } catch {
              failEnd(new BoxdError('invalid Connect event frame', 502));
              return;
            }
            onEvent(event);
          }
          if ((flags & 0x02) !== 0 || event?.event?.end !== undefined) {
            settleEnd(normalizeEnd(event?.event?.end));
          }
        }
      }
      if (!settled) failEnd(new BoxdError('Connect stream closed before end frame', 502));
    } catch (cause) {
      if (!closed) failEnd(cause instanceof Error ? cause : new BoxdError(String(cause), 502));
    }
  };
  void pump();

  return {
    waitEnd: () => end.promise,
    close: () => {
      closed = true;
      if (reader) void reader.cancel().catch(() => {});
      failEnd(new BoxdError('Connect stream closed by client', 499));
    }
  };
}

function dataBytes(data: unknown, onData: (data: Uint8Array) => void): void {
  if (typeof data === 'string') return;
  if (data instanceof Uint8Array) {
    onData(data);
    return;
  }
  if (data instanceof ArrayBuffer) {
    onData(new Uint8Array(data));
    return;
  }
  if (ArrayBuffer.isView(data)) {
    onData(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
    return;
  }
  if (data instanceof Blob) {
    void data.arrayBuffer().then((buffer) => onData(new Uint8Array(buffer)));
  }
}

function processSelector(req: ProcessSelector | { pid?: number; tag?: string }): ProcessSelector {
  return { ...req };
}

interface RawWebSocketCallbacks {
  onText(text: string): void;
  onBinary(data: Uint8Array): void;
  onClose(error?: Error): void;
}

class RawWebSocket {
  private readonly socket: Socket;
  private readonly key: string;
  private readonly callbacks: RawWebSocketCallbacks;
  private buffer = Buffer.alloc(0);
  private handshakeComplete = false;
  private closed = false;
  private fragmentOpcode = 0;
  private fragments: Buffer[] = [];

  private constructor(socket: Socket, callbacks: RawWebSocketCallbacks) {
    this.socket = socket;
    this.key = randomBytes(16).toString('base64');
    this.callbacks = callbacks;
  }

  static connect(target: URL, headers: Record<string, string>, callbacks: RawWebSocketCallbacks): Promise<RawWebSocket> {
    return new Promise<RawWebSocket>((resolve, reject) => {
      const secure = target.protocol === 'wss:';
      const port = Number(target.port || (secure ? 443 : 80));
      const socket = secure
        ? connectTls({ host: target.hostname, port, servername: target.hostname })
        : connectTcp({ host: target.hostname, port });
      const ws = new RawWebSocket(socket, callbacks);
      let settled = false;

      const failHandshake = (error: Error): void => {
        if (settled) return;
        settled = true;
        reject(error);
      };
      socket.on('error', (error) => failHandshake(new BoxdError(error.message, 502)));
      socket.on('close', () => {
        ws.closed = true;
        callbacks.onClose();
      });

      let handshake = Buffer.alloc(0);
      const onHandshakeData = (chunk: Buffer): void => {
        handshake = Buffer.concat([handshake, chunk]);
        const separator = handshake.indexOf('\r\n\r\n');
        if (separator < 0) return;
        const head = handshake.subarray(0, separator).toString('utf8');
        const rest = handshake.subarray(separator + 4);
        socket.off('data', onHandshakeData);

        const status = head.split('\r\n', 1)[0] ?? '';
        const expected = createHash('sha1')
          .update(ws.key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11')
          .digest('base64');
        const accept = head
          .split('\r\n')
          .find((line) => line.toLowerCase().startsWith('sec-websocket-accept:'))
          ?.split(':')
          .slice(1)
          .join(':')
          .trim();

        if (!status.startsWith('HTTP/1.1 101') && !status.startsWith('HTTP/1.0 101')) {
          failHandshake(new BoxdError(`WebSocket handshake failed: ${status}`, 502));
          socket.destroy();
          return;
        }
        if (accept !== expected) {
          failHandshake(new BoxdError('WebSocket handshake accept mismatch', 502));
          socket.destroy();
          return;
        }

        ws.handshakeComplete = true;
        socket.on('data', (data: Buffer) => ws.handleFrameData(data));
        if (rest.length > 0) ws.handleFrameData(rest);
        if (!settled) {
          settled = true;
          resolve(ws);
        }
      };
      socket.on('data', onHandshakeData);

      const requestPath = `${target.pathname || '/'}${target.search}`;
      let request = `GET ${requestPath} HTTP/1.1\r\nHost: ${target.host}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: ${ws.key}\r\nSec-WebSocket-Version: 13\r\n`;
      for (const [name, value] of Object.entries(headers)) request += `${name}: ${value}\r\n`;
      socket.write(`${request}\r\n`);
    });
  }

  send(data: Uint8Array): void {
    if (this.closed) return;
    const payload = Buffer.from(data.buffer, data.byteOffset, data.byteLength);
    const mask = randomBytes(4);
    const masked = Buffer.alloc(payload.length);
    for (let index = 0; index < payload.length; index += 1) masked[index] = payload[index] ^ mask[index % 4];

    let header: Buffer;
    if (payload.length < 126) {
      header = Buffer.from([0x82, 0x80 | payload.length]);
    } else if (payload.length <= 0xffff) {
      header = Buffer.alloc(4);
      header[0] = 0x82;
      header[1] = 0x80 | 126;
      header.writeUInt16BE(payload.length, 2);
    } else {
      header = Buffer.alloc(10);
      header[0] = 0x82;
      header[1] = 0x80 | 127;
      header.writeBigUInt64BE(BigInt(payload.length), 2);
    }
    this.socket.write(Buffer.concat([header, mask, masked]));
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    const mask = randomBytes(4);
    this.socket.write(Buffer.concat([Buffer.from([0x88, 0x80]), mask]));
    this.socket.end();
  }

  private handleFrameData(chunk: Buffer): void {
    this.buffer = Buffer.concat([this.buffer, chunk]);
    while (this.buffer.length >= 2) {
      const first = this.buffer[0];
      const second = this.buffer[1];
      const fin = (first & 0x80) !== 0;
      const opcode = first & 0x0f;
      const masked = (second & 0x80) !== 0;
      let length = second & 0x7f;
      let offset = 2;
      if (length === 126) {
        if (this.buffer.length < 4) return;
        length = this.buffer.readUInt16BE(2);
        offset = 4;
      } else if (length === 127) {
        if (this.buffer.length < 10) return;
        length = Number(this.buffer.readBigUInt64BE(2));
        offset = 10;
      }
      const maskOffset = offset;
      offset += masked ? 4 : 0;
      if (this.buffer.length < offset + length) return;

      const payload = Buffer.from(this.buffer.subarray(offset, offset + length));
      if (masked) {
        const mask = this.buffer.subarray(maskOffset, maskOffset + 4);
        for (let index = 0; index < payload.length; index += 1) payload[index] = payload[index] ^ mask[index % 4];
      }
      this.buffer = this.buffer.subarray(offset + length);
      this.handleFrame(fin, opcode, payload);
      if (!this.handshakeComplete) return;
    }
  }

  private handleFrame(fin: boolean, opcode: number, payload: Buffer): void {
    if (opcode === 0x08) {
      this.closed = true;
      this.socket.end();
      this.callbacks.onClose();
      return;
    }
    if (opcode === 0x09) {
      this.socket.write(Buffer.concat([Buffer.from([0x8a, 0x80 | payload.length]), randomBytes(4), payload]));
      return;
    }
    if (opcode === 0x0a) return;

    if (opcode === 0x01 || opcode === 0x02) {
      this.fragmentOpcode = opcode;
      this.fragments = [payload];
    } else if (opcode === 0x00) {
      this.fragments.push(payload);
    } else {
      return;
    }
    if (!fin) return;

    const message = Buffer.concat(this.fragments);
    this.fragments = [];
    try {
      if (this.fragmentOpcode === 0x01) this.callbacks.onText(message.toString('utf8'));
      else this.callbacks.onBinary(message);
    } catch (cause) {
      this.callbacks.onClose(cause instanceof Error ? cause : new BoxdError(String(cause), 502));
    }
  }
}

export class BoxdClient {
  private readonly baseUrl: string;
  private readonly sandboxId: string | undefined;
  private readonly token: string | undefined;
  private readonly timeoutMs: number | undefined;
  private readonly headers: Record<string, string>;

  constructor(options: BoxdClientOptions = {}) {
    this.baseUrl = resolveDataUrl(options.baseUrl).replace(/\/+$/, '');
    this.sandboxId = options.sandboxId;
    this.token = resolveBearerToken(options);
    this.timeoutMs = options.timeoutMs;
    this.headers = { ...options.headers };
  }

  private target(path: string, sandboxId?: string, requireSandbox = true): URL {
    const id = (sandboxId ?? this.sandboxId ?? '').trim();
    if (id === '' && requireSandbox) throw new BoxdError('sandboxID is required', 400);
    const target = new URL(this.baseUrl + path);
    if (id !== '') target.searchParams.set('sandboxID', id);
    return target;
  }

  private async raw(
    method: string,
    path: string,
    body: unknown,
    sandboxId: string | undefined,
    contentType = 'application/json',
    requireSandbox = true
  ): Promise<Response> {
    const target = this.target(path, sandboxId, requireSandbox);
    const id = target.searchParams.get('sandboxID');
    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...this.headers,
      'Content-Type': contentType
    };
    if (id !== null && id !== '') headers[SANDBOX_HEADER] = id;
    if (this.token) headers.Authorization = `Bearer ${this.token}`;

    const signal = this.timeoutMs !== undefined && this.timeoutMs > 0 ? AbortSignal.timeout(this.timeoutMs) : undefined;
    try {
      return await fetch(target, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal
      });
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause);
      throw new BoxdError(message, 0);
    }
  }

  private async toError(response: Response, bodyText?: string): Promise<BoxdError> {
    const text = bodyText ?? (await response.text().catch(() => ''));
    let message = text || response.statusText || `HTTP ${response.status}`;
    if (text !== '') {
      try {
        const value = recordOf(JSON.parse(text));
        const nested = recordOf(value?.error);
        message = firstString(value, ['message', 'error']) ?? firstString(nested, ['message']) ?? message;
      } catch {
        message = text;
      }
    }
    return new BoxdError(message, response.status);
  }

  private async json<T>(
    method: string,
    path: string,
    body: unknown,
    sandboxId?: string,
    contentType = 'application/json',
    requireSandbox = true
  ): Promise<T> {
    const response = await this.raw(method, path, body, sandboxId, contentType, requireSandbox);
    if (!response.ok) throw await this.toError(response);
    const text = await response.text();
    if (text === '') return undefined as T;
    try {
      return JSON.parse(text) as T;
    } catch {
      throw new BoxdError('invalid JSON response', response.status);
    }
  }

  health(sandboxId?: string): Promise<{ status: string; service: string; startedAtUnixNano?: number }> {
    return this.json('GET', '/healthz', undefined, sandboxId, 'application/json', false);
  }

  async exec(cmd: string[], envVars?: Record<string, string>, sandboxId?: string): Promise<ExecResult> {
    const response = await this.raw('POST', '/exec', { cmd, envVars }, sandboxId);
    const text = await response.text();
    if (!response.ok && response.status !== 422) throw await this.toError(response, text);
    return parseExecResult(text, response.status);
  }

  async listProcesses(sandboxId?: string): Promise<ProcessInfo[]> {
    const result = await this.json<{ processes?: ProcessInfo[] } | ProcessInfo[]>('GET', '/processes', undefined, sandboxId);
    if (Array.isArray(result)) return result;
    return result?.processes ?? [];
  }

  async startProcess(req: StartProcessRequest, sandboxId?: string): Promise<ProcessInfo> {
    const result = await this.json<{ process?: ProcessInfo } | ProcessInfo>('POST', '/processes', req, sandboxId);
    if (result && 'process' in result && result.process) return result.process as ProcessInfo;
    return result as unknown as ProcessInfo;
  }

  async waitProcess(id: string, sandboxId?: string): Promise<ProcessInfo> {
    const result = await this.json<{ process?: ProcessInfo } | ProcessInfo>('GET', `/processes/${encodeURIComponent(id)}/wait`, undefined, sandboxId);
    if (result && 'process' in result && result.process) return result.process as ProcessInfo;
    return result as unknown as ProcessInfo;
  }

  async resizeProcess(id: string, rows: number, cols: number, sandboxId?: string): Promise<void> {
    await this.json('POST', `/processes/${encodeURIComponent(id)}/resize`, { rows, cols }, sandboxId);
  }

  async signalProcess(id: string, signal: string, sandboxId?: string): Promise<void> {
    await this.json('POST', `/processes/${encodeURIComponent(id)}/signal`, { signal }, sandboxId);
  }

  async connectProcess(id: string, handlers: ProcessSocketHandlers = {}, sandboxId?: string): Promise<ProcessSocket> {
    const target = this.target(`/processes/${encodeURIComponent(id)}/connect`, sandboxId);
    target.protocol = target.protocol === 'https:' ? 'wss:' : 'ws:';

    const end = deferred<EndEvent>();
    void end.promise.catch(() => {});
    let settled = false;
    const settleEnd = (value: EndEvent): void => {
      if (settled) return;
      settled = true;
      end.resolve(value);
    };
    const failEnd = (error: unknown): void => {
      if (settled) return;
      settled = true;
      end.reject(error);
    };
    const reportError = (error: Error): void => {
      handlers.onError?.(error);
      failEnd(error);
    };
    const handleWsEvent = (message: JsonRecord): void => {
      if (message.type === 'start') {
        handlers.onStart?.((recordOf(message.process) ?? message) as ProcessInfo);
        return;
      }
      if (message.type === 'end') {
        const value = normalizeEnd(recordOf(message.exit) as Partial<EndEvent> | undefined);
        settleEnd(value);
        handlers.onEnd?.(value);
      }
    };

    const requestHeaders: Record<string, string> = {
      [SANDBOX_HEADER]: target.searchParams.get('sandboxID') as string,
      ...this.headers
    };
    if (this.token) requestHeaders.Authorization = `Bearer ${this.token}`;

    const socket = await RawWebSocket.connect(target, requestHeaders, {
      onText: (text) => {
        let message: JsonRecord;
        try {
          message = recordOf(JSON.parse(text)) ?? {};
        } catch {
          reportError(new BoxdError('invalid WebSocket JSON frame', 502));
          return;
        }
        handleWsEvent(message);
      },
      onBinary: (bytes) => {
        // boxproxy 会把 TEXT 事件帧改写成 BINARY，先探测 JSON 事件。
        const message = probeWsEvent(bytes);
        if (message !== undefined) {
          handleWsEvent(message);
          return;
        }
        handlers.onData?.(bytes);
      },
      onClose: (error) => failEnd(error ?? new BoxdError('WebSocket closed before end frame', 502))
    });

    return {
      send: (data: Uint8Array | string): void => {
        const bytes = typeof data === 'string' ? new TextEncoder().encode(data) : data;
        socket.send(bytes);
      },
      close: (): void => socket.close(),
      waitEnd: () => end.promise
    };
  }

  async listProcessesConnect(sandboxId?: string): Promise<ConnectProcessEntry[]> {
    const result = await this.json<{ processes: ConnectProcessEntry[] }>(
      'POST',
      '/process.Process/List',
      {},
      sandboxId,
      'application/connect+json'
    );
    return result?.processes ?? [];
  }

  async startProcessStream(
    req: StartProcessStreamRequest,
    onEvent: (event: ProcessEvent) => void = () => {},
    sandboxId?: string
  ): Promise<ProcessStream> {
    const response = await this.raw('POST', '/process.Process/Start', req, sandboxId, 'application/connect+json');
    if (!response.ok) throw await this.toError(response);
    return createProcessStream(response, onEvent);
  }

  async connectProcessStream(
    req: ConnectProcessStreamRequest,
    onEvent: (event: ProcessEvent) => void = () => {},
    sandboxId?: string
  ): Promise<ProcessStream> {
    const response = await this.raw('POST', '/process.Process/Connect', req, sandboxId, 'application/connect+json');
    if (!response.ok) throw await this.toError(response);
    return createProcessStream(response, onEvent);
  }

  async updateProcess(req: UpdateProcessRequest, sandboxId?: string): Promise<void> {
    await this.json('POST', '/process.Process/Update', req, sandboxId, 'application/connect+json');
  }

  async sendInput(req: SendInputRequest, sandboxId?: string): Promise<void> {
    const input = { ...req.input };
    if (input.stdin) input.stdin = Buffer.from(input.stdin, 'utf8').toString('base64');
    if (input.pty) input.pty = Buffer.from(input.pty, 'utf8').toString('base64');
    await this.json('POST', '/process.Process/SendInput', { ...req, input }, sandboxId, 'application/connect+json');
  }

  async sendSignal(req: SendSignalRequest, sandboxId?: string): Promise<void> {
    await this.json('POST', '/process.Process/SendSignal', req, sandboxId, 'application/connect+json');
  }
}

export { processSelector };
