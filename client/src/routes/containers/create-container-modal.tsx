import { useState } from "react";
import { CheckCircle2, Loader2, Plus, X } from "lucide-react";
import { toast } from "sonner";

import { Button } from "_layouts/_components/ui/button";
import Api from "_utils/api";

// New container flow — Configure → Review.
// Spec: design-templates designs/shared/panel-new-container.md @ 301c1ad. The modal writes a
// docker-compose.yml under /home/server/containers/<name>/ and runs `docker compose
// up -d` there — never `docker run`. A run command evaporates once issued; the file
// persists, editable and diffable, and survives a host rebuild.
//
// The safety point: `-p 9001:8080` publishes on 0.0.0.0 — every interface,
// including the public one. That silent default is how MinIO's console, openinary
// and mysql:3306 became reachable from the internet. Every app here sits behind
// Caddy, so loopback is correct in nearly every case: it is the default, and
// public is visible, named, and chosen each time.

type Plan = { name: string; path: string; compose: string; warnings: string[]; blocks: string[] };

export default function CreateContainerModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
    const [step, setStep] = useState<"configure" | "review">("configure");
    const [image, setImage] = useState("");
    const [name, setName] = useState("");
    const [ports, setPorts] = useState("");
    const [env, setEnv] = useState("");
    const [envFile, setEnvFile] = useState("");
    const [envMode, setEnvMode] = useState<"file" | "inline">("file");
    const [volumes, setVolumes] = useState("");
    const [restart, setRestart] = useState("unless-stopped");
    const [network, setNetwork] = useState<"bridge" | "host" | "none">("bridge");
    // Loopback default. Deliberately NOT remembered between uses — the whole value
    // is that exposure is chosen each time.
    const [scope, setScope] = useState<"127.0.0.1" | "0.0.0.0">("127.0.0.1");
    const [acceptPublic, setAcceptPublic] = useState(false);
    const [confirmSock, setConfirmSock] = useState(false);
    // §9 resource caps — empty by default. openinary_processor leaked to ~1.5 GB
    // uncapped on this host, starving everything sharing it.
    const [memLimit, setMemLimit] = useState("");
    const [cpus, setCpus] = useState("");
    // §10 privileged — the same host access as docker.sock by another route.
    const [privileged, setPrivileged] = useState(false);
    const [confirmPrivileged, setConfirmPrivileged] = useState(false);
    const [alwaysPull, setAlwaysPull] = useState(false);
    const [plan, setPlan] = useState<Plan | null>(null);
    const [planError, setPlanError] = useState("");
    const [planning, setPlanning] = useState(false);
    const [creating, setCreating] = useState(false);
    const [output, setOutput] = useState("");

    const lines = (value: string) => value.split("\n").map((l) => l.trim()).filter(Boolean);
    const portsDisabled = network !== "bridge";

    // A line that already carries an explicit bind address is passed through
    // untouched — never rewrite what the operator spelled out.
    const hasOwnBind = (line: string) => /^\d{1,3}(\.\d{1,3}){3}:/.test(line);
    const scopedPorts = () => lines(ports).map((p) => (hasOwnBind(p) || scope === "0.0.0.0" ? p : `127.0.0.1:${p}`));
    const overridden = lines(ports).some(hasOwnBind);
    const publicPorts = portsDisabled || scope !== "0.0.0.0" ? [] : lines(ports).filter((p) => !hasOwnBind(p));

    const spec = () => ({
        image: image.trim(),
        name: name.trim(),
        ports: portsDisabled ? [] : scopedPorts(),
        env: envMode === "inline" ? lines(env) : [],
        envFile: envMode === "file" ? envFile.trim() : "",
        volumes: lines(volumes),
        restart,
        network,
        confirmDockerSock: confirmSock,
        memLimit: memLimit.trim(),
        cpus: cpus.trim(),
        privileged,
        confirmPrivileged,
        alwaysPull,
    });

    const review = async () => {
        setPlanning(true);
        setPlanError("");
        try {
            const res = await fetch(`${Api.current.containers}/plan`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(spec()),
            });
            const data: { plan?: Plan; error?: string } = await res.json();
            if (data.error) {
                setPlanError(data.error);
                return;
            }
            setPlan({ name: data.plan?.name ?? "", path: data.plan?.path ?? "", compose: data.plan?.compose ?? "", warnings: data.plan?.warnings ?? [], blocks: data.plan?.blocks ?? [] });
            setStep("review");
        } catch (err) {
            setPlanError(err instanceof Error ? err.message : "Could not build the compose file");
        } finally {
            setPlanning(false);
        }
    };

    const create = async () => {
        setCreating(true);
        setOutput("");
        try {
            const response = await fetch(`${Api.current.containers}/create`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(spec()),
            });
            const data: { output?: string; error?: string } = await response.json();
            if (data.error) {
                setOutput(data.output || "");
                toast.error(data.error);
                return;
            }
            toast.success(`${name.trim() || "Container"} created`);
            onCreated();
        } catch (createError) {
            toast.error(createError instanceof Error ? createError.message : "Failed to create container");
        } finally {
            setCreating(false);
        }
    };

    const busy = creating || planning;
    const canReview = Boolean(image.trim()) && (publicPorts.length === 0 || acceptPublic);
    const inputCls = "w-full rounded-md border border-border bg-background px-3 py-2 font-mono outline-none focus:border-primary";

    return (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm" onClick={() => (busy ? null : onClose())}>
            <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-md border border-border bg-card shadow-xl" onClick={(e) => e.stopPropagation()}>
                <div className="flex items-center justify-between border-b border-border px-5 py-4">
                    <div>
                        <h2 className="text-sm font-semibold text-foreground">
                            New container
                            <span className="ml-1.5 text-[11px] font-normal text-muted-foreground">
                                {step === "configure" ? "· 1 of 2 · Configure" : "· 2 of 2 · Review"}
                            </span>
                        </h2>
                        {/* Conservation: the meaning must survive — only the command
                            name is reworded, because the flow no longer uses docker run. */}
                        <p className="mt-0.5 text-xs text-muted-foreground">
                            Runs <code>docker compose up -d</code> as root. For stack apps, use their deploy pipeline instead.
                        </p>
                    </div>
                    <Button size="icon" variant="ghost" className="h-8 w-8" onClick={onClose} disabled={busy} aria-label="Close">
                        <X className="h-4 w-4" />
                    </Button>
                </div>

                {step === "configure" ? (
                    <div className="min-h-0 flex-1 space-y-4 overflow-auto px-5 py-4 text-xs">
                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">
                                Image <span className="text-destructive">*</span>
                            </span>
                            <input value={image} onChange={(e) => setImage(e.target.value)} placeholder="nocodb/nocodb:latest" autoFocus className={inputCls} />
                        </label>

                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">Name</span>
                            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="optional — derived from the image if empty" className={inputCls} />
                        </label>

                        <div>
                            <span className="mb-1 block font-medium text-foreground">Network</span>
                            <div className="flex flex-wrap gap-4">
                                {(["bridge", "host", "none"] as const).map((n) => (
                                    <label key={n} className="inline-flex items-center gap-1.5 text-muted-foreground">
                                        <input type="radio" name="network" checked={network === n} onChange={() => setNetwork(n)} />
                                        <span className={network === n ? "text-foreground" : ""}>
                                            {n}
                                            {n === "bridge" ? " (default)" : ""}
                                        </span>
                                    </label>
                                ))}
                            </div>
                            {portsDisabled ? (
                                <p className="mt-1 text-[11px] text-muted-foreground">
                                    With {network} networking the container uses the host&apos;s ports directly — port mapping does not apply.
                                </p>
                            ) : null}
                        </div>

                        <div className={portsDisabled ? "pointer-events-none opacity-40" : ""}>
                            <span className="mb-1 block font-medium text-foreground">Ports</span>
                            <textarea
                                value={ports}
                                onChange={(e) => setPorts(e.target.value)}
                                rows={2}
                                disabled={portsDisabled}
                                placeholder={"one per line\n9001:8080"}
                                className={`${inputCls} resize-none disabled:cursor-not-allowed`}
                            />
                            <span className="mt-1 block text-[11px] text-muted-foreground">host:container</span>

                            <div className="mt-2 space-y-1">
                                <span className="block text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Publish on</span>
                                <label className="flex items-center gap-2 text-muted-foreground">
                                    <input
                                        type="radio"
                                        name="scope"
                                        checked={scope === "127.0.0.1"}
                                        disabled={portsDisabled}
                                        onChange={() => {
                                            setScope("127.0.0.1");
                                            setAcceptPublic(false);
                                        }}
                                    />
                                    <span className="font-mono text-foreground">127.0.0.1</span>
                                    <span>private — reachable through Caddy only</span>
                                </label>
                                <label className="flex items-center gap-2 text-muted-foreground">
                                    <input type="radio" name="scope" checked={scope === "0.0.0.0"} disabled={portsDisabled} onChange={() => setScope("0.0.0.0")} />
                                    <span className="font-mono text-foreground">0.0.0.0</span>
                                    <span className="text-amber-600 dark:text-amber-400">PUBLIC — reachable from the internet ⚠</span>
                                </label>

                                {overridden ? (
                                    <p className="text-[11px] text-muted-foreground">
                                        A line with its own bind address is passed through unchanged — the setting above does not apply to it.
                                    </p>
                                ) : null}

                                {publicPorts.length > 0 ? (
                                    <label className="mt-1 flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
                                        <input type="checkbox" checked={acceptPublic} onChange={(e) => setAcceptPublic(e.target.checked)} className="mt-0.5" />
                                        <span>
                                            Port {publicPorts.map((p) => p.split(":")[0]).join(", ")} will be reachable from any address on the internet. I intend this.
                                        </span>
                                    </label>
                                ) : null}
                            </div>
                        </div>

                        <div>
                            <span className="mb-1 block font-medium text-foreground">Environment</span>
                            <div className="mb-1.5 flex gap-4">
                                {(["file", "inline"] as const).map((m) => (
                                    <label key={m} className="inline-flex items-center gap-1.5 text-muted-foreground">
                                        <input type="radio" name="envmode" checked={envMode === m} onChange={() => setEnvMode(m)} />
                                        <span className={envMode === m ? "text-foreground" : ""}>{m === "file" ? "From file" : "Inline"}</span>
                                    </label>
                                ))}
                            </div>
                            {envMode === "file" ? (
                                <>
                                    <input value={envFile} onChange={(e) => setEnvFile(e.target.value)} placeholder="/home/server/gateway-service.env" className={inputCls} />
                                    <span className="mt-1 block text-[11px] text-muted-foreground">
                                        Passed as <code>--env-file</code>. The panel never reads or stores the contents — a path reference keeps secrets out of the page and out of logs.
                                    </span>
                                </>
                            ) : (
                                <>
                                    <textarea value={env} onChange={(e) => setEnv(e.target.value)} rows={3} placeholder={"one per line\nKEY=VALUE"} className={`${inputCls} resize-none`} />
                                    <span className="mt-1 block text-[11px] text-muted-foreground">For non-secret values — inline values are visible in the page.</span>
                                </>
                            )}
                        </div>

                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">Volumes</span>
                            <textarea value={volumes} onChange={(e) => setVolumes(e.target.value)} rows={2} placeholder={"one per line\n/data/nocodb:/usr/app/data"} className={`${inputCls} resize-none`} />
                            <span className="mt-1 block text-[11px] text-muted-foreground">src:dst[:ro]</span>
                        </label>

                        <label className="block">
                            <span className="mb-1 block font-medium text-foreground">Restart policy</span>
                            <select value={restart} onChange={(e) => setRestart(e.target.value)} className="w-full rounded-md border border-border bg-background px-3 py-2 outline-none focus:border-primary">
                                <option value="unless-stopped">unless-stopped</option>
                                <option value="always">always</option>
                                <option value="on-failure">on-failure</option>
                                <option value="no">no</option>
                            </select>
                        </label>

                        <div className="grid grid-cols-2 gap-3">
                            <label className="block">
                                <span className="mb-1 block font-medium text-foreground">Memory limit</span>
                                <input value={memLimit} onChange={(e) => setMemLimit(e.target.value)} placeholder="512m" className={inputCls} />
                            </label>
                            <label className="block">
                                <span className="mb-1 block font-medium text-foreground">CPU limit</span>
                                <input value={cpus} onChange={(e) => setCpus(e.target.value)} placeholder="1.5" className={inputCls} />
                            </label>
                        </div>
                        <p className="-mt-2 text-[11px] text-muted-foreground">
                            No limit means this container can consume all host RAM. Every site on this box shares it.
                        </p>

                        <label className="flex items-start gap-2 text-muted-foreground">
                            <input type="checkbox" checked={alwaysPull} onChange={(e) => setAlwaysPull(e.target.checked)} className="mt-0.5" />
                            <span>
                                Always pull the image <span className="text-[11px]">— <code>pull_policy: always</code>, so a restart takes the newest image rather than a stale local copy.</span>
                            </span>
                        </label>

                        <label className="flex items-start gap-2 text-muted-foreground">
                            <input
                                type="checkbox"
                                checked={privileged}
                                onChange={(e) => {
                                    setPrivileged(e.target.checked);
                                    if (!e.target.checked) setConfirmPrivileged(false);
                                }}
                                className="mt-0.5"
                            />
                            <span>
                                Privileged mode <span className="text-[11px] text-amber-600 dark:text-amber-400">— grants effectively full host access, the same as mounting docker.sock.</span>
                            </span>
                        </label>
                        {privileged ? (
                            <label className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-[11px] text-destructive">
                                <input type="checkbox" checked={confirmPrivileged} onChange={(e) => setConfirmPrivileged(e.target.checked)} className="mt-0.5" />
                                <span>I understand this grants effectively full host access, and intend it.</span>
                            </label>
                        ) : null}

                        {planError ? <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-destructive">{planError}</p> : null}
                    </div>
                ) : (
                    <div className="min-h-0 flex-1 space-y-3 overflow-auto px-5 py-4 text-xs">
                        <div>
                            <div className="mb-1 flex items-center justify-between gap-3">
                                <span className="min-w-0 font-medium text-foreground">
                                    This file will be written
                                    <code className="ml-1.5 break-all font-normal text-[11px] text-muted-foreground">{plan?.path}</code>
                                </span>
                                <button
                                    type="button"
                                    onClick={() =>
                                        navigator.clipboard.writeText(plan?.compose ?? "").then(
                                            () => toast.success("Compose file copied"),
                                            () => toast.error("Could not copy"),
                                        )
                                    }
                                    className="shrink-0 rounded border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground hover:bg-muted hover:text-foreground"
                                >
                                    Copy
                                </button>
                            </div>
                            <pre className="overflow-x-auto whitespace-pre rounded-md border border-border bg-zinc-950 p-3 font-mono text-[11px] leading-5 text-zinc-200">{plan?.compose}</pre>
                            <p className="mt-1 text-[11px] text-muted-foreground">
                                Then <code>docker compose up -d</code> runs in that directory. The file stays on disk — editable, diffable, and it
                                outlives the container.
                            </p>
                        </div>

                        {(plan?.blocks ?? []).length > 0 ? (
                            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2">
                                <p className="text-[11px] font-semibold text-destructive">Blocked — fix before creating</p>
                                <ul className="mt-1 list-disc space-y-0.5 pl-4 text-[11px] text-destructive">
                                    {(plan?.blocks ?? []).map((b, i) => (
                                        <li key={i}>{b}</li>
                                    ))}
                                </ul>
                                {(plan?.blocks ?? []).some((b) => b.includes("docker.sock")) ? (
                                    <label className="mt-2 flex items-start gap-2 text-[11px] text-destructive">
                                        <input
                                            type="checkbox"
                                            checked={confirmSock}
                                            onChange={(e) => {
                                                setConfirmSock(e.target.checked);
                                                setPlan(null);
                                                setStep("configure");
                                            }}
                                            className="mt-0.5"
                                        />
                                        <span>I understand this grants full host root, and intend it. (Review again to apply.)</span>
                                    </label>
                                ) : null}
                            </div>
                        ) : null}

                        {(plan?.warnings ?? []).length > 0 ? (
                            <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2">
                                <p className="text-[11px] font-semibold text-amber-700 dark:text-amber-300">Warnings</p>
                                <ul className="mt-1 list-disc space-y-0.5 pl-4 text-[11px] text-amber-700 dark:text-amber-300">
                                    {(plan?.warnings ?? []).map((wm, i) => (
                                        <li key={i}>{wm}</li>
                                    ))}
                                </ul>
                            </div>
                        ) : null}

                        {(plan?.blocks ?? []).length === 0 && (plan?.warnings ?? []).length === 0 ? (
                            <p className="flex items-center gap-1.5 text-[11px] text-emerald-600 dark:text-emerald-400">
                                <CheckCircle2 className="h-3.5 w-3.5" /> No warnings.
                            </p>
                        ) : null}

                        {output ? <pre className="max-h-40 overflow-auto rounded-md border border-destructive/30 bg-zinc-950 p-3 font-mono text-[11px] leading-5 text-red-300">{output}</pre> : null}
                    </div>
                )}

                <div className="flex items-center justify-between gap-2 border-t border-border px-5 py-3">
                    <span className="text-[11px] text-muted-foreground">{step === "review" ? "Nothing has run yet." : ""}</span>
                    <div className="flex gap-2">
                        {step === "review" ? (
                            <Button variant="outline" size="sm" onClick={() => setStep("configure")} disabled={busy}>
                                Back
                            </Button>
                        ) : (
                            <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
                                Cancel
                            </Button>
                        )}
                        {step === "configure" ? (
                            <Button size="sm" className="gap-2" onClick={review} disabled={busy || !canReview}>
                                {planning ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                                Review
                            </Button>
                        ) : (
                            <Button size="sm" className="gap-2" onClick={create} disabled={busy || (plan?.blocks ?? []).length > 0}>
                                {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                                Create
                            </Button>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}
