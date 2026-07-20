import { type FormEvent, useEffect, useState } from "react";
import { Braces, Check, Copy, Loader2, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";

interface APIKey {
    acceptedIps: string[];
    createdAt: string;
    enabled: boolean;
    id: string;
    keyPrefix: string;
    lastUsedAt: string | null;
    name: string;
}

export default function APIsRoute() {
    const [apis, setAPIs] = useState<APIKey[]>([]);
    const [name, setName] = useState("");
    const [acceptedIPs, setAcceptedIPs] = useState("");
    const [createdSecret, setCreatedSecret] = useState("");
    const [copied, setCopied] = useState(false);
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);

    const loadAPIs = async () => {
        try {
            const response = await fetch("/post/apis", { cache: "no-store" });
            if (!response.ok) throw new Error((await response.text()) || "Failed to load API keys");
            const data = await response.json();
            setAPIs(data.apis || []);
            setError("");
        } catch (requestError) {
            setError(requestError instanceof Error ? requestError.message : "Failed to load API keys");
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        void loadAPIs();
    }, []);

    const createAPI = async (event: FormEvent) => {
        event.preventDefault();
        if (!name.trim()) return;
        setSaving(true);
        try {
            const response = await fetch("/post/apis", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ name: name.trim(), acceptedIps: parseAcceptedIPs(acceptedIPs) }),
            });
            if (!response.ok) throw new Error((await response.text()) || "Failed to create API key");
            const data = await response.json();
            setCreatedSecret(data.secret || "");
            setAPIs((current) => [data.api, ...current]);
            setName("");
            setAcceptedIPs("");
            toast.success("API key created");
        } catch (requestError) {
            toast.error(requestError instanceof Error ? requestError.message : "Failed to create API key");
        } finally {
            setSaving(false);
        }
    };

    const updateAPI = async (id: string, changes: { enabled?: boolean; acceptedIps?: string[] }) => {
        const response = await fetch("/post/apis", {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ id, ...changes }),
        });
        if (!response.ok) throw new Error((await response.text()) || "Failed to update API key");
        setAPIs((current) => current.map((api) => api.id === id ? { ...api, ...changes } : api));
    };

    const editAcceptedIPs = async (api: APIKey) => {
        const value = window.prompt("Accepted IP addresses or CIDR ranges, separated by commas. Leave empty to allow all IPs.", api.acceptedIps.join(", "));
        if (value === null) return;
        try {
            await updateAPI(api.id, { acceptedIps: parseAcceptedIPs(value) });
            toast.success("Accepted IPs updated");
        } catch (requestError) {
            toast.error(requestError instanceof Error ? requestError.message : "Failed to update accepted IPs");
        }
    };

    const deleteAPI = async (api: APIKey) => {
        if (!window.confirm(`Delete API key "${api.name}"? This action cannot be undone.`)) return;
        const response = await fetch(`/post/apis?id=${encodeURIComponent(api.id)}`, { method: "DELETE" });
        if (!response.ok) {
            toast.error((await response.text()) || "Failed to delete API key");
            return;
        }
        setAPIs((current) => current.filter((item) => item.id !== api.id));
        toast.success(`Deleted "${api.name}"`);
    };

    const copySecret = async () => {
        await navigator.clipboard.writeText(createdSecret);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
    };

    return (
        <DashboardLayout title="APIs" description="Create and manage API credentials and IP access restrictions.">
            <div className="space-y-4">
                <form onSubmit={createAPI} className="grid gap-3 border border-border bg-card p-4 md:grid-cols-[minmax(180px,1fr)_minmax(260px,2fr)_auto] md:items-end">
                    <label className="space-y-1.5 text-xs font-medium">
                        <span>API Name</span>
                        <input className="h-9 w-full border border-input bg-background px-3 text-sm outline-none focus:border-primary" value={name} onChange={(event) => setName(event.target.value)} maxLength={80} placeholder="Deploy automation" required />
                    </label>
                    <label className="space-y-1.5 text-xs font-medium">
                        <span>Accepted IPs</span>
                        <input className="h-9 w-full border border-input bg-background px-3 font-mono text-xs outline-none focus:border-primary" value={acceptedIPs} onChange={(event) => setAcceptedIPs(event.target.value)} placeholder="203.0.113.10, 10.0.0.0/8 (empty allows all)" />
                    </label>
                    <Button type="submit" disabled={saving} className="gap-2">
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                        Create API Key
                    </Button>
                </form>

                {createdSecret ? (
                    <div className="border border-amber-500/30 bg-amber-500/10 p-4">
                        <p className="text-xs font-semibold text-foreground">Copy this API key now. It will not be shown again.</p>
                        <div className="mt-2 flex items-center gap-2">
                            <code className="min-w-0 flex-1 overflow-x-auto border border-border bg-background px-3 py-2 text-xs">{createdSecret}</code>
                            <Button type="button" size="icon" variant="outline" onClick={copySecret} title="Copy API key">
                                {copied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}
                            </Button>
                        </div>
                    </div>
                ) : null}

                {error ? <p className="border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">{error}</p> : null}

                <div className="overflow-x-auto border border-border bg-card">
                    <table className="w-full min-w-[900px] text-left text-xs">
                        <thead className="border-b border-border bg-muted/40 text-muted-foreground">
                            <tr>
                                <th className="px-4 py-3 font-semibold">Name</th>
                                <th className="px-4 py-3 font-semibold">Key</th>
                                <th className="px-4 py-3 font-semibold">Accepted IPs</th>
                                <th className="px-4 py-3 font-semibold">Status</th>
                                <th className="px-4 py-3 font-semibold">Created</th>
                                <th className="px-4 py-3 text-right font-semibold">Actions</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                            {loading ? (
                                <tr><td colSpan={6} className="p-10 text-center"><Loader2 className="mx-auto h-5 w-5 animate-spin text-muted-foreground" /></td></tr>
                            ) : apis.length === 0 ? (
                                <tr><td colSpan={6} className="p-10 text-center text-muted-foreground">No API keys created.</td></tr>
                            ) : apis.map((api) => (
                                <tr key={api.id} className="hover:bg-muted/20">
                                    <td className="px-4 py-3 font-medium text-foreground">{api.name}</td>
                                    <td className="px-4 py-3 font-mono text-muted-foreground">{api.keyPrefix}••••••••</td>
                                    <td className="max-w-xs px-4 py-3 font-mono text-[11px] text-muted-foreground">{api.acceptedIps.length ? api.acceptedIps.join(", ") : "All IPs"}</td>
                                    <td className="px-4 py-3">
                                        <button type="button" onClick={() => void updateAPI(api.id, { enabled: !api.enabled }).then(() => toast.success(api.enabled ? "API key disabled" : "API key enabled")).catch((requestError) => toast.error(requestError.message))} className={`font-semibold ${api.enabled ? "text-emerald-600 dark:text-emerald-400" : "text-muted-foreground"}`}>{api.enabled ? "Enabled" : "Disabled"}</button>
                                    </td>
                                    <td className="px-4 py-3 text-muted-foreground">{formatDate(api.createdAt)}</td>
                                    <td className="px-4 py-3">
                                        <div className="flex justify-end gap-1">
                                            <Button type="button" size="icon" variant="ghost" onClick={() => void editAcceptedIPs(api)} title="Edit accepted IPs"><Pencil className="h-4 w-4" /></Button>
                                            <Button type="button" size="icon" variant="ghost" className="text-destructive" onClick={() => void deleteAPI(api)} title="Delete API key"><Trash2 className="h-4 w-4" /></Button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>
        </DashboardLayout>
    );
}

function parseAcceptedIPs(value: string) {
    return value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
}

function formatDate(value: string) {
    const date = new Date(value.includes("T") ? value : `${value.replace(" ", "T")}Z`);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
