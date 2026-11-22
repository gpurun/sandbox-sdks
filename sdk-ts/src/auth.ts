import type { ClientOptions } from './types.js';

function trimmed(value: string | undefined): string | undefined {
  if (typeof value !== 'string') return undefined;
  const result = value.trim();
  return result.length > 0 ? result : undefined;
}

function env(primary: string, fallback?: string): string | undefined {
  return trimmed(process.env[primary]) ?? (fallback !== undefined ? trimmed(process.env[fallback]) : undefined);
}

export function resolveApiKey(options: ClientOptions = {}): string | undefined {
  return trimmed(options.apiKey) ?? env('GPURUN_API_KEY', 'NOVITA_API_KEY');
}

export function resolveAccessToken(options: ClientOptions = {}): string | undefined {
  return trimmed(options.accessToken) ?? trimmed(options.token) ?? env('GPURUN_ACCESS_TOKEN', 'NOVITA_ACCESS_TOKEN');
}

export function resolveBearerToken(options: ClientOptions = {}): string | undefined {
  return resolveApiKey(options) ?? resolveAccessToken(options);
}

export function resolveBaseUrl(options: ClientOptions = {}): string {
  return trimmed(options.baseUrl) ?? env('GPURUN_BOXAPI_URL') ?? 'http://127.0.0.1:8080';
}

export function resolveDataUrl(dataUrl?: string): string {
  return trimmed(dataUrl) ?? env('GPURUN_BOXD_URL') ?? 'http://127.0.0.1:8082';
}

export function resolveDomain(): string | undefined {
  return env('GPURUN_DOMAIN', 'NOVITA_DOMAIN');
}
