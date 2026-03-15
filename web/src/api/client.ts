export const BASE_URL = import.meta.env.VITE_API_URL || '/api'

let authToken = ''

export function setAuthToken(token: string) {
  authToken = token
}

export function getAuthToken(): string {
  return authToken
}

function authHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { ...extra }
  if (authToken) headers['Authorization'] = `Bearer ${authToken}`
  return headers
}

export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = authHeaders({
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> || {}),
  })

  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers })

  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new Error(body || `API error: ${res.status} ${res.statusText}`)
  }

  return res.json()
}

/** Fetch a binary resource (e.g. check image) and return a blob object URL. */
export async function apiFetchBlobUrl(path: string): Promise<string> {
  const res = await fetch(`${BASE_URL}${path}`, { headers: authHeaders() })
  if (!res.ok) throw new Error(`Image fetch failed: ${res.status}`)
  const blob = await res.blob()
  return URL.createObjectURL(blob)
}
