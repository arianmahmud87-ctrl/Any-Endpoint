export type Role = 'owner' | 'admin' | 'member'
export type Page = 'overview' | 'profiles' | 'keys' | 'activity' | 'playground'

export interface Me {
  user: { id: string; email: string; email_verified: boolean }
  organization: { id: string; role: Role }
  csrf_token: string
}

export interface Profile {
  id: string
  provider: 'codex' | 'claude_code'
  label: string
  status: string
  allowed_models: string[] | null
  created_at: string
  updated_at: string
  worker_status: string
  last_heartbeat: string | null
}

export interface Grant {
  profile_id: string
  provider: string
  model_pattern?: string
  route: string
  requests_per_minute: number
}

export interface ApiKey {
  id: string
  name: string
  prefix: string
  type: 'codex' | 'claude' | 'universal'
  state: string
  expires_at?: string
  last_used_at?: string
  created_at: string
  revoked_at?: string
  grants: Grant[]
}

export interface UsageRow {
  route: string
  model?: string
  status_code: number
  latency_ms?: number
  created_at: string
  key_id?: string
  provider_profile_id?: string
}

export interface Summary {
  profiles: { total: number }
  keys: { active: number }
  workers: { online: number; stale: number }
  last_24_hours: { requests: number; failures: number }
}

export interface PlaygroundResponse {
  id?: string
  object?: string
  model?: string
  choices?: Array<{ message?: { role?: string; content?: string }; finish_reason?: string }>
  output?: unknown
  status?: string
  latency_ms?: number
}

export interface ApiError extends Error { status: number; type?: string }

let csrfToken = ''

export function setCSRFToken(token: string) { csrfToken = token }

export const previewMode = import.meta.env.VITE_PREVIEW_MODE !== 'false'

const demoMe: Me = { user: { id: 'preview-user', email: 'preview@anyendpoint.local', email_verified: true }, organization: { id: 'preview-org', role: 'owner' }, csrf_token: 'preview-csrf' }
let demoProfiles: Profile[] = [
  { id: 'preview-codex', provider: 'codex', label: 'Personal Codex', status: 'pending', allowed_models: ['gpt-6-luna'], created_at: new Date().toISOString(), updated_at: new Date().toISOString(), worker_status: 'not_enrolled', last_heartbeat: null },
  { id: 'preview-claude', provider: 'claude_code', label: 'Claude Workspace', status: 'pending', allowed_models: ['sonnet'], created_at: new Date().toISOString(), updated_at: new Date().toISOString(), worker_status: 'not_enrolled', last_heartbeat: null },
]
let demoKeys: ApiKey[] = []
const demoSummary: Summary = { profiles: { total: 2 }, keys: { active: 0 }, workers: { online: 0, stale: 0 }, last_24_hours: { requests: 0, failures: 0 } }


async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(init.method ?? 'GET') && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(path, { ...init, headers, credentials: 'include' })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = new Error(body?.error?.message ?? `Request failed (${response.status})`) as ApiError
    error.status = response.status
    error.type = body?.error?.type
    throw error
  }
  return body as T
}

export const api = {
  me: () => previewMode ? Promise.resolve(demoMe) : request<Me>('/api/me'),
  logout: () => previewMode ? Promise.resolve({ status: 'preview_only' }) : request<{ status: string }>('/api/auth/logout', { method: 'POST' }),
  profiles: () => previewMode ? Promise.resolve({ data: demoProfiles }) : request<{ data: Profile[] }>('/api/profiles'),
  createProfile: (body: { provider: string; label: string; allowed_models: string[] }) => {
    if (previewMode) {
      const profile: Profile = { id: `preview-${Date.now()}`, provider: body.provider as Profile['provider'], label: body.label, status: 'pending', allowed_models: body.allowed_models, created_at: new Date().toISOString(), updated_at: new Date().toISOString(), worker_status: 'not_enrolled', last_heartbeat: null }
      demoProfiles = [profile, ...demoProfiles]
      demoSummary.profiles.total = demoProfiles.length
      return Promise.resolve({ profile })
    }
    return request<{ profile: Profile }>('/api/profiles', { method: 'POST', body: JSON.stringify(body) })
  },
  profileAction: (id: string, action: 'connect' | 'disable' | 'reconnect') => {
    if (previewMode) {
      const profile = demoProfiles.find(item => item.id === id)
      if (profile && action === 'disable') profile.status = 'disabled'
      if (profile && action === 'reconnect') profile.status = 'pending'
      return Promise.resolve({ enrollment_token: action === 'disable' ? undefined : `enroll_preview_${id}`, expires_at: new Date(Date.now() + 600000).toISOString(), status: action === 'disable' ? 'disabled' : 'connecting' })
    }
    return request<{ enrollment_token?: string; expires_at?: string; status?: string }>(`/api/profiles/${id}/${action}`, { method: 'POST' })
  },
  keys: () => previewMode ? Promise.resolve({ data: demoKeys }) : request<{ data: ApiKey[] }>('/api/keys'),
  createKey: (body: unknown) => {
    if (previewMode) {
      const input = body as { name: string; type: ApiKey['type']; grants: Grant[] }
      const metadata: ApiKey = { id: `preview-key-${Date.now()}`, name: input.name, prefix: 'skv1_preview', type: input.type, state: 'active', created_at: new Date().toISOString(), grants: input.grants }
      demoKeys = [metadata, ...demoKeys]
      demoSummary.keys.active = demoKeys.length
      return Promise.resolve({ key: `skv1_preview_${Math.random().toString(36).slice(2)}_save-this-demo-key`, metadata, warning: 'Preview key only.' })
    }
    return request<{ key: string; metadata: ApiKey; warning: string }>('/api/keys', { method: 'POST', body: JSON.stringify(body) })
  },
  keyAction: (id: string, action: 'rotate' | 'revoke') => {
    if (previewMode) {
      const key = demoKeys.find(item => item.id === id)
      if (key && action === 'revoke') { key.state = 'revoked'; demoSummary.keys.active = demoKeys.filter(item => item.state === 'active').length }
      return Promise.resolve(action === 'revoke' ? { status: 'revoked' } : { key: `skv1_preview_${Math.random().toString(36).slice(2)}_rotated-demo-key`, metadata: key })
    }
    return request<{ key?: string; metadata?: ApiKey; status?: string }>(`/api/keys/${id}/${action}`, { method: 'POST' })
  },
  usage: () => previewMode ? Promise.resolve({ data: [] as UsageRow[] }) : request<{ data: UsageRow[] }>('/api/usage?limit=50'),
  summary: () => previewMode ? Promise.resolve(demoSummary) : request<Summary>('/api/operations/summary'),
  playground: (body: { profile_id: string; model: string; prompt: string }) => previewMode ? Promise.resolve({ id: 'preview-run', model: body.model, choices: [{ message: { role: 'assistant', content: `Preview mode response for ${body.model}.\n\nYour prompt was received locally:\n“${body.prompt}”\n\nDisable VITE_PREVIEW_MODE and connect a worker for real provider execution.` }, finish_reason: 'stop' }] }) : request<PlaygroundResponse>('/api/playground/runs', { method: 'POST', body: JSON.stringify(body) }),
}
