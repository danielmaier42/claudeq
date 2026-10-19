// The one way the UI talks to the daemon. A Blob body is sent as-is (an
// upload); anything else is JSON. `cache: 'no-store'` because a list read from a
// cache is exactly the stale page the app must never show.
export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function api<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const raw = body instanceof Blob
  const headers: Record<string, string> = {}
  if (body !== undefined)
    headers['Content-Type'] = raw ? body.type || 'application/octet-stream' : 'application/json'
  const r = await fetch(path, {
    method,
    signal,
    cache: 'no-store',
    headers,
    body: raw ? body : body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!r.ok && r.status !== 204) {
    let msg = r.statusText
    try {
      msg = ((await r.json()) as {error?: string}).error ?? msg
    } catch {
      /* not JSON: keep the status text */
    }
    throw new ApiError(r.status, msg)
  }
  if (r.status === 204) return undefined as T
  const ct = r.headers.get('content-type') ?? ''
  return (ct.includes('json') ? r.json() : r.text()) as Promise<T>
}

export const get = <T>(path: string, signal?: AbortSignal) => api<T>('GET', path, undefined, signal)
export const post = <T = unknown>(path: string, body?: unknown) => api<T>('POST', path, body)
export const put = <T = unknown>(path: string, body?: unknown) => api<T>('PUT', path, body)
export const del = <T = unknown>(path: string) => api<T>('DELETE', path)

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
