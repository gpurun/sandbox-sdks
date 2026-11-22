import { resolveBaseUrl, resolveBearerToken } from './auth.js';
import { BoxApiError } from './errors.js';
import type {
  BalloonConfig,
  BalloonHinting,
  BalloonStats,
  BoxApiClientOptions,
  CompatibleTemplate,
  ConvertResult,
  CreateImageRequest,
  CreateSandboxRequest,
  DecideRequest,
  Decision,
  ImageInfo,
  JsonRecord,
  ListSandboxesV2Options,
  ListSandboxesV2Result,
  OverlaybdRootfs,
  Quota,
  RuntimeCapabilities,
  RuntimeSummary,
  Sandbox,
  SandboxInfo,
  SandboxListItem,
  Snapshot,
  StartBalloonHintingRequest,
  Strategy,
  TemplateBuildInfo,
  TemplateV3,
  TemplateWithBuilds,
  UpdateBalloonRequest,
  UpdateBalloonStatsRequest
} from './types.js';

interface RequestResult<T> {
  data: T | undefined;
  headers: Headers;
}

function errorCode(status: number): string {
  if (status === 400) return 'bad_request';
  if (status === 401) return 'unauthorized';
  if (status === 403) return 'forbidden';
  if (status === 404) return 'not_found';
  if (status === 409) return 'conflict';
  if (status === 422) return 'unprocessable_entity';
  if (status >= 500) return 'internal_error';
  return 'http_error';
}

function pascalDecideRequest(req: DecideRequest): JsonRecord {
  const source = req as JsonRecord;
  const payload: JsonRecord = {};
  const copy = (lower: string, upper: string): void => {
    const value = source[lower] ?? source[upper];
    if (value !== undefined) payload[upper] = value;
  };
  copy('sandboxID', 'SandboxID');
  copy('templateID', 'TemplateID');
  copy('imageID', 'ImageID');
  copy('labels', 'Labels');
  copy('annotations', 'Annotations');
  copy('region', 'Region');
  copy('providerHint', 'ProviderHint');

  const requirements = source.requirements ?? source.Requirements;
  if (requirements !== undefined && requirements !== null && typeof requirements === 'object') {
    const raw = requirements as JsonRecord;
    const nested: JsonRecord = {};
    const copyRequirement = (lower: string, upper: string): void => {
      const value = raw[lower] ?? raw[upper];
      if (value !== undefined) nested[upper] = value;
    };
    copyRequirement('minCPU', 'MinCPU');
    copyRequirement('minMemory', 'MinMemory');
    copyRequirement('needGPU', 'NeedGPU');
    copyRequirement('region', 'Region');
    if (Object.keys(nested).length > 0) payload.Requirements = nested;
  }

  return payload;
}

function assertCreateRequest(req: CreateSandboxRequest): void {
  if (req === null || typeof req !== 'object') {
    throw new BoxApiError('create sandbox request is required', 400, 'invalid_argument');
  }
  const hasSource = [
    req.templateID,
    req.imageID,
    req.snapshotID,
    req.rootfs,
    req['image_id'],
    req['snapshot_id']
  ].some((value) => value !== undefined && value !== null && value !== '');
  if (!hasSource) {
    throw new BoxApiError('one of templateID, imageID, snapshotID or rootfs is required', 400, 'invalid_argument');
  }
  if (req.rootfs !== undefined) {
    const rootfs = req.rootfs as OverlaybdRootfs;
    if (
      rootfs === null ||
      typeof rootfs !== 'object' ||
      rootfs.provider !== 'overlaybd' ||
      typeof rootfs.image !== 'string' ||
      rootfs.image.length === 0
    ) {
      throw new BoxApiError('rootfs must be {provider: "overlaybd", image: string}', 400, 'invalid_argument');
    }
  }
}

function segment(value: string): string {
  return encodeURIComponent(value);
}

function toCreateSandboxWire(req: CreateSandboxRequest): JsonRecord {
  const wire: JsonRecord = { ...req };
  if (req.imageID !== undefined && wire['image_id'] === undefined) {
    wire['image_id'] = req.imageID;
    delete wire['imageID'];
  }
  if (req.snapshotID !== undefined && wire['snapshot_id'] === undefined) {
    wire['snapshot_id'] = req.snapshotID;
    delete wire['snapshotID'];
  }
  if (req.runtimeType !== undefined && wire['runtime_type'] === undefined) {
    wire['runtime_type'] = req.runtimeType;
    delete wire['runtimeType'];
  }
  return wire;
}

export class BoxApiClient {
  private readonly baseUrl: string;
  private readonly token: string | undefined;
  private readonly timeoutMs: number | undefined;
  private readonly headers: Record<string, string>;

  constructor(options: BoxApiClientOptions = {}) {
    this.baseUrl = resolveBaseUrl(options).replace(/\/+$/, '');
    this.token = resolveBearerToken(options);
    this.timeoutMs = options.timeoutMs;
    this.headers = { ...options.headers };
  }

  private async requestRaw<T>(
    method: string,
    path: string,
    options: { body?: unknown; query?: URLSearchParams } = {}
  ): Promise<RequestResult<T>> {
    const target = new URL(this.baseUrl + path);
    if (options.query) target.search = options.query.toString();

    const headers: Record<string, string> = { Accept: 'application/json', ...this.headers };
    let body: string | undefined;
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify(options.body);
    }
    if (this.token) headers.Authorization = `Bearer ${this.token}`;

    const signal = this.timeoutMs !== undefined && this.timeoutMs > 0 ? AbortSignal.timeout(this.timeoutMs) : undefined;
    let response: Response;
    try {
      response = await fetch(target, { method, headers, body, signal });
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause);
      throw new BoxApiError(message, 0, 'network_error');
    }

    if (!response.ok) throw await this.toError(response);

    const text = await response.text();
    if (text === '') return { data: undefined, headers: response.headers };
    try {
      return { data: JSON.parse(text) as T, headers: response.headers };
    } catch {
      throw new BoxApiError('invalid JSON response', response.status, 'invalid_response');
    }
  }

  private async request<T>(method: string, path: string, options: { body?: unknown; query?: URLSearchParams } = {}): Promise<T> {
    const result = await this.requestRaw<T>(method, path, options);
    return result.data as T;
  }

  private async toError(response: Response): Promise<BoxApiError> {
    const text = await response.text().catch(() => '');
    let code = errorCode(response.status);
    let message = text || response.statusText || `HTTP ${response.status}`;
    if (text !== '') {
      try {
        const value = JSON.parse(text) as JsonRecord;
        if (typeof value.code === 'string' && value.code !== '') code = value.code;
        if (typeof value.message === 'string' && value.message !== '') message = value.message;
      } catch {
        message = text;
      }
    }
    return new BoxApiError(message, response.status, code);
  }

  health(): Promise<{ status: string }> {
    return this.request('GET', '/health');
  }

  createSandbox(req: CreateSandboxRequest): Promise<Sandbox> {
    assertCreateRequest(req);
    return this.request('POST', '/sandboxes', { body: toCreateSandboxWire(req) });
  }

  listSandboxes(): Promise<SandboxInfo[]> {
    return this.request('GET', '/v1/sandboxes');
  }

  async listSandboxesV2(opts: ListSandboxesV2Options = {}): Promise<ListSandboxesV2Result> {
    const nextToken = opts.nextToken ?? 0;
    let limit = opts.limit ?? 100;
    if (!Number.isFinite(nextToken) || nextToken < 0) {
      throw new BoxApiError('nextToken must be a non-negative number', 400, 'invalid_argument');
    }
    if (!Number.isFinite(limit) || limit < 0) {
      throw new BoxApiError('limit must be a non-negative number', 400, 'invalid_argument');
    }
    if (limit > 1000) limit = 1000;

    const query = new URLSearchParams();
    for (const state of opts.state ?? []) query.append('state', state);
    query.set('nextToken', String(nextToken));
    query.set('limit', String(limit));

    const result = await this.requestRaw<SandboxListItem[] | { items: SandboxListItem[]; nextToken?: string }>('GET', '/sandboxes', { query });
    const headerToken = result.headers.get('x-next-token') ?? undefined;
    const body = result.data;
    const items = Array.isArray(body) ? body : (body?.items ?? []);
    return {
      items,
      nextToken: headerToken !== undefined && headerToken !== '' ? headerToken : Array.isArray(body) ? undefined : body?.nextToken
    };
  }

  getSandbox(id: string): Promise<SandboxInfo> {
    return this.request('GET', `/sandboxes/${segment(id)}`);
  }

  async killSandbox(id: string): Promise<void> {
    await this.request('DELETE', `/sandboxes/${segment(id)}`);
  }

  pauseSandbox(id: string): Promise<Snapshot> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/pause`);
  }

  resumeSandbox(id: string): Promise<SandboxInfo> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/resume`);
  }

  connectSandbox(id: string): Promise<Sandbox> {
    return this.request('POST', `/sandboxes/${segment(id)}/connect`);
  }

  async setSandboxTimeout(id: string): Promise<void> {
    await this.request('POST', `/sandboxes/${segment(id)}/timeout`);
  }

  poweroffSandbox(id: string): Promise<SandboxInfo> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/poweroff`);
  }

  poweronSandbox(id: string): Promise<SandboxInfo> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/poweron`);
  }

  rebootSandbox(id: string): Promise<SandboxInfo> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/reboot`);
  }

  getBalloon(id: string): Promise<BalloonConfig> {
    return this.request('GET', `/v1/sandboxes/${segment(id)}/balloon`);
  }

  updateBalloon(id: string, req: UpdateBalloonRequest): Promise<BalloonConfig> {
    return this.request('PATCH', `/v1/sandboxes/${segment(id)}/balloon`, { body: req });
  }

  getBalloonStats(id: string): Promise<BalloonStats> {
    return this.request('GET', `/v1/sandboxes/${segment(id)}/balloon/statistics`);
  }

  updateBalloonStats(id: string, req: UpdateBalloonStatsRequest): Promise<BalloonConfig> {
    return this.request('PATCH', `/v1/sandboxes/${segment(id)}/balloon/statistics`, { body: req });
  }

  getBalloonHinting(id: string): Promise<BalloonHinting> {
    return this.request('GET', `/v1/sandboxes/${segment(id)}/balloon/hinting`);
  }

  startBalloonHinting(id: string, req: StartBalloonHintingRequest = {}): Promise<BalloonHinting> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/balloon/hinting/start`, { body: req });
  }

  stopBalloonHinting(id: string): Promise<BalloonHinting> {
    return this.request('POST', `/v1/sandboxes/${segment(id)}/balloon/hinting/stop`);
  }

  listTemplates(): Promise<CompatibleTemplate[]> {
    return this.request('GET', '/templates');
  }

  getTemplate(id: string): Promise<TemplateWithBuilds> {
    return this.request('GET', `/templates/${segment(id)}`);
  }

  async deleteTemplate(id: string): Promise<void> {
    await this.request('DELETE', `/templates/${segment(id)}`);
  }

  getTemplateBuildStatus(templateId: string, buildId: string): Promise<TemplateBuildInfo> {
    return this.request('GET', `/templates/${segment(templateId)}/builds/${segment(buildId)}/status`);
  }

  async startTemplateBuildV2(templateId: string, buildId: string, req: JsonRecord): Promise<void> {
    await this.request('POST', `/v2/templates/${segment(templateId)}/builds/${segment(buildId)}`, { body: req });
  }

  createTemplateV3(req: JsonRecord): Promise<TemplateV3> {
    return this.request('POST', '/v3/templates', { body: req });
  }

  convertTemplate(req: JsonRecord): Promise<ConvertResult> {
    return this.request('POST', '/v1/templates/convert', { body: req });
  }

  createImage(req: CreateImageRequest): Promise<ImageInfo> {
    if (req === null || typeof req !== 'object' || typeof req.templateID !== 'string' || req.templateID === '') {
      throw new BoxApiError('templateID is required', 400, 'invalid_argument');
    }
    return this.request('POST', '/v1/images', { body: req });
  }

  listImages(): Promise<{ images: ImageInfo[] }> {
    return this.request('GET', '/v1/images');
  }

  getImage(id: string): Promise<ImageInfo> {
    return this.request('GET', `/v1/images/${segment(id)}`);
  }

  async deleteImage(id: string): Promise<void> {
    await this.request('DELETE', `/v1/images/${segment(id)}`);
  }

  listRuntimes(): Promise<{ runtimes: RuntimeSummary[] }> {
    return this.request('GET', '/v1/runtimes');
  }

  getRuntime(type: string): Promise<RuntimeSummary> {
    return this.request('GET', `/v1/runtimes/${segment(type)}`);
  }

  getRuntimeCapabilities(type: string): Promise<RuntimeCapabilities> {
    return this.request('GET', `/v1/runtimes/${segment(type)}/capabilities`);
  }

  listStrategies(): Promise<{ strategies: Strategy[] }> {
    return this.request('GET', '/v1/scheduler/strategies');
  }

  setStrategy(req: JsonRecord): Promise<Strategy> {
    return this.request('POST', '/v1/scheduler/strategies', { body: req });
  }

  getStrategy(id: string): Promise<Strategy> {
    return this.request('GET', `/v1/scheduler/strategies/${segment(id)}`);
  }

  deleteStrategy(id: string): Promise<{ deleted: string }> {
    return this.request('DELETE', `/v1/scheduler/strategies/${segment(id)}`);
  }

  getDefaultStrategy(): Promise<Strategy> {
    return this.request('GET', '/v1/scheduler/strategies/default');
  }

  async setDefaultStrategy(id: string): Promise<{ defaultStrategy: string }> {
    return this.request('PUT', '/v1/scheduler/strategies/default', { body: { id } });
  }

  getQuotas(): Promise<{ quotas: Quota[] }> {
    return this.request('GET', '/v1/scheduler/quotas');
  }

  updateQuota(req: JsonRecord): Promise<Quota> {
    return this.request('PUT', '/v1/scheduler/quotas', { body: req });
  }

  decide(req: DecideRequest): Promise<Decision> {
    return this.request('POST', '/v1/scheduler/decide', { body: pascalDecideRequest(req) });
  }
}
