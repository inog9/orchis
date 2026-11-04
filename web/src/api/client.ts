import { apiErrorText } from '../i18n/errors';
export type FieldError = { name: string; message: string; code?: string };

export class ApiError extends Error {
  private readonly serverFields: FieldError[];
  constructor(
    public status: number,
    _title: string,
    _detail: string,
    public errorId?: string,
    public requestId?: string,
    fields: FieldError[] = [],
    public retryAfter?: number,
    public code?: string,
  ) {
    super();
    this.name = 'ApiError';
    this.serverFields = (Array.isArray(fields) ? fields : []).filter(field=>field && typeof field.name === 'string').map(field=>({name:field.name, code:typeof field.code === 'string'?field.code:undefined, message:''}));
  }
  get message(): string { return apiErrorText(this.status,this.code); }
  get title(): string { return apiErrorText(this.status,this.code); }
  get detail(): string { return apiErrorText(this.status,this.code); }
  get fields(): FieldError[] { return this.serverFields.map(field=>({...field,message:apiErrorText(422,field.code??'field_invalid')})); }
}

// Sesi yang berakhir di tengah penggunaan (mis. akun dinonaktifkan): beri tahu aplikasi agar kembali ke login.
// Jalur /auth/ dikecualikan: 401 di sana berarti password salah atau token reset tidak berlaku, bukan sesi berakhir.
export const UNAUTHORIZED_EVENT = 'orchis:unauthorized';

async function request<T>(method: string, path: string, body?: unknown, idempotencyKey?: string): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: {
      'X-Requested-With': 'orchis-web',
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: Record<string, unknown> = {};
  try {
    data = text ? (JSON.parse(text) as Record<string, unknown>) : {};
  } catch {
    // respons non-JSON (misalnya halaman galat dari proxy)
  }
  if (res.status === 401 && !path.startsWith('/api/v1/auth/')) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
  if (!res.ok) {
    throw new ApiError(
      res.status,
      String(data.title ?? res.statusText),
      String(data.detail ?? ''),
      data.error_id as string | undefined,
      data.request_id as string | undefined,
      (data.fields as FieldError[] | undefined) ?? [],
      data.retry_after_seconds as number | undefined,
            typeof data.code === "string" ? data.code : undefined,
    );
  }
  return data as T;
}

export const api = {
  postIdempotent: <T>(path: string, body: unknown, key: string) => request<T>('POST', path, body, key),
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  del: <T = void>(path: string, body?: unknown) => request<T>('DELETE', path, body),
};
