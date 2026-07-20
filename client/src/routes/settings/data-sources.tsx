import { useCallback, useEffect, useState, type ReactNode } from "react";
import { Loader2, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";

type DataSource = {
    id: string;
    name: string;
    engine: string;
    host: string;
    port: string;
    database: string;
    user: string;
    passwordSet: boolean;
};

type TestResult = { ok: boolean; error?: string };

type Draft = {
    id?: string;
    name: string;
    engine: string;
    host: string;
    port: string;
    database: string;
    user: string;
    password: string;
};

const ENGINES: Array<[string, string]> = [
    ["mysql", "MySQL"],
    ["postgres", "PostgreSQL"],
    ["sqlite", "SQLite"],
];

const DEFAULT_PORTS: Record<string, string> = { mysql: "3306", postgres: "5432", sqlite: "" };

function engineLabel(engine: string): string {
    return ENGINES.find(([value]) => value === engine)?.[1] ?? engine;
}

function blankDraft(): Draft {
    return { name: "", engine: "mysql", host: "127.0.0.1", port: "3306", database: "propertyteam", user: "root", password: "" };
}

export default function DataSourcesSection() {
    const [sources, setSources] = useState<DataSource[]>([]);
    const [loading, setLoading] = useState(true);
    const [draft, setDraft] = useState<Draft | null>(null);
    const [saving, setSaving] = useState(false);
    const [testingId, setTestingId] = useState<string | null>(null);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetch("/post/datasources", { cache: "no-store" });
            const data = res.ok ? await res.json() : null;
            setSources((data?.dataSources as DataSource[]) ?? []);
        } catch {
            setSources([]);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    const startAdd = () => {
        setDraft(blankDraft());
    };

    const startEdit = (s: DataSource) => {
        setDraft({ id: s.id, name: s.name, engine: s.engine, host: s.host, port: s.port, database: s.database, user: s.user, password: "" });
    };

    const changeEngine = (engine: string) => {
        setDraft((d) => (d ? { ...d, engine, port: DEFAULT_PORTS[engine] ?? d.port } : d));
    };

    const save = async () => {
        if (!draft) return;
        setSaving(true);
        try {
            const res = await fetch("/post/datasources", {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(draft),
            });
            if (!res.ok) {
                toast.error(`Save failed: ${(await res.text()).trim() || res.statusText}`);
                return;
            }
            toast.success(`Data source "${draft.name}" saved`);
            setDraft(null);
            await load();
        } catch (err) {
            toast.error(`Save failed: ${String(err)}`);
        } finally {
            setSaving(false);
        }
    };

    const remove = async (s: DataSource) => {
        if (!window.confirm(`Remove data source "${s.name}"?`)) return;
        try {
            const res = await fetch(`/post/datasources?id=${encodeURIComponent(s.id)}`, { method: "DELETE" });
            if (res.ok) toast.success(`Removed "${s.name}"`);
            else toast.error(`Failed to remove "${s.name}": ${(await res.text()).trim() || res.statusText}`);
        } catch (err) {
            toast.error(`Failed to remove "${s.name}": ${String(err)}`);
        }
        await load();
    };

    const test = async (s: DataSource) => {
        setTestingId(s.id);
        const toastId = toast.loading(`Testing "${s.name}"…`);
        try {
            const res = await fetch("/post/datasources/test", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: s.id }),
            });
            const result = (await res.json()) as TestResult;
            if (result.ok) toast.success(`"${s.name}" — connected`, { id: toastId });
            else toast.error(`"${s.name}" — ${result.error || "connection failed"}`, { id: toastId });
        } catch (err) {
            toast.error(`"${s.name}" — ${String(err)}`, { id: toastId });
        } finally {
            setTestingId(null);
        }
    };

    const isSqlite = draft?.engine === "sqlite";

    return (
        <div className="mx-auto max-w-2xl space-y-6">
            <div className="flex items-start justify-between gap-4">
                <div>
                    <h2 className="text-lg font-semibold">Data Sources</h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                        Named database connections that panel features (such as Caddy vhost management) consume by
                        name. Passwords are stored server-side and never shown here.
                    </p>
                </div>
                {draft ? null : (
                    <Button onClick={startAdd} size="sm">
                        <Plus className="mr-2 h-4 w-4" /> Add
                    </Button>
                )}
            </div>

            {loading ? (
                <p className="text-sm text-muted-foreground">Loading data sources…</p>
            ) : sources.length === 0 && !draft ? (
                <p className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
                    No data sources yet. Add one to connect a database.
                </p>
            ) : (
                <div className="space-y-3">
                    {sources.map((s) => {
                        return (
                            <div key={s.id} className="rounded-md border border-border bg-card p-4">
                                <div className="flex flex-wrap items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="flex items-center gap-2">
                                            <span className="font-medium">{s.name}</span>
                                            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                                                {engineLabel(s.engine)}
                                            </span>
                                        </div>
                                        <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                                            {s.engine === "sqlite"
                                                ? s.database
                                                : `${s.user ? `${s.user}@` : ""}${s.host}:${s.port}/${s.database}`}
                                            {s.passwordSet ? " · password set" : " · no password"}
                                        </p>
                                    </div>
                                    <div className="flex shrink-0 items-center gap-1">
                                        <Button variant="outline" size="sm" onClick={() => test(s)} disabled={testingId === s.id}>
                                            {testingId === s.id ? <Loader2 className="h-4 w-4 animate-spin" /> : "Test"}
                                        </Button>
                                        <button
                                            type="button"
                                            onClick={() => startEdit(s)}
                                            className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border hover:bg-muted"
                                            aria-label={`Edit ${s.name}`}
                                        >
                                            <Pencil className="h-3.5 w-3.5" />
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => remove(s)}
                                            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-destructive hover:bg-destructive/10"
                                            aria-label={`Remove ${s.name}`}
                                        >
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </button>
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}

            {draft ? (
                <div className="space-y-4 rounded-md border border-border bg-card p-4">
                    <h3 className="text-sm font-semibold">{draft.id ? "Edit data source" : "New data source"}</h3>
                    <div className="divide-y divide-border rounded-md border border-border">
                        <Field id="ds-name" label="Name">
                            <input
                                id="ds-name"
                                value={draft.name}
                                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                                placeholder="propertyteam"
                                className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                            />
                        </Field>
                        <Field id="ds-engine" label="Engine">
                            <select
                                id="ds-engine"
                                value={draft.engine}
                                onChange={(e) => changeEngine(e.target.value)}
                                className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                            >
                                {ENGINES.map(([value, label]) => (
                                    <option key={value} value={value}>
                                        {label}
                                    </option>
                                ))}
                            </select>
                        </Field>
                        {isSqlite ? (
                            <Field id="ds-database" label="Database file">
                                <input
                                    id="ds-database"
                                    value={draft.database}
                                    onChange={(e) => setDraft({ ...draft, database: e.target.value })}
                                    placeholder="/var/lib/app/data.db"
                                    className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                />
                            </Field>
                        ) : (
                            <>
                                <Field id="ds-host" label="Host">
                                    <input
                                        id="ds-host"
                                        value={draft.host}
                                        onChange={(e) => setDraft({ ...draft, host: e.target.value })}
                                        placeholder="127.0.0.1"
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </Field>
                                <Field id="ds-port" label="Port">
                                    <input
                                        id="ds-port"
                                        value={draft.port}
                                        onChange={(e) => setDraft({ ...draft, port: e.target.value })}
                                        inputMode="numeric"
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </Field>
                                <Field id="ds-database2" label="Database">
                                    <input
                                        id="ds-database2"
                                        value={draft.database}
                                        onChange={(e) => setDraft({ ...draft, database: e.target.value })}
                                        placeholder="propertyteam"
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </Field>
                                <Field id="ds-user" label="Username">
                                    <input
                                        id="ds-user"
                                        value={draft.user}
                                        onChange={(e) => setDraft({ ...draft, user: e.target.value })}
                                        autoComplete="off"
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </Field>
                                <Field id="ds-password" label="Password">
                                    <input
                                        id="ds-password"
                                        type="password"
                                        value={draft.password}
                                        onChange={(e) => setDraft({ ...draft, password: e.target.value })}
                                        autoComplete="new-password"
                                        placeholder={draft.id ? "•••••••• (unchanged)" : "enter password"}
                                        className="h-9 rounded-md border border-input bg-background px-3 font-mono text-sm outline-none focus:ring-1 focus:ring-ring"
                                    />
                                </Field>
                            </>
                        )}
                    </div>
                    <div className="flex items-center gap-3">
                        <Button onClick={save} disabled={saving}>
                            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                            Save
                        </Button>
                        <Button variant="ghost" onClick={() => setDraft(null)} disabled={saving}>
                            Cancel
                        </Button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

function Field({ id, label, children }: { id: string; label: string; children: ReactNode }) {
    return (
        <div className="grid gap-3 p-4 sm:grid-cols-[180px_1fr] sm:items-center">
            <label htmlFor={id} className="text-sm font-medium">
                {label}
            </label>
            {children}
        </div>
    );
}
