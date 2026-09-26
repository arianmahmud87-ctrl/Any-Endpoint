import { useEffect, useMemo, useState } from 'react'
import { api, previewMode, type ApiError, type ApiKey, type Me, type Page, type Profile, type Summary, type UsageRow, setCSRFToken } from './api'

type Notice = { kind: 'success' | 'error'; text: string } | null

const nav: Array<{ id: Page; label: string; icon: string }> = [
  { id: 'overview', label: 'Overview', icon: '⌂' },
  { id: 'profiles', label: 'Provider profiles', icon: '◈' },
  { id: 'keys', label: 'API keys', icon: '⌘' },
  { id: 'activity', label: 'Activity', icon: '◷' },
  { id: 'playground', label: 'Playground', icon: '✦' },
]

export function App() {
  const [me, setMe] = useState<Me | null>(null)
  const [page, setPage] = useState<Page>('overview')
  const [loading, setLoading] = useState(true)
  const [notice, setNotice] = useState<Notice>(null)

  const loadMe = async () => {
    try {
      const response = await api.me()
      setCSRFToken(response.csrf_token)
      setMe(response)
    } catch (error) {
      if ((error as ApiError).status !== 401) showError(error)
      setMe(null)
    } finally {
      setLoading(false)
    }
  }

  const showError = (error: unknown) => setNotice({ kind: 'error', text: error instanceof Error ? error.message : 'Something went wrong.' })
  const showSuccess = (text: string) => setNotice({ kind: 'success', text })

  useEffect(() => { void loadMe() }, [])
  useEffect(() => { if (notice) { const timer = window.setTimeout(() => setNotice(null), 5000); return () => window.clearTimeout(timer) } }, [notice])

  if (loading) return <div className="boot"><span className="mark">A</span><span>Loading control plane…</span></div>
  if (!me) return <Login />

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark">A</span><span>Any Endpoint</span></div>
      <div className="workspace"><span className="workspace-avatar">{me.user.email[0]?.toUpperCase()}</span><span><small>Workspace</small><strong>Personal workspace</strong></span><span className="chevron">⌄</span></div>
      <nav aria-label="Main navigation">{nav.map(item => <button className={page === item.id ? 'nav-item active' : 'nav-item'} key={item.id} onClick={() => setPage(item.id)}><span>{item.icon}</span>{item.label}{item.id === 'playground' && <em>soon</em>}</button>)}</nav>
      <div className="sidebar-bottom"><div className="secure-note"><span className="status-dot" />Private control plane</div><button className="account" onClick={async () => { if (previewMode) { showSuccess('Preview mode is enabled. Login remains bypassed locally.') } else { try { await api.logout(); setMe(null) } catch (error) { showError(error) } } }}><span className="avatar">{me.user.email[0]?.toUpperCase()}</span><span><strong>{me.user.email}</strong><small>{me.organization.role}</small></span><span className="more">•••</span></button></div>
    </aside>
    <main className="main-content">
      <header className="topbar"><div><p className="eyebrow">CONTROL PLANE / {page.toUpperCase()}</p><h1>{pageTitle(page)}</h1></div><div className="top-actions"><span className="live-pill"><span className="status-dot" />System operational</span><button className="icon-button" aria-label="Notifications">♢</button></div></header>
      {notice && <div className={`notice ${notice.kind}`} role="status">{notice.kind === 'success' ? '✓' : '!'} {notice.text}</div>}
      {page === 'overview' && <Overview onNavigate={setPage} onError={showError} />}
      {page === 'profiles' && <Profiles role={me.organization.role} onError={showError} onSuccess={showSuccess} />}
      {page === 'keys' && <Keys role={me.organization.role} onError={showError} onSuccess={showSuccess} />}
      {page === 'activity' && <Activity onError={showError} />}
      {page === 'playground' && <Playground />}
    </main>
  </div>
}

function pageTitle(page: Page) { return ({ overview: 'Overview', profiles: 'Provider profiles', keys: 'API keys', activity: 'Activity', playground: 'AI playground' })[page] }
function Login() { return <div className="login-page"><div className="login-card"><span className="brand-mark large">A</span><p className="eyebrow">ANY ENDPOINT</p><h1>One calm place for your AI infrastructure.</h1><p className="muted">Connect your authorized Codex and Claude Code profiles. Your provider credentials stay in isolated worker runtimes.</p><button className="google-button" onClick={() => { window.location.assign('/api/auth/google/start') }}><span className="google-g">G</span> Continue with Google <span>→</span></button><small className="login-foot">Google OAuth only · Secure session · No credentials in browser</small></div></div> }

function Overview({ onNavigate, onError }: { onNavigate: (p: Page) => void; onError: (e: unknown) => void }) {
  const [summary, setSummary] = useState<Summary | null>(null); const [profiles, setProfiles] = useState<Profile[]>([]); const [usage, setUsage] = useState<UsageRow[]>([])
  useEffect(() => { Promise.all([api.summary(), api.profiles(), api.usage()]).then(([s, p, u]) => { setSummary(s); setProfiles(p.data); setUsage(u.data) }).catch(onError) }, [onError])
  const failureRate = summary && summary.last_24_hours.requests ? Math.round(summary.last_24_hours.failures / summary.last_24_hours.requests * 100) : 0
  return <div className="page-stack"><section className="hero"><div><span className="gradient-label">YOUR AI INFRASTRUCTURE</span><h2>Good morning<span className="accent">.</span></h2><p className="muted">Everything is connected, scoped, and ready when you are.</p></div><button className="primary" onClick={() => onNavigate('profiles')}>+ Add provider profile</button></section><section className="metric-grid"><Metric label="Active profiles" value={summary?.profiles.total ?? '—'} detail={`${profiles.filter(p => p.worker_status === 'online').length} online`} icon="◈" /><Metric label="Active API keys" value={summary?.keys.active ?? '—'} detail="All scopes healthy" icon="⌘" /><Metric label="Requests · 24h" value={summary?.last_24_hours.requests ?? '—'} detail={`${failureRate}% failure rate`} icon="↗" tone={failureRate > 5 ? 'warn' : undefined} /><Metric label="Workers online" value={summary?.workers.online ?? '—'} detail={summary?.workers.stale ? `${summary.workers.stale} need attention` : 'No issues detected'} icon="◉" tone={summary?.workers.stale ? 'warn' : undefined} /></section><div className="two-col"><section className="panel"><PanelHeader title="Provider health" action="View all" onClick={() => onNavigate('profiles')} />{profiles.length === 0 ? <Empty text="No provider profiles yet." action="Connect your first profile" onClick={() => onNavigate('profiles')} /> : <div className="health-list">{profiles.slice(0, 4).map(p => <div className="health-row" key={p.id}><ProviderIcon provider={p.provider} /><div className="row-main"><strong>{p.label}</strong><span>{p.provider === 'claude_code' ? 'Claude Code' : 'Codex'} · {p.worker_status}</span></div><Status status={p.status === 'disabled' ? 'disabled' : p.worker_status === 'online' ? 'online' : 'pending'} /></div>)}</div>}</section><section className="panel"><PanelHeader title="Recent requests" action="View activity" onClick={() => onNavigate('activity')} />{usage.length === 0 ? <Empty text="No requests recorded yet." /> : <div className="request-list">{usage.slice(0, 5).map((u, i) => <div className="request-row" key={`${u.created_at}-${i}`}><span className={u.status_code >= 400 ? 'request-code error' : 'request-code'}>{u.status_code}</span><div className="row-main"><strong>{u.model || u.route}</strong><span>{u.route} · {formatRelative(u.created_at)}</span></div><span className="latency">{u.latency_ms || 0}ms</span></div>)}</div>}</section></div></div>
}
function Metric({ label, value, detail, icon, tone }: { label: string; value: string | number; detail: string; icon: string; tone?: 'warn' }) { return <div className="metric"><div className="metric-top"><span>{label}</span><b className={tone ? 'metric-icon warn' : 'metric-icon'}>{icon}</b></div><strong className="metric-value">{value}</strong><small className={tone ? 'warn-text' : ''}>{detail}</small></div> }
function PanelHeader({ title, action, onClick }: { title: string; action?: string; onClick?: () => void }) { return <div className="panel-header"><h3>{title}</h3>{action && <button className="text-button" onClick={onClick}>{action} →</button>}</div> }
function Empty({ text, action, onClick }: { text: string; action?: string; onClick?: () => void }) { return <div className="empty"><span className="empty-icon">＋</span><p>{text}</p>{action && <button className="text-button" onClick={onClick}>{action} →</button>}</div> }
function ProviderIcon({ provider }: { provider: string }) { return <span className={provider === 'codex' ? 'provider-icon codex' : 'provider-icon claude'}>{provider === 'codex' ? 'C' : '✳'}</span> }
function Status({ status }: { status: string }) { const label = status === 'online' ? 'Online' : status === 'disabled' ? 'Disabled' : 'Pending'; return <span className={`status ${status}`}><span className="status-dot" />{label}</span> }

function Profiles({ role, onError, onSuccess }: { role: string; onError: (e: unknown) => void; onSuccess: (s: string) => void }) { const [profiles, setProfiles] = useState<Profile[]>([]); const [showForm, setShowForm] = useState(false); const [provider, setProvider] = useState('codex'); const [label, setLabel] = useState(''); const [models, setModels] = useState(''); const [token, setToken] = useState('')
  const refresh = () => api.profiles().then(r => setProfiles(r.data)).catch(onError); useEffect(() => { void refresh() }, [])
  const create = async () => { try { await api.createProfile({ provider, label, allowed_models: models.split(',').map(v => v.trim()).filter(Boolean) }); setLabel(''); setModels(''); setShowForm(false); onSuccess('Provider profile created. Connect a private worker to continue.'); await refresh() } catch (e) { onError(e) } }
  const action = async (id: string, type: 'connect' | 'disable' | 'reconnect') => { try { const result = await api.profileAction(id, type); if (result.enrollment_token) setToken(result.enrollment_token); onSuccess(type === 'disable' ? 'Profile disabled and worker access revoked.' : 'Enrollment token created. Copy it now.'); await refresh() } catch (e) { onError(e) } }
  return <div className="page-stack"><section className="page-intro"><div><p className="muted">Each profile maps to one isolated provider runtime.</p></div>{role !== 'member' && <button className="primary" onClick={() => setShowForm(v => !v)}>+ Add profile</button>}</section>{showForm && <section className="panel form-panel"><div className="panel-header"><h3>New provider profile</h3><button className="close" onClick={() => setShowForm(false)}>×</button></div><div className="form-grid"><label>Provider<select value={provider} onChange={e => setProvider(e.target.value)}><option value="codex">Codex</option><option value="claude_code">Claude Code</option></select></label><label>Display name<input value={label} onChange={e => setLabel(e.target.value)} placeholder="Personal Codex" /></label><label className="wide">Allowed models <span className="field-help">comma separated</span><input value={models} onChange={e => setModels(e.target.value)} placeholder="gpt-6-luna, sonnet" /></label></div><div className="form-actions"><button className="secondary" onClick={() => setShowForm(false)}>Cancel</button><button className="primary" disabled={!label.trim()} onClick={create}>Create profile</button></div></section>}{token && <section className="token-banner"><div><span className="eyebrow">ONE-TIME ENROLLMENT TOKEN</span><strong>{token}</strong><small>Only give this to your private worker agent. It cannot be viewed again.</small></div><button className="secondary" onClick={() => { void navigator.clipboard?.writeText(token); onSuccess('Enrollment token copied.') }}>Copy token</button><button className="close" onClick={() => setToken('')}>×</button></section>}<section className="profile-grid">{profiles.map(p => <div className="profile-card" key={p.id}><div className="profile-card-top"><ProviderIcon provider={p.provider} /><Status status={p.status === 'disabled' ? 'disabled' : p.worker_status === 'online' ? 'online' : 'pending'} /></div><h3>{p.label}</h3><p>{p.provider === 'codex' ? 'Codex' : 'Claude Code'} · {p.allowed_models?.length || 0} model scopes</p><div className="profile-meta"><span>Worker: <b>{p.worker_status}</b></span><span>{p.last_heartbeat ? `Seen ${formatRelative(p.last_heartbeat)}` : 'Never connected'}</span></div>{role !== 'member' && <div className="card-actions">{p.status === 'disabled' ? <button className="text-button" onClick={() => void action(p.id, 'reconnect')}>Reconnect →</button> : <><button className="text-button" onClick={() => void action(p.id, p.worker_status === 'online' ? 'reconnect' : 'connect')}>{p.worker_status === 'online' ? 'Reconnect' : 'Connect worker'} →</button><button className="danger-button" onClick={() => void action(p.id, 'disable')}>Disable</button></>}</div>}</div>)}{profiles.length === 0 && <div className="wide-empty"><Empty text="No provider profiles connected." action={role !== 'member' ? 'Add your first profile' : undefined} onClick={() => setShowForm(true)} /></div>}</section></div> }

function Keys({ role, onError, onSuccess }: { role: string; onError: (e: unknown) => void; onSuccess: (s: string) => void }) { const [keys, setKeys] = useState<ApiKey[]>([]); const [profiles, setProfiles] = useState<Profile[]>([]); const [showForm, setShowForm] = useState(false); const [rawKey, setRawKey] = useState(''); const [type, setType] = useState<'codex' | 'claude' | 'universal'>('universal'); const [name, setName] = useState('')
  const refresh = () => Promise.all([api.keys(), api.profiles()]).then(([k, p]) => { setKeys(k.data); setProfiles(p.data) }).catch(onError); useEffect(() => { void refresh() }, [])
  const create = async () => { const selected = profiles.filter(p => type === 'universal' || p.provider === (type === 'claude' ? 'claude_code' : type)).map(p => ({ profile_id: p.id, model_pattern: '*', routes: p.provider === 'claude_code' ? ['models', 'chat.completions'] : ['models', 'responses', 'chat.completions'], requests_per_minute: 60 })); try { const result = await api.createKey({ name, type, grants: selected }); setRawKey(result.key); setShowForm(false); setName(''); await refresh() } catch (e) { onError(e) } }
  const keyAction = async (id: string, action: 'rotate' | 'revoke') => { try { const result = await api.keyAction(id, action); if (result.key) setRawKey(result.key); onSuccess(action === 'revoke' ? 'Key revoked.' : 'Key rotated. Save the new secret now.'); await refresh() } catch (e) { onError(e) } }
  return <div className="page-stack"><section className="page-intro"><p className="muted">Scoped keys are shown once. Store them in your client secret manager.</p>{role !== 'member' && <button className="primary" onClick={() => setShowForm(v => !v)}>+ Create key</button>}</section>{showForm && <section className="panel form-panel"><div className="panel-header"><h3>Create API key</h3><button className="close" onClick={() => setShowForm(false)}>×</button></div><div className="form-grid"><label>Key name<input value={name} onChange={e => setName(e.target.value)} placeholder="Production client" /></label><label>Key type<select value={type} onChange={e => setType(e.target.value as typeof type)}><option value="universal">Universal</option><option value="codex">Codex only</option><option value="claude">Claude only</option></select></label></div><p className="form-note">This creates grants for all currently available matching profiles. Refine scopes through the API when granular selection is needed.</p><div className="form-actions"><button className="secondary" onClick={() => setShowForm(false)}>Cancel</button><button className="primary" disabled={!name.trim() || profiles.length === 0} onClick={create}>Generate key</button></div></section>}{rawKey && <section className="token-banner key-banner"><div><span className="eyebrow">SAVE THIS SECRET NOW</span><strong>{rawKey}</strong><small>For security, this key will never be shown again.</small></div><button className="secondary" onClick={() => { void navigator.clipboard?.writeText(rawKey); onSuccess('API key copied.') }}>Copy key</button><button className="close" onClick={() => setRawKey('')}>×</button></section>}<section className="panel table-panel"><div className="panel-header"><h3>Keys</h3><span className="count">{keys.length} total</span></div><div className="table-wrap"><table><thead><tr><th>Name</th><th>Type</th><th>Scopes</th><th>Last used</th><th>Status</th><th /></tr></thead><tbody>{keys.map(k => <tr key={k.id}><td><strong>{k.name}</strong><small>{k.prefix}…</small></td><td><span className={`type-pill ${k.type}`}>{k.type}</span></td><td>{k.grants.length} grants</td><td>{k.last_used_at ? formatRelative(k.last_used_at) : 'Never'}</td><td><Status status={k.state === 'active' ? 'online' : 'disabled'} /></td><td>{role !== 'member' && k.state === 'active' && <div className="row-actions"><button className="text-button" onClick={() => void keyAction(k.id, 'rotate')}>Rotate</button><button className="danger-button" onClick={() => void keyAction(k.id, 'revoke')}>Revoke</button></div>}</td></tr>)}</tbody></table>{keys.length === 0 && <Empty text="No API keys created yet." />}</div></section></div> }

function LegacyPlayground() { return <div className="page-stack"><section className="playground-hero"><span className="gradient-label">COMING NEXT</span><h2>Test your providers<br /><span className="accent">without the guesswork.</span></h2><p className="muted">The secure playground will let you test an authorized profile, model, and API key from one place.</p><div className="playground-lock">✦ <span>Backend playground endpoint is not enabled yet.</span></div></section><div className="playground-preview"><div className="preview-toolbar"><span className="fake-dot red" /><span className="fake-dot yellow" /><span className="fake-dot green" /><span className="preview-title">any endpoint / playground</span></div><div className="preview-body"><div className="fake-sidebar"><span className="fake-line short" /><span className="fake-line" /><span className="fake-line" /></div><div className="fake-chat"><span className="fake-line short" /><span className="fake-line wide" /><span className="fake-line" /><div className="fake-input">Playground will be available after the secure execution endpoint is added.</div></div></div></div></div> }

function formatRelative(value: string) { const date = new Date(value); const minutes = Math.max(0, Math.round((Date.now() - date.getTime()) / 60000)); if (minutes < 1) return 'just now'; if (minutes < 60) return `${minutes}m ago`; const hours = Math.round(minutes / 60); if (hours < 24) return `${hours}h ago`; return `${Math.round(hours / 24)}d ago` }

function Playground() {
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileId, setProfileId] = useState('')
  const [model, setModel] = useState('')
  const [prompt, setPrompt] = useState('')
  const [result, setResult] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => { api.profiles().then(response => setProfiles(response.data)).catch(() => setProfiles([])) }, [])
  const selected = profiles.find(profile => profile.id === profileId)
  const models = selected?.allowed_models?.filter(Boolean) ?? []

  const run = async () => {
    setError('')
    setResult('')
    if (!profileId || !model || !prompt.trim()) { setError('Choose a profile, model, and enter a prompt.'); return }
    setBusy(true)
    try {
      const response = await api.playground({ profile_id: profileId, model, prompt })
      const content = response.choices?.[0]?.message?.content
      setResult(content || JSON.stringify(response, null, 2))
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'The playground request failed.')
    } finally { setBusy(false) }
  }

  return <div className="page-stack playground-page">
    <section className="page-intro"><div><p className="muted">Test an authorized profile without exposing a durable API key to the browser.</p></div><span className="secure-badge">⌁ Session-authorized</span></section>
    <section className="playground-layout">
      <div className="panel playground-controls"><div className="panel-header"><h3>Run a prompt</h3><span className="small-label">PRIVATE</span></div>
        <label>Provider profile<select value={profileId} onChange={event => { setProfileId(event.target.value); setModel('') }}><option value="">Select profile</option>{profiles.map(profile => <option key={profile.id} value={profile.id}>{profile.label} · {profile.provider === 'claude_code' ? 'Claude Code' : 'Codex'}</option>)}</select></label>
        <label>Model<select value={model} onChange={event => setModel(event.target.value)} disabled={!profileId}><option value="">Select model</option>{models.map(item => <option key={item} value={item}>{item}</option>)}</select></label>
        <label>Prompt<textarea value={prompt} onChange={event => setPrompt(event.target.value)} placeholder="Ask something safe and useful…" rows={9} maxLength={20000} /></label>
        <div className="playground-actions"><span>{prompt.length.toLocaleString()} / 20,000</span><button className="primary" onClick={run} disabled={busy || profiles.length === 0}>{busy ? 'Running…' : 'Run prompt →'}</button></div>
        {error && <p className="inline-error" role="alert">{error}</p>}
        <p className="form-note">Your session authorizes this run. The prompt is not saved by the frontend, and no provider credential is sent to the browser.</p>
      </div>
      <div className="panel result-panel"><div className="panel-header"><h3>Response</h3>{result && <span className="success-text">Completed</span>}</div>{result ? <pre className="result-text">{result}</pre> : <div className="result-empty"><span>✦</span><p>Your response will appear here.</p><small>Choose a profile and send a prompt to begin.</small></div>}</div>
    </section>
  </div>
}

function Activity({ onError }: { onError: (e: unknown) => void }) {
  const [usage, setUsage] = useState<UsageRow[]>([])
  useEffect(() => { api.usage().then(response => setUsage(response.data)).catch(onError) }, [onError])
  return <div className="page-stack"><section className="page-intro"><p className="muted">Recent request activity for this workspace. Prompt and response content is never displayed here.</p></section><section className="panel table-panel"><div className="panel-header"><h3>Requests</h3><span className="count">{usage.length} recent</span></div><div className="table-wrap"><table><thead><tr><th>Route</th><th>Model</th><th>Status</th><th>Latency</th><th>Time</th></tr></thead><tbody>{usage.map((u, i) => <tr key={`${u.created_at}-${i}`}><td><code>{u.route}</code></td><td>{u.model || '—'}</td><td><span className={u.status_code >= 400 ? 'request-code error' : 'request-code'}>{u.status_code}</span></td><td>{u.latency_ms || 0}ms</td><td>{formatRelative(u.created_at)}</td></tr>)}</tbody></table>{usage.length === 0 && <Empty text="No request activity recorded yet." />}</div></section></div>
}
