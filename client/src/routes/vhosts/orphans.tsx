import { useState } from "react";
import { CornerUpRight, ExternalLink, Trash2 } from "lucide-react";

import { Button } from "_layouts/_components/ui/button";
import { EmptyBanner, Modal, ViewHeader } from "./shared";
import RedirectForm from "./redirect-form";

// hostOf strips the ".caddy" suffix so an orphan file maps to its hostname.
function hostOf(name: string): string {
    return name.endsWith(".caddy") ? name.slice(0, -".caddy".length) : name;
}

// OrphansView lists on-disk `<host>.caddy` files with no active DB row. Prune is
// deliberate — per-file or a bulk select+confirm; there is NO blind "prune all",
// and protected/wildcard files are refused server-side.
export default function OrphansView({
    orphans,
    live,
    busy,
    onPrune,
    onSaved,
}: {
    orphans: string[];
    live: boolean;
    busy: boolean;
    onPrune: (names: string[]) => Promise<void>;
    onSaved: () => void;
}) {
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [confirm, setConfirm] = useState<string[] | null>(null);
    const [redirectHost, setRedirectHost] = useState<string | null>(null);

    const toggle = (name: string) =>
        setSelected((prev) => {
            const next = new Set(prev);
            next.has(name) ? next.delete(name) : next.add(name);
            return next;
        });
    const allSelected = orphans.length > 0 && selected.size === orphans.length;
    const toggleAll = () => setSelected(allSelected ? new Set() : new Set(orphans));

    const run = async (names: string[]) => {
        setConfirm(null);
        await onPrune(names);
        setSelected(new Set());
    };

    if (orphans.length === 0) {
        return (
            <div>
                <ViewHeader title="Orphans" subtitle="On-disk vhost files with no active database row." />
                <EmptyBanner title="No orphans" body="Every vhost file on disk maps to an active row. Nothing to clean up." />
            </div>
        );
    }

    return (
        <div>
            <ViewHeader
                title="Orphans"
                subtitle="On-disk vhost files with no active database row. Open to check if a site still serves, convert a moved site to a 301 redirect, or prune deliberately."
                actions={
                    <div className="flex items-center gap-2">
                        <Button variant="outline" size="sm" onClick={toggleAll}>
                            {allSelected ? "Clear" : "Select all"}
                        </Button>
                        <Button
                            variant="outline"
                            size="sm"
                            className="gap-1.5 text-destructive hover:bg-destructive/10"
                            disabled={selected.size === 0 || busy}
                            onClick={() => setConfirm(Array.from(selected))}
                        >
                            <Trash2 className="h-3.5 w-3.5" />
                            Prune selected ({selected.size})
                        </Button>
                    </div>
                }
            />
            {!live ? (
                <p className="mb-3 text-[11px] text-amber-600 dark:text-amber-400">
                    Live reconcile is gated — pruning returns the gate message and removes nothing until an operator arms it.
                </p>
            ) : null}
            <div className="overflow-hidden rounded-md border border-destructive/25 bg-destructive/[0.03]">
                <ul className="divide-y divide-border">
                    {orphans.map((name) => (
                        <li key={name} className="flex items-center gap-3 px-4 py-2.5">
                            <input type="checkbox" checked={selected.has(name)} onChange={() => toggle(name)} className="h-3.5 w-3.5" />
                            <span className="flex-1 font-mono text-xs text-foreground">{name}</span>
                            <a
                                href={`https://${hostOf(name)}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium text-muted-foreground hover:bg-muted hover:text-primary"
                                title={`Open https://${hostOf(name)} in a new tab — check if it still serves before pruning`}
                            >
                                <ExternalLink className="h-3.5 w-3.5" />
                                Open
                            </a>
                            <Button
                                variant="ghost"
                                size="sm"
                                className="gap-1.5"
                                onClick={() => setRedirectHost(hostOf(name))}
                                title="Convert to a 301 redirect (preserves old links/SEO) instead of deleting"
                            >
                                <CornerUpRight className="h-3.5 w-3.5" />
                                Redirect
                            </Button>
                            <Button
                                variant="ghost"
                                size="sm"
                                className="gap-1.5 text-destructive hover:bg-destructive/10"
                                disabled={busy}
                                onClick={() => setConfirm([name])}
                            >
                                <Trash2 className="h-3.5 w-3.5" />
                                Prune
                            </Button>
                        </li>
                    ))}
                </ul>
            </div>

            {confirm ? (
                <Modal onClose={() => setConfirm(null)} title={`Prune ${confirm.length} orphan file${confirm.length === 1 ? "" : "s"}`}>
                    <p className="text-xs text-muted-foreground">
                        These files are removed from disk, then Caddy is re-validated and reloaded. The removal is recorded as intentional so the
                        outage guard allows it. Protected/wildcard files are refused server-side.
                    </p>
                    <ul className="mt-3 max-h-40 space-y-0.5 overflow-y-auto rounded-md border border-border bg-muted/30 p-2.5">
                        {confirm.map((n) => (
                            <li key={n} className="truncate font-mono text-[11px] text-foreground">
                                {n}
                            </li>
                        ))}
                    </ul>
                    <div className="mt-5 flex justify-end gap-2">
                        <Button variant="outline" size="sm" onClick={() => setConfirm(null)}>
                            Cancel
                        </Button>
                        <Button variant="destructive" size="sm" className="gap-2" onClick={() => run(confirm)}>
                            <Trash2 className="h-4 w-4" />
                            Prune {confirm.length}
                        </Button>
                    </div>
                </Modal>
            ) : null}

            {redirectHost ? (
                <RedirectForm
                    row={{ id: 0, host: redirectHost, target: "", code: 301, isActive: true, softDeleted: false }}
                    lockHost
                    title={`Redirect ${redirectHost}`}
                    onClose={() => setRedirectHost(null)}
                    onSaved={() => {
                        setRedirectHost(null);
                        void onSaved();
                    }}
                />
            ) : null}
        </div>
    );
}
