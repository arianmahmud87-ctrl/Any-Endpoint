# Any Endpoint frontend

## Control-plane integration

The frontend is configured for the real control-plane by default:

- `VITE_PREVIEW_MODE=false` (or unset) uses same-origin API calls;
- Vite runs on `127.0.0.1:5173` and proxies `/api`, `/v1`, `/healthz`, and `/readyz` to `127.0.0.1:8080`;
- `/api/me` supplies the HttpOnly-session identity and runtime CSRF token;
- response adapters unwrap backend list envelopes and normalize nested summary/profile/key shapes;
- Claude UI values map to the backend `claude_code` provider enum;
- API-key creation sends explicit profile grants and `expires_at`;
- playground response handling supports the backend OpenAI Chat Completion response shape.

Use `VITE_PREVIEW_MODE=true` only for local visual demos. Do not deploy preview mode.
