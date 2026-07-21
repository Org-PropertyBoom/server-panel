import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Field, FormActions, inputCls, type ManageRow, Modal } from "./shared";

type RedirectTarget = { domain: string; website?: string; websiteId?: number };

function targetHost(t: string): string {
    try {
        return new URL(t.trim()).hostname.toLowerCase();
    } catch {
        return "";
    }
}

// RedirectForm creates/edits a platform_redirect_hosts row. Shared by the Redirects
// tab and the orphan → Redirect flow. When lockHost is set (orphan conversion) the
// hostname is fixed. Rejects a self-redirect loop (target host == source host).
export default function RedirectForm({
    row,
    lockHost,
    title,
    onClose,
    onSaved,
}: {
    row: ManageRow;
    lockHost?: boolean;
    title?: string;
    onClose: () => void;
    onSaved: () => void;
}) {
    const [host, setHost] = useState(row.host);
    const [target, setTarget] = useState(row.target);
    const [code, setCode] = useState(row.code ?? 301);
    const [isActive, setIsActive] = useState(row.isActive);
    const [saving, setSaving] = useState(false);
    const [targets, setTargets] = useState<RedirectTarget[]>([]);
    const [showSuggest, setShowSuggest] = useState(false);

    useEffect(() => {
        void fetch("/post/vhost/redirect-targets", { cache: "no-store" })
            .then((r) => (r.ok ? r.json() : null))
            .then((d) => setTargets((d?.targets as RedirectTarget[]) ?? []))
            .catch(() => setTargets([]));
    }, []);

    const selfLoop = target.trim() !== "" && targetHost(target) === host.trim().toLowerCase() && host.trim() !== "";

    // Suggestions filtered by what's typed (scheme stripped), excluding the source host.
    const q = target.replace(/^https?:\/\//i, "").toLowerCase().trim();
    const hostLower = host.trim().toLowerCase();
    const suggestions = targets
        .filter((t) => t.domain.toLowerCase() !== hostLower)
        .filter((t) => q === "" || t.domain.toLowerCase().includes(q) || (t.website ?? "").toLowerCase().includes(q))
        .slice(0, 8);

    const save = async () => {
        setSaving(true);
        try {
            const res = await fetch("/post/vhost/redirect", {
                method: row.id === 0 ? "POST" : "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ id: row.id, host: host.trim(), target: target.trim(), code, isActive }),
            });
            if (!res.ok) {
                toast.error((await res.text()).trim() || res.statusText);
                return;
            }
            toast.success(`Redirect for ${host.trim()} saved`);
            onSaved();
        } catch (err) {
            toast.error(`Save failed: ${String(err)}`);
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal onClose={onClose} title={title ?? (row.id === 0 ? "Add redirect" : "Edit redirect")}>
            <div className="space-y-3">
                <Field label="Hostname" hint={lockHost ? "The domain being redirected (from the orphan file)." : undefined}>
                    <input
                        value={host}
                        onChange={(e) => setHost(e.target.value)}
                        placeholder="old.example.com"
                        className={inputCls}
                        readOnly={lockHost}
                        autoFocus={!lockHost}
                    />
                </Field>
                <Field label="Target URL" hint="Type any URL, or pick one of your tenant domains below.">
                    <div className="relative">
                        <input
                            value={target}
                            onChange={(e) => {
                                setTarget(e.target.value);
                                setShowSuggest(true);
                            }}
                            onFocus={() => setShowSuggest(true)}
                            onBlur={() => window.setTimeout(() => setShowSuggest(false), 120)}
                            placeholder="https://new.example.com"
                            className={inputCls}
                            autoFocus={lockHost}
                            autoComplete="off"
                        />
                        {showSuggest && suggestions.length > 0 ? (
                            <ul className="absolute z-10 mt-1 max-h-56 w-full overflow-y-auto rounded-md border border-border bg-card shadow-lg">
                                {suggestions.map((t) => (
                                    <li key={t.domain}>
                                        <button
                                            type="button"
                                            onMouseDown={(e) => {
                                                e.preventDefault();
                                                setTarget(`https://${t.domain}`);
                                                setShowSuggest(false);
                                            }}
                                            className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-xs hover:bg-muted"
                                        >
                                            <span className="font-mono text-foreground">{t.domain}</span>
                                            {t.website ? (
                                                <span className="truncate text-[11px] text-muted-foreground">
                                                    {t.websiteId ? `#${t.websiteId} ` : ""}
                                                    {t.website}
                                                </span>
                                            ) : null}
                                        </button>
                                    </li>
                                ))}
                            </ul>
                        ) : null}
                    </div>
                </Field>
                {selfLoop ? (
                    <p className="text-[11px] text-destructive">Target host equals the source host — that's a redirect loop.</p>
                ) : null}
                <Field label="Status code">
                    <select value={code} onChange={(e) => setCode(Number(e.target.value))} className={inputCls}>
                        <option value={301}>301 — Permanent</option>
                        <option value={302}>302 — Found (temporary)</option>
                        <option value={307}>307 — Temporary (keep method)</option>
                        <option value={308}>308 — Permanent (keep method)</option>
                    </select>
                </Field>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                    Active
                </label>
            </div>
            <FormActions saving={saving} onCancel={onClose} onSave={save} disabled={!host.trim() || !target.trim() || selfLoop} />
        </Modal>
    );
}
