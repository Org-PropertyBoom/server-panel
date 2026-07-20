import { useCallback, useEffect, useMemo, useState } from "react";
import { ExternalLink, Globe, Loader2, RefreshCw, Search, ShieldCheck, ShieldOff } from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import { runtime } from "../../runtime";

type CaddyVHost = {
    aliases: string[];
    hostname: string;
    listen: string[];
    server: "caddy";
    tls: boolean;
};

export default function VHostsRoute() {
    const [vhosts, setVhosts] = useState<CaddyVHost[]>([]);
    const [query, setQuery] = useState("");
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");
    const endpoint = runtime.isRoot ? "/post/vhost/list" : "/api/vhost/list";

    const loadVHosts = useCallback(async () => {
        setLoading(true);
        setError("");
        try {
            const response = await fetch(endpoint, { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load Caddy virtual hosts");
            const data: { vhosts?: CaddyVHost[] } = await response.json();
            setVhosts((data.vhosts ?? []).filter((vhost) => vhost.server === "caddy"));
        } catch (loadError) {
            setError(loadError instanceof Error ? loadError.message : "Failed to load Caddy virtual hosts");
        } finally {
            setLoading(false);
        }
    }, [endpoint]);

    useEffect(() => {
        loadVHosts();
    }, [loadVHosts]);

    const filtered = useMemo(() => {
        const value = query.trim().toLowerCase();
        if (!value) return vhosts;
        return vhosts.filter((vhost) => vhost.hostname.toLowerCase().includes(value) || vhost.aliases.some((alias) => alias.toLowerCase().includes(value)));
    }, [query, vhosts]);

    return (
        <DashboardLayout
            title="Caddy VHosts"
            description="View public hosts and routes loaded from the system Caddyfile."
            wide
            actions={
                <Button variant="outline" size="sm" className="gap-2" onClick={loadVHosts} disabled={loading}>
                    <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
                    Refresh
                </Button>
            }
        >
            <div className="space-y-4">
                <div className="flex h-10 max-w-md items-center gap-2 rounded-md border border-border bg-card px-3">
                    <Search className="h-4 w-4 text-muted-foreground" />
                    <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search Caddy hosts..." className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground" />
                </div>

                {error ? <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error.trim()}</div> : null}

                {loading ? (
                    <div className="flex min-h-64 items-center justify-center rounded-md border border-border bg-card"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>
                ) : filtered.length === 0 ? (
                    <div className="flex min-h-64 flex-col items-center justify-center rounded-md border border-dashed border-border text-center">
                        <Globe className="mb-3 h-9 w-9 text-muted-foreground/40" />
                        <p className="text-sm font-medium text-foreground">No Caddy virtual hosts found</p>
                        <p className="mt-1 text-xs text-muted-foreground">Add a host block to /etc/caddy/Caddyfile, then reload Caddy.</p>
                    </div>
                ) : (
                    <div className="overflow-hidden rounded-md border border-border bg-card">
                        <div className="overflow-x-auto">
                            <table className="w-full min-w-[760px] text-left text-xs">
                                <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                                    <tr>
                                        <th className="px-4 py-3 font-medium">Hostname</th>
                                        <th className="px-4 py-3 font-medium">Aliases</th>
                                        <th className="px-4 py-3 font-medium">Listen</th>
                                        <th className="px-4 py-3 font-medium">TLS</th>
                                        <th className="px-4 py-3 text-right font-medium">Open</th>
                                    </tr>
                                </thead>
                                <tbody className="divide-y divide-border">
                                    {filtered.map((vhost) => (
                                        <tr key={vhost.hostname} className="hover:bg-muted/30">
                                            <td className="px-4 py-3 font-medium text-foreground"><span className="flex items-center gap-2"><Globe className="h-4 w-4 text-primary" />{vhost.hostname}</span></td>
                                            <td className="px-4 py-3 text-muted-foreground">{vhost.aliases.length ? vhost.aliases.join(", ") : "—"}</td>
                                            <td className="px-4 py-3 font-mono text-muted-foreground">{vhost.listen.length ? vhost.listen.join(", ") : ":80, :443"}</td>
                                            <td className="px-4 py-3">
                                                <span className={`inline-flex items-center gap-1.5 ${vhost.tls ? "text-emerald-600 dark:text-emerald-400" : "text-muted-foreground"}`}>
                                                    {vhost.tls ? <ShieldCheck className="h-3.5 w-3.5" /> : <ShieldOff className="h-3.5 w-3.5" />}
                                                    {vhost.tls ? "Automatic" : "HTTP"}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-right">
                                                <a href={`${vhost.tls ? "https" : "http"}://${vhost.hostname}`} target="_blank" rel="noreferrer" className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border text-muted-foreground hover:bg-muted hover:text-foreground" aria-label={`Open ${vhost.hostname}`} title={`Open ${vhost.hostname}`}>
                                                    <ExternalLink className="h-3.5 w-3.5" />
                                                </a>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    </div>
                )}
            </div>
        </DashboardLayout>
    );
}
