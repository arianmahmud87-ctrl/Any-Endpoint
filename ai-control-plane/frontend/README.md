# Any Endpoint frontend

Minimal React/TypeScript dashboard for the AI control plane.

## Local development

From `ai-control-plane/frontend`:

```powershell
npm install
npm run dev
```

Vite proxies `/api`, `/v1`, `/healthz`, and `/readyz` to the local Go control plane at `http://127.0.0.1:8080`. The UI uses same-origin relative URLs and `credentials: include`; it does not store Google tokens, provider credentials, or durable raw API keys.

## Current screens

- Google sign-in entry
- Overview metrics and provider health
- Provider profile creation/connect/reconnect/disable
- One-time enrollment token copy panel
- API key create/rotate/revoke with one-time reveal
- Usage and audit activity tables
- Operations summary
- Secure same-origin AI playground with profile/model scope

## Security/accessibility notes

- The session cookie remains server-managed and HttpOnly.
- The CSRF token from `/api/me` is held in runtime memory only and sent on state-changing requests.
- Raw generated keys are shown in a transient panel and never persisted by the UI.
- Model output is not currently rendered because the playground backend is not enabled; future output must be rendered as escaped text, never injected HTML.
- Buttons, form controls, labels, focus states, responsive navigation, and status text are included for keyboard and screen-reader use.
- Serve the frontend and API from the same origin in production, or use a strict same-origin reverse-proxy/BFF. Do not add permissive CORS.


## Preview mode

For local UI review, `VITE_PREVIEW_MODE` defaults to enabled when unset. It bypasses the login screen and uses in-memory demo data; it does not disable backend authentication. This mode must be disabled in any deployed build:

```powershell
$env:VITE_PREVIEW_MODE = "false"
npm run build
```
