# Phase 1 security baseline

## Scope

এই phase শুধু platform foundation তৈরি করে। Google OAuth, provider credential, public `sk` key এবং profile routing এখনো নেই। এই সীমা ইচ্ছাকৃত—authorization layer ছাড়া provider access expose করা হবে না।

## Threats and controls

| সম্ভাব্য দুর্বলতা | Phase 1 প্রতিরোধ |
| --- | --- |
| ভুলভাবে public HTTP চালু | production-এ `https://` public URL বাধ্যতামূলক; reverse proxy template রাখা হয়েছে |
| insecure production secrets | production startup-এ session secret, key pepper, trusted proxy এবং non-local secret backend বাধ্যতামূলক |
| secret commit | `.env`/local data ignore; `.env.example`-এ placeholder only |
| log-এ credential/query leak | structured request logs-এ query, authorization ও body লেখা হয় না |
| request smuggling/DoS | header/read/write timeout এবং bounded request body config |
| browser framing/MIME abuse | security response headers |
| DB/Redis public exposure | Compose-এ তাদের host port mapping নেই; internal service DNS only |
| dependency outage | `/readyz` configured dependency reachability দেখায়; liveness আলাদা `/healthz` |
| future CORS mistake | Phase 1-এ permissive CORS নেই; UI same-origin BFF pattern অনুসরণ করবে |

## Operational rules

1. Real provider credential, Google token, API key বা production secret repository-তে রাখা যাবে না।
2. Production run reverse proxy/TLS-এর পেছনে হবে; Go process public TLS endpoint হিসেবে ধরা হবে না।
3. Database migration transaction/backup policy ছাড়া production schema পরিবর্তন করা যাবে না।
4. Provider worker public network-এ bind করবে না।
5. `/healthz` dependency status বা secret/config detail প্রকাশ করবে না।
6. Later auth/key phases-এ fail-closed authorization, tenant-scoped queries এবং revocation checks বাধ্যতামূলক।

## Phase 1 exit criteria

- `go test`, `go vet`, `go build` সফল।
- production config invalid হলে process listener bind করার আগে fail করে।
- `/healthz` ও `/readyz` request ID এবং safe security headers দেয়।
- migration-এ raw secret column নেই; credential storage এখনও implementation করা হয়নি।
- Compose-এ PostgreSQL/Redis public host port-এ expose হয় না।


## Phase 2 additions

| সম্ভাব্য দুর্বলতা | Phase 2 প্রতিরোধ |
| --- | --- |
| OAuth CSRF/state replay | high-entropy state, browser binding, Redis TTL, single-use consume |
| authorization-code interception | S256 PKCE verifier stored server-side and consumed on exchange |
| token forgery/mix-up | Google OIDC issuer/audience/signature/expiry/nonce verification; verified email required |
| session fixation/theft | random opaque session, HMAC digest only in PostgreSQL, HttpOnly/SameSite cookie, revoke on logout |
| CSRF on management mutations | same-origin check plus session-derived synchronizer token on POST/PUT/PATCH/DELETE |
| user/org IDOR | every future management handler must load session membership and query with organization scope |
| duplicate first-login workspace | `personal` flag and partial unique owner index; membership idempotent upsert |
| token/secret logging | OAuth error logs use stable generic reasons; tokens/codes/session values are never logged or audited |

Phase 2 authentication is enabled only with `AUTH_ENABLED=true`. Production startup rejects missing OIDC configuration, missing dependencies, insecure public URL, local secret backend, or missing required secrets before opening the listener.


## Phase 3 additions

| সম্ভাব্য দুর্বলতা | Phase 3 প্রতিরোধ |
| --- | --- |
| browser/provider credential leak | public API accepts only profile metadata; no password, bearer token, auth file, or raw secret field |
| secret database compromise | only opaque external secret reference and backend type are stored; provider material remains in isolated worker/secret manager |
| enrollment token replay | high-entropy token, HMAC digest only, short TTL, row lock and consumed-at transaction |
| public access to worker control | separate `INTERNAL_HTTP_ADDR`; Caddy explicitly returns 404 for `/internal/*`; no public port mapping |
| cross-organization profile access | every list/detail/connect query includes the authenticated organization ID and admin role check |
| worker credential replay | worker token digest only, heartbeat requires bearer token, revoked/status-checked runtime row |
| agent/profile mix-up | enrollment transaction binds the consumed attempt to one profile and replaces only that profile's runtime |
| unbounded profile metadata | provider enum, label/model length limits, unknown JSON fields rejected, model list capped |

The internal listener still requires private Docker networking, firewall/security-group rules, and later mTLS. A bearer worker token is an interim enrollment credential, not a substitute for production mTLS.


## Phase 4 additions

| সম্ভাব্য দুর্বলতা | Phase 4 প্রতিরোধ |
| --- | --- |
| raw API key database/log leak | only public prefix + peppered HMAC verifier stored; raw key returned once and never logged |
| low-entropy or malformed key | CSPRNG 256-bit secret, strict `skv1_<id>_<secret>` parser, constant-time verifier comparison |
| universal-key privilege expansion | explicit profile/provider/route grants; key type/provider mismatch rejected; ambiguous scope denied |
| revoked/expired key reuse | every gateway verification checks state and expiry; revoke changes state immediately |
| rotation overlap | new key is created, old key is revoked; compensation revokes the new key if later rotation step fails |
| cross-organization grant | grant profile lookup always includes organization ID and disabled profiles are rejected |
| provider key leakage | gateway verifies user key but never forwards it to workers; provider credentials remain worker-side |
| arbitrary upstream access | Phase 4 gateway supports only a small `/v1` route allowlist and returns 501 before provider routing |
| malformed management JSON | unknown fields rejected, body is globally bounded, grant/model/route counts and lengths are capped |

API-key verification is intentionally separate from browser session authentication. A management session cannot call `/v1`, and a public key cannot call `/api` management routes.


## Phase 5 additions

| সম্ভাব্য দুর্বলতা | Phase 5 প্রতিরোধ |
| --- | --- |
| SSRF via worker URL | enrollment-time HTTP(S) validation, allowed-host list/private DNS resolution, private remote-IP check on every dial |
| caller key forwarded to provider | gateway replaces `Authorization` with encrypted worker transport token; caller headers are allowlisted |
| worker token plaintext at rest | AES-GCM encrypted transport token plus HMAC digest for heartbeat verification |
| request/worker DoS | bounded body/response, request timeout, Redis fixed-window per-key/profile/route limit, concurrency semaphore |
| stale/offline worker routing | route requires `online` status and heartbeat within two minutes |
| cross-profile routing | selected grant and organization-scoped worker query must match the same profile/provider |
| provider error/credential leak | gateway returns generic 502 for worker failures and does not forward worker error bodies |
| rate-limit backend outage bypass | Redis rate-limit failure returns 503 instead of allowing the request |
| stream/body replay corruption | gateway buffers bounded request body before authorization and restores it for forwarding |

Phase 5 worker transport encryption is an interim single-server control-plane measure. Production worker agents still require mTLS, certificate rotation, and private firewall/security-group rules before public deployment.


## Phase 6 additions

| সম্ভাব্য দুর্বলতা | Phase 6 প্রতিরোধ |
| --- | --- |
| cross-tenant observability leak | usage/audit/summary queries always filter by session organization ID |
| destructive worker action | disable/reconnect require owner/admin session plus CSRF; disable revokes transport ciphertext/digest |
| stale worker shown healthy | summary separates online recent-heartbeat workers from stale workers |
| prompt/privacy leakage in dashboards | usage/audit responses contain route, model, status and latency only; no prompt or response body |
| unbounded audit/usage API response | limit query capped at 200; production retention/partition job remains required before launch |
| operational error exposure | aggregate endpoints return safe counters; SQL/provider errors become generic API errors |
| accidental production readiness | readiness remains dependency-aware; production configuration still fails closed on missing secure prerequisites |

Production deployment must add encrypted backups, tested restore, log rotation, monitoring/alerting, database retention jobs, mTLS certificate rotation, firewall rules, and an incident runbook before public launch.


## Provider worker / mTLS phase

| সম্ভাব্য দুর্বলতা | প্রতিরোধ |
| --- | --- |
| bearer-only internal transport compromise | production mTLS requires private CA, server certificate and verified worker client certificate |
| certificate misconfiguration | startup fails when MTLS_ENABLED lacks any CA/cert/key path; TLS 1.3 minimum |
| public exposure of worker control | internal listener remains separate; Caddy rejects `/internal/*`; worker port is not host-mapped |
| profile/worker mismatch | profile-bound worker token remains required in addition to certificate transport |
| certificate expiry | rotation is a deployment gate; certificates must be renewed before expiry and old certs revoked |
| credential cross-contamination | Codex adapter requires one isolated auth home per profile; Claude adapter requires isolated CLI home/container strategy |

mTLS files are runtime secrets and must be mounted from AWS/Gcore secret storage or an equivalent secret manager, never committed to the repository. The worker-agent provider adapters remain explicit scaffolds until provider credential isolation is verified.


## Playground additions

| সম্ভাব্য দুর্বলতা | Playground প্রতিরোধ |
| --- | --- |
| browser API-key/provider-token leak | playground uses signed-in session and server-side profile routing; no durable key or provider credential input |
| cross-organization profile execution | selected profile is queried with session organization ID and disabled profiles are rejected |
| model scope bypass | backend requires model to exist in profile `allowed_models`; browser selection is not trusted alone |
| prompt persistence/privacy leak | usage/audit store metadata only; prompt and response content are not persisted by control plane |
| playground abuse/cost spike | per-user/profile Redis minute limit, shared concurrency cap, body bounds and gateway timeout |
| response XSS | frontend renders result as escaped `<pre>` text, never HTML injection |
