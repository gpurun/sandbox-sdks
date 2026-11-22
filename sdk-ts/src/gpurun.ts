import { BoxApiClient } from './boxapi.js';
import { BoxdClient } from './boxd.js';
import { resolveDataUrl } from './auth.js';
import type { CreateSandboxRequest, ExecResult, GpurunOptions, Sandbox } from './types.js';

export class Gpurun {
  readonly box: BoxApiClient;
  readonly data: BoxdClient;
  readonly sandboxId: string | undefined;

  constructor(options: GpurunOptions = {}) {
    this.sandboxId = options.sandboxId;
    this.box = new BoxApiClient(options);
    this.data = new BoxdClient({
      ...options,
      baseUrl: resolveDataUrl(options.dataUrl)
    });
  }

  createSandbox(req: CreateSandboxRequest): Promise<Sandbox> {
    return this.box.createSandbox(req);
  }

  exec(cmd: string[], envVars?: Record<string, string>, sandboxId?: string): Promise<ExecResult> {
    return this.data.exec(cmd, envVars, sandboxId ?? this.sandboxId);
  }
}
