import { type ReactNode } from "react";
import { Database, ExternalLink, Loader2 } from "lucide-react";

import { Button } from "_layouts/_components/ui/button";

// ---- Types (mirror the /post/vhost/state + reconcile Result payloads) ----

export type HostRow = {
    hostname: string;
    kind: string;
    stack?: string;
    upstream?: string;
    status: "in_sync" | "will_write" | "will_remove" | "orphan" | string;
};

export type Skip = { table: string; host: string; reason: string };

export type DryRun = {
    vhosts_dir: string;
    files: Array<{ name: string }>;
    hosts: HostRow[];
    would_write: string[];
    would_remove: string[];
    orphans: string[];
    skips?: Skip[];
    missing_tables?: string[];
    in_sync: boolean;
};

export type ManageRow = {
    id: number;
    host: string;
    serverStack?: string;
    target: string;
    code?: number;
    isActive: boolean;
    softDeleted: boolean;
};

export type Upstream = {
    name: string; // container name
    target: string; // 127.0.0.1:<port>
};

export type ManageSets = {
    systemHosts: ManageRow[];
    redirects: ManageRow[];
    stacks: string[];
    upstreams: Upstream[];
};

export type HostHealth = {
    host: string;
    alert: boolean;
    dnsOk: boolean;
    tlsOk: boolean;
    resolvedIps?: string[];
    certExpiryMs?: number;
    lastError?: string;
    failures: number;
    checkedAtMs: number;
};

export type VhostState = {
    configured: boolean;
    source?: string;
    vhostsDir: string;
    liveReload: boolean;
    message?: string;
    error?: string;
    dryRun?: DryRun;
    manage?: ManageSets;
    health?: Record<string, HostHealth>;
    healthOn?: boolean;
    protected?: PinnedRow[];
    protectedWarning?: string;
};

export type PinnedRow = {
    host: string;
    role?: string;
    upstreams?: string[];
    guarded: boolean;
    pinned: boolean;
    drift?: "missing" | "unmanaged" | string;
};

export type ReconcileResult = {
    reloaded: boolean;
    written: string[];
    removed: string[];
    removes_suppressed?: string[];
    orphans: string[];
    skips?: Skip[];
    adapt_warnings?: string[];
    missing_tables?: string[];
    blocked_drops?: string[];
    backup_path?: string;
    error?: string;
    duration_ms: number;
};

export type Source = { id: string; name: string; engine: string };

export type Section = "tenant" | "system" | "redirects" | "orphans";

// ---- Normalizers: Go marshals nil slices as JSON null, so coerce to [] ----

export function arr<T>(x: T[] | null | undefined): T[] {
    return Array.isArray(x) ? x : [];
}

export function normalizeState(s: VhostState): VhostState {
    if (s.dryRun) {
        const d = s.dryRun;
        s.dryRun = {
            ...d,
            files: arr(d.files),
            hosts: arr(d.hosts),
            would_write: arr(d.would_write),
            would_remove: arr(d.would_remove),
            orphans: arr(d.orphans),
            skips: arr(d.skips),
            missing_tables: arr(d.missing_tables),
        };
    }
    if (s.manage) {
        s.manage = {
            systemHosts: arr(s.manage.systemHosts),
            redirects: arr(s.manage.redirects),
            stacks: arr(s.manage.stacks),
            upstreams: arr(s.manage.upstreams),
        };
    }
    if (!s.health) s.health = {};
    s.protected = arr(s.protected);
    return s;
}

// UnreachableChip rides ALONGSIDE the sync chip: an orthogonal reachability warning
// (DNS/TLS) that is alert-only and never drives a write or removal.
export function UnreachableChip({ health }: { health?: HostHealth }) {
    if (!health?.alert) return null;
    return (
        <span
            className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[11px] font-semibold text-amber-600 dark:text-amber-400"
            title={health.lastError || "Not reaching this server"}
        >
            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
            Not reaching us
        </span>
    );
}

export function normalizeResult(r: ReconcileResult): ReconcileResult {
    return {
        ...r,
        written: arr(r.written),
        removed: arr(r.removed),
        removes_suppressed: arr(r.removes_suppressed),
        orphans: arr(r.orphans),
        skips: arr(r.skips),
        adapt_warnings: arr(r.adapt_warnings),
        missing_tables: arr(r.missing_tables),
        blocked_drops: arr(r.blocked_drops),
    };
}

export function summarizeError(err: string): string {
    const first = err.split("\n")[0].trim();
    return first.length > 140 ? `${first.slice(0, 137)}…` : first;
}

// ---- Shared presentational components ----

export const inputCls =
    "w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring";

export function Pill({ tone, children }: { tone: "ok" | "warn" | "err"; children: ReactNode }) {
    const cls =
        tone === "ok"
            ? "text-emerald-600 dark:text-emerald-400 border-emerald-500/20 bg-emerald-500/10"
            : tone === "warn"
              ? "text-amber-600 dark:text-amber-400 border-amber-500/25 bg-amber-500/10"
              : "text-destructive border-destructive/20 bg-destructive/10";
    return (
        <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${cls}`}>
            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
            {children}
        </span>
    );
}

export function StatusChip({ status }: { status: string }) {
    const map: Record<string, { tone: "ok" | "warn" | "err"; label: string }> = {
        in_sync: { tone: "ok", label: "In sync" },
        will_write: { tone: "warn", label: "Will write" },
        will_remove: { tone: "err", label: "Will remove" },
        orphan: { tone: "err", label: "Orphan" },
    };
    const s = map[status] ?? { tone: "warn" as const, label: status };
    return <Pill tone={s.tone}>{s.label}</Pill>;
}

export function rowTint(status: string): string {
    if (status === "orphan" || status === "will_remove") return "bg-destructive/[0.04]";
    if (status === "will_write") return "bg-amber-500/[0.05]";
    return "";
}

export function Modal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
            <div className="w-full max-w-md rounded-lg border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
                <h2 className="mb-3 text-base font-semibold text-foreground">{title}</h2>
                {children}
            </div>
        </div>
    );
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
    return (
        <label className="block">
            <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>
            {children}
            {hint ? <span className="mt-1 block text-[11px] text-muted-foreground">{hint}</span> : null}
        </label>
    );
}

export function FormActions({ saving, disabled, onCancel, onSave }: { saving: boolean; disabled?: boolean; onCancel: () => void; onSave: () => void }) {
    return (
        <div className="mt-5 flex justify-end gap-2">
            <Button variant="outline" size="sm" onClick={onCancel} disabled={saving}>
                Cancel
            </Button>
            <Button size="sm" className="gap-2" onClick={onSave} disabled={saving || disabled}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                Save
            </Button>
        </div>
    );
}

export function EmptyBanner({ title, body }: { title: string; body: string }) {
    return (
        <div className="flex min-h-40 flex-col items-center justify-center rounded-md border border-dashed border-border px-6 text-center">
            <Database className="mb-3 h-8 w-8 text-muted-foreground/40" />
            <p className="text-sm font-medium text-foreground">{title}</p>
            <p className="mt-1 max-w-md text-xs text-muted-foreground">{body}</p>
        </div>
    );
}

// linkCls is the standard clickable-link treatment: link color + underline +
// pointer, so it reads as a link (not body text). Pairs with a trailing ↗ icon
// since these open in a new tab.
const linkCls =
    "inline-flex items-center gap-0.5 text-sky-600 underline decoration-sky-600/30 underline-offset-2 hover:decoration-sky-600 dark:text-sky-400 dark:decoration-sky-400/30 dark:hover:decoration-sky-400";

// HostLink renders a hostname as a link to https://<host> in a new tab — so an
// operator can check whether a site is still live (e.g. before pruning an orphan).
export function HostLink({ host, className }: { host: string; className?: string }) {
    return (
        <a
            href={`https://${host}`}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className={`${linkCls} ${className ?? ""}`}
            title={`Open https://${host} in a new tab`}
        >
            {host}
            <ExternalLink className="h-3 w-3 shrink-0 opacity-60" />
        </a>
    );
}

// UrlLink renders an already-absolute URL (e.g. a redirect target) as a new-tab link.
export function UrlLink({ url, className }: { url: string; className?: string }) {
    return (
        <a
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className={`${linkCls} ${className ?? ""}`}
            title={`Open ${url} in a new tab`}
        >
            {url}
            <ExternalLink className="h-3 w-3 shrink-0 opacity-60" />
        </a>
    );
}

export function ViewHeader({ title, subtitle, actions }: { title: string; subtitle: string; actions?: ReactNode }) {
    return (
        <div className="mb-4 flex flex-wrap items-end justify-between gap-2">
            <div>
                <h2 className="text-lg font-semibold text-foreground">{title}</h2>
                <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>
            </div>
            {actions}
        </div>
    );
}
