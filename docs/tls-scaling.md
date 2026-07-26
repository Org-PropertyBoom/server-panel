# TLS scaling — when to put Cloudflare in front of Caddy

Decision criteria for the platform's TLS/edge strategy. Written 2026-07-26 after the
on-demand-TLS rollout. Read this before assuming "Caddy can't scale" or reaching for
Cloudflare — the limits are specific, and none of them are cert count.

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
