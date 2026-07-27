# TLS scaling — when to put Cloudflare in front of Caddy

Decision criteria for the platform's TLS/edge strategy. Written 2026-07-26 after the
on-demand-TLS rollout. Read this before assuming "Caddy can't scale" or reaching for
Cloudflare — the limits are specific, and none of them are cert count.

## Decision (2026-07-26): go with Cloudflare

The edge provider will be **Cloudflare**, not AWS-native (CloudFront/ACM). The
deciding factor is the per-tenant custom-domain cert problem: **Cloudflare for SaaS**
is a turnkey product for it, whereas AWS has no equivalent and would require building +
running that automation ourselves. Bandwidth economics (no per-GB egress) and setup
simplicity also favor Cloudflare. See "Alternatives considered — AWS-native" below for
the full comparison.

**The origin stays on AWS EC2 with Caddy either way** — this is only about what sits in
front. Adopting Cloudflare undoes no AWS investment; it's an edge choice, not a
migration. Roll it out in the two stages under "The Cloudflare adoption plan".

## What's live today (the baseline)

- **Caddy terminates TLS on a single origin** for all platform + tenant hosts.
- **On-demand TLS**: host files render `tls { on_demand }`; Caddy obtains a cert only
  when a host is actually visited, gated by the panel's `GET /internal/tls-ask`
  endpoint (authorizes against active `platform_hosts` / `website_hosts` /
  `platform_redirect_hosts` rows; fail-closed; loopback-only). Dead/mispointed domains
  never trigger ACME. See `docs/specs/caddy-vhost-management.md` and
  `services/caddy/`.
- **Issuer pinned to Let's Encrypt production** (`issuer acme` in the template); the
  dead ZeroSSL legacy fallback (code 2977) is dropped. Account email in the global
  `/etc/caddy/Caddyfile` block.

This is a correct, cheap, single-origin architecture. It does **not** fall over on
number of certs — hundreds to low-thousands of on-demand certs on one box is fine.

## The three real scaling limits (none is cert count)

### 1. Let's Encrypt rate limits — bites on BURSTS, not steady state
Since issuance is pinned to LE only, LE's ceilings apply. The relevant ones:
- **~300 new orders per 3 hours** per account (~2,400/day).
- **5 failed validations per hostname per hour** (the dead-domain case).

Steady state is fine — renewals happen ~30 days before expiry, spread out, and
on-demand only issues on first visit (self-throttling). It bites when:
- **onboarding hundreds of tenants at once**, or
- **mass re-issuance** (e.g. cert storage volume wiped).

**Mitigations:** stagger onboarding; keep the cert storage volume durable/backed-up;
or add a second ACME issuer (ZeroSSL **with** proper EAB credentials — NOT the dead
legacy integration) to roughly double issuance headroom.

### 2. Multi-server / HA — the real re-architecture point
On **one** origin, on-demand is simple. Two+ Caddy instances behind a load balancer
each need **shared cert storage + a shared ACME account**, or each issues its own
certs — multiplying rate-limit usage and causing inconsistency. Solvable with a Caddy
storage backend (shared DB / S3), but it is genuine work. Cloudflare sidesteps it
because the edge holds the certs.

### 3. Origin CPU + no edge protection — likely the FIRST practical limit
Caddy terminating all TLS means the origin bears **every handshake's CPU** and there is
**no edge caching or DDoS absorption** — all traffic hits the box. On a small/burstable
instance (the host is a t3 with CPU credits) heavy traffic, or a domain getting
attacked, is felt directly. This is the most likely trigger to hit before 1 or 2.

## Triggers — stay on Caddy-only until one of these is true

Stay on Caddy-only TLS while: single origin, gradual onboarding, moderate traffic, not
a DDoS target. Put Cloudflare in front when a **specific** trigger fires:

| Trigger | Signal to watch | Cloudflare answer |
| --- | --- | --- |
| **1. Onboarding / issuance bursts** | Hitting LE new-order limits; issuance backlog | CF-for-SaaS issues at its own edge (no LE limits) |
| **2. Need HA / multiple origins** | Adding a second app/edge server; multi-region | CF edge holds certs; no shared Caddy storage to run |
| **3. Origin CPU / DDoS / cache** | t3 CPU credits draining; traffic spikes; attacks | CF offloads handshakes, caches, absorbs floods |

None of these are hitting us as of 2026-07-26. There is no urgency; these are the
signposts for when to revisit.

## The Cloudflare adoption plan (agreed 2026-07-26)

Goal: **use Cloudflare as edge caching** (+ CDN / DDoS), keep Caddy as origin. Do it in
two stages — the split is forced by DNS control, not preference.

### Stage 1 (do now, free, low-risk) — our own domains
For hosts in the `propertyweb.co` zone we control (`grafana`, `cp`, `app`, `api`, …):
1. Cloudflare DNS record → **Proxied** (orange cloud).
2. SSL/TLS mode → **Full (strict)**.
3. On Caddy, serve a **Cloudflare Origin Certificate** (free, 15-year, covers
   `*.propertyweb.co`) for those hosts — **not** on-demand LE (see gotcha).
4. Add **Cache Rules** (static assets cache by default; `Cache Everything` + edge TTL
   for cacheable HTML).

Result: edge caching + CDN + DDoS + free edge cert, zero change to the tenant model.

### Stage 2 (later, paid, only if a trigger fires) — tenant custom domains
Cloudflare can only cache a domain it **proxies**, and can only proxy a domain whose DNS
it controls. The ~94 tenant property domains are **tenant-owned** — we don't control
their DNS. So edge-caching those requires **Cloudflare for SaaS custom hostnames** (paid
beyond a free quota — check current pricing). Until then, tenant sites stay
**Caddy-direct** (on-demand LE, uncached). Nothing is wasted: the on-demand work is the
right baseline and the fallback for non-proxied domains.

## Alternatives considered — AWS-native (rejected 2026-07-26)

Since the origin is already an EC2 box, the AWS-native edge stack was a real option:

| Need | Cloudflare (chosen) | AWS-native (rejected) |
| --- | --- | --- |
| Edge cache / CDN | CF proxy | CloudFront |
| Public certs | Universal SSL (free) | ACM (free) |
| Per-tenant custom domains | **Cloudflare for SaaS** (turnkey) | CloudFront + ACM + **our own automation** |
| DNS | CF DNS | Route 53 |
| WAF | CF WAF | AWS WAF |
| DDoS | included free | Shield (Standard free, Advanced paid) |

Why Cloudflare won:

1. **Per-tenant custom domains is the deciding factor.** Cloudflare for SaaS auto-issues
   + auto-renews a cert per tenant custom hostname via one API. AWS has **no turnkey
   equivalent** — we'd script ACM DNS-validated issuance + attach domains to CloudFront
   distributions (100 alt-names/distribution default → shard at scale). That's
   engineering we'd own and maintain.
2. **Bandwidth economics.** CloudFront bills **per GB egress**; for image-heavy property
   sites that adds up. Cloudflare bandwidth is effectively free on standard plans.
3. **Simplicity** for a small team, and no lock-in to AWS edge services.

Notes that shaped the call:

- **ACM public certs can't be installed on our own EC2/Caddy** (the key can't be
  exported — ACM only works with CloudFront/ALB/etc.). So the AWS path would FORCE TLS
  termination at CloudFront, same as Cloudflare terminates at its edge. No advantage
  there for keeping certs on Caddy.
- AWS-native would only have won if we were deliberately AWS-all-in (compliance /
  data-residency), already leaning on CloudFront/ALB, wanted a single vendor/bill, or
  needed tight VPC/private-origin integration — none of which apply now.

## ⚠ The ask endpoint is on the HANDSHAKE path (2026-07-26 outage)

**Corrects an earlier assumption in this doc and in the original spec.** We reasoned
"existing certs keep serving, a fail-closed ask only pauses NEW issuance." **That is
false.** Caddy consults the `ask` endpoint on **handshakes** for any hostname not in
its in-memory cert cache — not only at issuance. With a fail-closed ask, that makes
the panel a **single point of failure for TLS across every on-demand host**.

Observed: the Owner clicked **Check Update**; the panel restarted; during that window
`/internal/tls-ask` failed and **go3, laravel3 and cp.propertyweb.co all returned curl
`000` simultaneously**. It self-healed the moment the panel came back — but a routine
panel deploy took HTTPS down platform-wide, and every future deploy would repeat it.

Mitigation shipped (see `services/caddy_engine.go` + `tls_ask_allow` table):
- Positive answers cached **12h** (was 45s; override `TLS_ASK_CACHE_HOURS`).
- The allowlist is **persisted to SQLite and re-read at boot**, so a restart keeps
  answering 200 for already-known-good hosts.
- The allowlist is consulted **before** the DB, so a host valid recently still answers
  while the panel is warming or the shared DB is unreachable.
- Staleness is handled by **explicit eviction** (disable/suppress removes the host
  immediately) rather than a short clock.
- Genuinely **unknown hosts stay fail-closed** — that's the abuse guard that killed the
  ACME storm.

**VERIFIED 2026-07-27:** 60 unbroken `200`s across a live
`systemctl restart ppt-server-panel@root`. Panel restarts no longer affect TLS.

Re-running the acceptance test (these gotchas cost several attempts):

```bash
( for i in $(seq 1 60); do curl -s -o /dev/null -w "%{http_code} " -A '<full browser UA>' https://<host>; sleep 1; done ) & sleep 5; systemctl restart ppt-server-panel@root; wait
```

- Run it from **real SSH / EC2 Instance Connect — NOT the panel's own web terminal**.
  That shell is a child of the panel process and dies with the restart.
- `sudo -i` first: otherwise `systemctl` hits a polkit prompt and times out.
- Once a host is **Proxied**, plain `curl` gets **403** from Cloudflare bot
  protection — send a FULL browser User-Agent (a bare `Mozilla/5.0` is not enough).
  A 403 still proves TLS succeeded; `000` is the failure signal to watch for.

**Residual risk:** while the panel process is fully **down** (not warming — down), the
ask is unreachable and Caddy refuses for hosts not in its own cert cache. The panel
cannot fix that from inside itself. Real mitigations: keep restarts short, and for
hosts that must never depend on the panel, use `cf_origin`/a static cert (no ask on
the path at all). **Do not put the panel's own domain on on-demand** — that's a
circular dependency (panel down → its domain can't get a cert → can't reach the panel
to fix it); keep `cp.propertyweb.co` a static/pinned block.

## Key gotchas (do not skip)

- **Proxying breaks on-demand LE for that host.** Once a domain is proxied through
  Cloudflare, Caddy CANNOT get an LE cert for it — the ACME challenge (TLS-ALPN-01 /
  HTTP-01) is intercepted by CF's edge, not the origin. Proxied hosts MUST use a **CF
  Origin Certificate** (no ACME) or DNS-01. On-demand stays as-is for **non-proxied**
  (tenant-direct) hosts. End state is a hybrid:
  - proxied own-domains → CF edge cert + Caddy Origin Cert, cached;
  - tenant-direct domains → Caddy on-demand LE, uncached.
- **Proxying = CF terminates TLS to visitors.** You cannot cache without it. Visitors
  see Cloudflare's cert; Caddy's cert drops to the origin leg only.
- **Caching effectiveness depends on content.** Static assets cache automatically;
  dynamic (phalcon-rendered, per-user) HTML only benefits with explicit cache rules and
  only if actually cacheable.
- **EC2 CPU-credit balance is a CloudWatch metric, not Prometheus/node-exporter.** Watch
  `node_cpu` saturation + load as a proxy; read credit balance from CloudWatch.

## TODO when Stage 1 is implemented
- Document the exact Caddy config for serving a CF Origin Cert on proxied hosts, and how
  it coexists with the on-demand template (`services/caddy/render`) so proxied vs
  on-demand hosts don't collide (likely: exclude proxied hosts from on-demand rendering,
  or a per-host cert override).
