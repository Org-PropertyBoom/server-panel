import { useCallback, useEffect, useState, type ReactNode } from "react";
import { CheckCircle2, Circle, Loader2, Pencil, Plus, Trash2 } from "lucide-react";
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
    active: boolean;
};

type TestResult = { ok: boolean; error?: string };

type ActiveHealth = { sourceId: string; name: string; ok: boolean; error?: string; checkedAtMs: number };

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
    ["mysql", "MySQL / MariaDB"],
    ["postgres", "PostgreSQL"],
    ["sqlite", "SQLite"],
];

const DEFAULT_PORTS: Record<string, string> = { mysql: "3306", postgres: "5432", sqlite: "" };

// Engine badge: short mono tag tinted per engine (matches the approved design).
const ENGINE_BADGE: Record<string, { text: string; className: string }> = {
    mysql: { text: "SQL", className: "text-sky-500 border-sky-500/30 bg-sky-500/10" },
    postgres: { text: "PG", className: "text-indigo-400 border-indigo-400/30 bg-indigo-400/10" },
    sqlite: { text: "LIT", className: "text-purple-400 border-purple-400/30 bg-purple-400/10" },
};

function engineBadge(engine: string) {
    return ENGINE_BADGE[engine] ?? { text: "DB", className: "text-muted-foreground border-border bg-muted" };
}

function connectionString(s: DataSource): string {
    if (s.engine === "sqlite") return s.database || "(no path)";
    return `${s.user ? `${s.user}@` : ""}${s.host}:${s.port} / ${s.database}`;
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
    const [results, setResults] = useState<Record<string, TestResult>>({});
    const [formTesting, setFormTesting] = useState(false);
    const [formResult, setFormResult] = useState<TestResult | null>(null);
    const [activeHealth, setActiveHealth] = useState<ActiveHealth | null>(null);
    const [activatingId, setActivatingId] = useState<string | null>(null);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetch("/post/datasources", { cache: "no-store" });
            const data = res.ok ? await res.json() : null;
            setSources((data?.dataSources as DataSource[]) ?? []);
            setActiveHealth((data?.activeHealth as ActiveHealth) ?? null);
        } catch {
            setSources([]);
            setActiveHealth(null);
        } finally {
            setLoading(false);
        }
    }, []);

    const activate = async (s: DataSource) => {
        setActivatingId(s.id);
        try {
            const res = await fetch("/post/datasources/activate", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: s.id }),
            });
            if (!res.ok) {
                toast.error(`Could not activate "${s.name}": ${(await res.text()).trim() || res.statusText}`);
                return;
            }
            toast.success(`"${s.name}" is now the active source`);
            await load();
        } catch (err) {
            toast.error(`Could not activate "${s.name}": ${String(err)}`);
        } finally {
            setActivatingId(null);
        }
    };

    useEffect(() => {
        void load();
    }, [load]);

    const startAdd = () => {
        setFormResult(null);
        setDraft(blankDraft());
    };

    const startEdit = (s: DataSource) => {
        setFormResult(null);
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
        if (draft?.id === s.id) setDraft(null);
        await load();
    };

    const runTest = async (id: string, into: "row" | "form") => {
        if (into === "row") setTestingId(id);
        else {
            setFormTesting(true);
            setFormResult(null);
        }
        let result: TestResult;
        try {
            const res = await fetch("/post/datasources/test", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id }),
            });
            result = (await res.json()) as TestResult;
        } catch (err) {
            result = { ok: false, error: String(err) };
        }
        setResults((r) => ({ ...r, [id]: result }));
        if (into === "row") setTestingId(null);
        else {
            setFormResult(result);
            setFormTesting(false);
        }
    };

    const isSqlite = draft?.engine === "sqlite";

    return (
        <div className="mx-auto max-w-2xl space-y-6">
            <div className="flex items-start justify-between gap-4">
                <div>
                    <h2 className="text-lg font-semibold">Data Sources</h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                        Database connections the panel and its features read from. Exactly one is <b>active</b> — every
                        feature reads that single source. Passwords are stored server-side and never shown here.
                    </p>
                </div>
                {draft ? null : (
                    <Button onClick={startAdd} size="sm">
                        <Plus className="mr-2 h-4 w-4" /> Add data source
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
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    {sources.map((s, i) => {
                        const badge = engineBadge(s.engine);
                        const liveResult: TestResult | undefined =
                            s.active && activeHealth ? { ok: activeHealth.ok, error: activeHealth.error } : undefined;
                        return (
                            <div
                                key={s.id}
                                className={`flex items-center gap-3.5 p-4 ${i > 0 ? "border-t border-border" : ""} ${s.active ? "bg-primary/[0.035]" : ""}`}
                            >
                                <button
                                    type="button"
                                    onClick={() => !s.active && activate(s)}
                                    disabled={s.active || activatingId === s.id}
                                    className={`shrink-0 ${s.active ? "cursor-default text-primary" : "text-muted-foreground hover:text-primary"}`}
                                    title={s.active ? "Active source" : "Set as active"}
                                    aria-label={s.active ? "Active source" : `Set ${s.name} active`}
                                >
                                    {activatingId === s.id ? (
                                        <Loader2 className="h-5 w-5 animate-spin" />
                                    ) : s.active ? (
                                        <CheckCircle2 className="h-5 w-5" />
                                    ) : (
                                        <Circle className="h-5 w-5" />
                                    )}
                                </button>
                                <span
                                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-md border font-mono text-[11px] font-bold ${badge.className}`}
                                    title={engineLabel(s.engine)}
                                >
                                    {badge.text}
                                </span>
                                <div className="min-w-0 flex-1">
                                    <p className="flex items-center gap-2 truncate text-sm font-semibold">
                                        {s.name}
                                        {s.active ? (
                                            <span className="rounded-full border border-primary/20 bg-primary/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
                                                Active
                                            </span>
                                        ) : null}
                                    </p>
                                    <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                                        {connectionString(s)}
                                        {s.passwordSet ? "" : " · no password"}
                                    </p>
                                </div>
                                <StatusPill result={results[s.id] ?? liveResult} testing={testingId === s.id} live={!results[s.id] && !!liveResult} />
                                <div className="flex shrink-0 items-center gap-1">
                                    <Button variant="ghost" size="sm" onClick={() => runTest(s.id, "row")} disabled={testingId === s.id}>
                                        {testingId === s.id ? <Loader2 className="h-4 w-4 animate-spin" /> : "Test"}
                                    </Button>
                                    <Button variant="ghost" size="sm" onClick={() => startEdit(s)}>
                                        <Pencil className="mr-1.5 h-3.5 w-3.5" /> Edit
                                    </Button>
                                    <button
                                        type="button"
                                        onClick={() => remove(s)}
                                        className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                                        aria-label={`Remove ${s.name}`}
                                        title={`Remove ${s.name}`}
                                    >
                                        <Trash2 className="h-3.5 w-3.5" />
                                    </button>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}

            {draft ? (
                <div className="overflow-hidden rounded-md border border-border bg-card">
                    <div className="flex items-center gap-2.5 border-b border-border px-4 py-3">
                        <span className="text-[10.5px] font-semibold uppercase tracking-wider text-muted-foreground">
                            {draft.id ? "Edit data source" : "New data source"}
                        </span>
                        {draft.id ? <span className="font-mono text-xs text-muted-foreground">{draft.name}</span> : null}
                    </div>
                    <div className="space-y-4 bg-muted/30 p-4">
                        <div className="grid gap-3.5 sm:grid-cols-2">
                            <Field id="ds-name" label="Name" span2={isSqlite}>
                                <input
                                    id="ds-name"
                                    value={draft.name}
                                    onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                                    placeholder="propertyteam"
                                    className={inputClass}
                                />
                            </Field>
                            <Field id="ds-engine" label="Engine" span2={isSqlite}>
                                <select
                                    id="ds-engine"
                                    value={draft.engine}
                                    onChange={(e) => changeEngine(e.target.value)}
                                    className={inputClass}
                                >
                                    {ENGINES.map(([value, label]) => (
                                        <option key={value} value={value}>
                                            {label}
                                        </option>
                                    ))}
                                </select>
                            </Field>
                            {isSqlite ? (
                                <Field id="ds-database" label="Database file" span2>
                                    <input
                                        id="ds-database"
                                        value={draft.database}
                                        onChange={(e) => setDraft({ ...draft, database: e.target.value })}
                                        placeholder="/var/lib/app/data.db"
                                        className={`${inputClass} font-mono`}
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
                                            className={`${inputClass} font-mono`}
                                        />
                                    </Field>
                                    <Field id="ds-port" label="Port">
                                        <input
                                            id="ds-port"
                                            value={draft.port}
                                            onChange={(e) => setDraft({ ...draft, port: e.target.value })}
                                            inputMode="numeric"
                                            className={`${inputClass} font-mono tabular-nums`}
                                        />
                                    </Field>
                                    <Field id="ds-database2" label="Database">
                                        <input
                                            id="ds-database2"
                                            value={draft.database}
                                            onChange={(e) => setDraft({ ...draft, database: e.target.value })}
                                            placeholder="propertyteam"
                                            className={`${inputClass} font-mono`}
                                        />
                                    </Field>
                                    <Field id="ds-user" label="User">
                                        <input
                                            id="ds-user"
                                            value={draft.user}
                                            onChange={(e) => setDraft({ ...draft, user: e.target.value })}
                                            autoComplete="off"
                                            className={`${inputClass} font-mono`}
                                        />
                                    </Field>
                                    <Field id="ds-password" label="Password" span2>
                                        <input
                                            id="ds-password"
                                            type="password"
                                            value={draft.password}
                                            onChange={(e) => setDraft({ ...draft, password: e.target.value })}
                                            autoComplete="new-password"
                                            placeholder={draft.id ? "•••• (set — leave blank to keep)" : "enter password"}
                                            className={`${inputClass} font-mono`}
                                        />
                                    </Field>
                                </>
                            )}
                        </div>

                        <div className="flex flex-wrap items-center gap-3 pt-1">
                            <Button
                                variant="outline"
                                onClick={() => draft.id && runTest(draft.id, "form")}
                                disabled={!draft.id || formTesting}
                                title={draft.id ? "Test the saved connection" : "Save first to test"}
                            >
                                {formTesting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                                Test Connection
                            </Button>
                            {formResult ? (
                                formResult.ok ? (
                                    <span className="text-sm text-emerald-600 dark:text-emerald-400">✓ Connected</span>
                                ) : (
                                    <span className="text-sm text-destructive">{formResult.error || "Connection failed"}</span>
                                )
                            ) : !draft.id ? (
                                <span className="text-xs text-muted-foreground">Save the source to test it.</span>
                            ) : null}

                            <div className="ml-auto flex items-center gap-2">
                                <Button variant="ghost" onClick={() => setDraft(null)} disabled={saving}>
                                    Cancel
                                </Button>
                                <Button onClick={save} disabled={saving}>
                                    {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                                    Save
                                </Button>
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

const inputClass =
    "h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring";

function engineLabel(engine: string): string {
    return ENGINES.find(([value]) => value === engine)?.[1] ?? engine;
}

function StatusPill({ result, testing, live }: { result?: TestResult; testing: boolean; live?: boolean }) {
    let tone: "ok" | "err" | "neutral" = "neutral";
    let text = "Not tested";
    if (testing) {
        text = "Testing…";
    } else if (result?.ok) {
        tone = "ok";
        text = live ? "Connected · live" : "Connected";
    } else if (result) {
        tone = "err";
        text = truncate(result.error || "Failed", 26);
    }
    const cls =
        tone === "ok"
            ? "text-emerald-600 dark:text-emerald-400 border-emerald-500/20 bg-emerald-500/10"
            : tone === "err"
              ? "text-destructive border-destructive/20 bg-destructive/10"
              : "text-muted-foreground border-border bg-muted";
    return (
        <span
            className={`hidden shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold sm:inline-flex ${cls}`}
            title={result && !result.ok ? result.error : undefined}
        >
            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
            {text}
        </span>
    );
}

function Field({ id, label, span2, children }: { id: string; label: string; span2?: boolean; children: ReactNode }) {
    return (
        <div className={`flex flex-col gap-1.5 ${span2 ? "sm:col-span-2" : ""}`}>
            <label htmlFor={id} className="text-[10.5px] font-semibold uppercase tracking-wider text-muted-foreground">
                {label}
            </label>
            {children}
        </div>
    );
}

function truncate(value: string, max: number): string {
    return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}
