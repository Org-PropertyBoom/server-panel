import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { Loader2, Lock } from "lucide-react";
import { toast } from "sonner";

import DashboardLayout from "_layouts/dashboard";
import { runtime } from "../../runtime";
import ReconcileHeader from "./reconcile-header";
import VHostsSidebar from "./sidebar";
import TenantView from "./tenant";
import SystemView from "./system";
import RedirectsView from "./redirects";
import OrphansView from "./orphans";
import {
    EmptyBanner,
    normalizeResult,
    normalizeState,
    type ReconcileResult,
    type Section,
    summarizeError,
    type VhostState,
} from "./shared";

function section(param?: string): Section {
    return param === "system" || param === "redirects" || param === "orphans" ? param : "tenant";
}

export default function VHostsRoute() {
    const { section: sectionParam } = useParams<{ section?: string }>();
    const active = section(sectionParam);

    if (!runtime.isRoot) {
        return (
            <DashboardLayout title="Caddy VHosts" description="Manage the public hosts Caddy serves.">
                <div className="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-border text-center">
                    <Lock className="mb-3 h-8 w-8 text-muted-foreground/40" />
                    <p className="text-sm font-medium">VHost management is available in the root session.</p>
                </div>
            </DashboardLayout>
        );
    }
    return <VHostsShell active={active} />;
}

function VHostsShell({ active }: { active: Section }) {
    const [state, setState] = useState<VhostState | null>(null);
    const [loading, setLoading] = useState(true);
    const [applying, setApplying] = useState<"reconcile" | "reload" | null>(null);
    const [pruning, setPruning] = useState(false);
    const [result, setResult] = useState<ReconcileResult | null>(null);

    const loadState = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetch("/post/vhost/state", { cache: "no-store" });
            setState(res.ok ? normalizeState(await res.json()) : { configured: false, vhostsDir: "", liveReload: false, error: await res.text() });
        } catch (err) {
            setState({ configured: false, vhostsDir: "", liveReload: false, error: String(err) });
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void loadState();
    }, [loadState]);

    const applyReconcile = async (kind: "reconcile" | "reload") => {
        setApplying(kind);
        try {
            const res = await fetch(`/post/vhost/${kind}`, { method: "POST" });
            const data = normalizeResult((await res.json()) as ReconcileResult);
            setResult(data);
            if (data.error) {
                toast.error(data.reloaded ? "Applied with warnings" : summarizeError(data.error));
            } else if (data.reloaded) {
                toast.success(`Caddy reloaded — ${data.written.length} written, ${data.removed.length} removed`);
            } else {
                toast.success("Reconcile complete (no reload needed)");
            }
            await loadState();
        } catch (err) {
            toast.error(`Apply failed: ${String(err)}`);
        } finally {
            setApplying(null);
        }
    };

    const toggleGate = async (enabled: boolean) => {
        try {
            const res = await fetch("/post/vhost/gate", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ enabled }),
            });
            if (!res.ok) {
                toast.error(`Could not ${enabled ? "arm" : "disarm"}: ${(await res.text()).trim() || res.statusText}`);
                return;
            }
            toast.success(enabled ? "Live reconcile armed" : "Live reconcile disarmed");
            await loadState();
        } catch (err) {
            toast.error(`Gate toggle failed: ${String(err)}`);
        }
    };

    const pruneOrphans = async (names: string[]) => {
        setPruning(true);
        try {
            const res = await fetch("/post/vhost/orphan/prune", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ names }),
            });
            const data = normalizeResult((await res.json()) as ReconcileResult);
            setResult(data);
            if (data.error) {
                toast.error(summarizeError(data.error));
            } else {
                toast.success(`Pruned ${names.length} orphan${names.length === 1 ? "" : "s"}`);
            }
            await loadState();
        } catch (err) {
            toast.error(`Prune failed: ${String(err)}`);
        } finally {
            setPruning(false);
        }
    };

    const dry = state?.dryRun;
    const manage = state?.manage;
    const tenantHosts = (dry?.hosts ?? []).filter((h) => h.kind === "tenant");

    return (
        <DashboardLayout title="Caddy VHosts" fullWidth>
            <div className="flex h-full flex-col overflow-hidden">
                {state ? (
                    <ReconcileHeader
                        state={state}
                        applying={applying}
                        result={result}
                        onApply={applyReconcile}
                        onToggleGate={toggleGate}
                        onDismissResult={() => setResult(null)}
                    />
                ) : null}

                <div className="grid min-h-0 flex-1 grid-cols-1 overflow-hidden md:grid-cols-[220px_1fr]">
                    <VHostsSidebar
                        section={active}
                        tenantCount={tenantHosts.length}
                        systemCount={manage?.systemHosts.length ?? 0}
                        redirectCount={manage?.redirects.length ?? 0}
                        orphanCount={dry?.orphans.length ?? 0}
                    />

                    <main className="overflow-y-auto p-6">
                        {loading && !state ? (
                            <div className="flex min-h-48 items-center justify-center">
                                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                            </div>
                        ) : !state?.configured || state.error ? (
                            <ConfiguringNotice state={state} />
                        ) : active === "tenant" ? (
                            <TenantView hosts={tenantHosts} health={state.health ?? {}} />
                        ) : active === "system" ? (
                            <SystemView rows={manage?.systemHosts ?? []} upstreams={manage?.upstreams ?? []} pinned={state.protected ?? []} pinnedWarning={state.protectedWarning} onSaved={loadState} />
                        ) : active === "redirects" ? (
                            <RedirectsView rows={manage?.redirects ?? []} onSaved={loadState} />
                        ) : (
                            <OrphansView orphans={dry?.orphans ?? []} live={state.liveReload} busy={pruning} onPrune={pruneOrphans} onSaved={loadState} />
                        )}
                    </main>
                </div>
            </div>
        </DashboardLayout>
    );
}

function ConfiguringNotice({ state }: { state: VhostState | null }) {
    if (state?.error) {
        return (
            <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                <span>{state.error}</span>
            </div>
        );
    }
    return (
        <EmptyBanner
            title="No active data source"
            body="Add a MySQL data source under Settings → Data Sources and mark it active — every feature reads the single active source."
        />
    );
}
