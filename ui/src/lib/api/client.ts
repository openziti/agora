import { getAgoraConfig } from '../config';
import { getCsrfToken } from '../cookies';

import type { ApiErrorBody } from './types';

export type ApiParams = URLSearchParams | Record<string, string | number | undefined>;

export type ApiRequestOptions = {
  method?: string;
  params?: ApiParams;
  body?: unknown;
  signal?: AbortSignal;
  headers?: HeadersInit;
  suppressUnauthorizedHandler?: boolean;
};

export type UnauthorizedHandler = (error: ApiError) => void;

let unauthorizedHandler: UnauthorizedHandler | undefined;

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly code?: string;

  constructor(status: number, statusText: string, body: unknown) {
    const apiBody = isApiErrorBody(body) ? body : undefined;

    super(apiBody?.message || statusText || `request failed with status ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
    this.code = apiBody?.code;
  }
}

export function setUnauthorizedHandler(handler: UnauthorizedHandler | undefined) {
  unauthorizedHandler = handler;

  return () => {
    if (unauthorizedHandler === handler) {
      unauthorizedHandler = undefined;
    }
  };
}

export function buildApiURL(path: string, params?: ApiParams): string {
  const { apiBasePath } = getAgoraConfig();
  const basePath = apiBasePath.replace(/\/+$/, '');
  const cleanPath = path.startsWith('/') ? path : `/${path}`;
  const query = normalizeParams(params).toString();

  return `${basePath}${cleanPath}${query ? `?${query}` : ''}`;
}

export async function apiRequest<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET';
  const headers = new Headers(options.headers);

  if (options.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  if (!isSafeMethod(method)) {
    headers.set('X-CSRF-Token', getCsrfToken() ?? '');
  }

  const response = await fetch(buildApiURL(path, options.params), {
    method,
    credentials: 'same-origin',
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
  });

  const responseBody = await readResponseBody(response);

  if (!response.ok) {
    const error = new ApiError(response.status, response.statusText, responseBody);

    if (response.status === 401 && !options.suppressUnauthorizedHandler) {
      if (unauthorizedHandler) {
        unauthorizedHandler(error);
      } else {
        console.warn('unauthorized API request', error);
      }
    }

    throw error;
  }

  return responseBody as T;
}

function normalizeParams(params?: ApiParams): URLSearchParams {
  if (!params) {
    return new URLSearchParams();
  }

  if (params instanceof URLSearchParams) {
    return params;
  }

  const search = new URLSearchParams();

  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      search.set(key, String(value));
    }
  });

  return search;
}

function isSafeMethod(method: string): boolean {
  return ['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase());
}

function isApiErrorBody(body: unknown): body is ApiErrorBody {
  return (
    typeof body === 'object' &&
    body !== null &&
    'code' in body &&
    'message' in body &&
    typeof body.code === 'string' &&
    typeof body.message === 'string'
  );
}

async function readResponseBody(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return undefined;
  }

  const text = await response.text();

  if (!text) {
    return undefined;
  }

  const contentType = response.headers.get('Content-Type') ?? '';

  if (!contentType.includes('application/json')) {
    return text;
  }

  return JSON.parse(text);
}
