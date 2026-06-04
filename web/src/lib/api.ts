const TOKEN_KEY = 'dataseai.token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}
export function setToken(t: string | null) {
  if (t === null) localStorage.removeItem(TOKEN_KEY)
  else localStorage.setItem(TOKEN_KEY, t)
}

export class ApiError extends Error {
  constructor(public status: number, public payload: unknown, message: string) {
    super(message)
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  const t = getToken()
  if (t) headers.set('Authorization', `Bearer ${t}`)
  const res = await fetch(path, { ...init, headers })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const json = text ? JSON.parse(text) : undefined
  if (!res.ok) {
    const msg = (json && (json.error || json.message)) || `HTTP ${res.status}`
    throw new ApiError(res.status, json, msg)
  }
  return json as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) => request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  patch: <T>(path: string, body: unknown) => request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
  deleteWithBody: <T>(path: string, body: unknown) => request<T>(path, { method: 'DELETE', body: JSON.stringify(body) }),
}
