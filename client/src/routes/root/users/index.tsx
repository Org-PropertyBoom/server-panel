import { type FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
    Plus,
    RefreshCw,
    Shield,
    Loader2,
    X,
    Key,
    CheckCircle,
    Eye,
    EyeOff,
    User,
    Terminal,
    Home,
    Trash2,
    LayoutDashboard,
    Folder,
    Boxes,
    ChevronRight,
    CheckCircle2,
    XCircle,
    GitBranch,
    Upload,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import { useApp } from "_contexts/app";
import { useTerminal } from "_contexts/terminal";

interface LinuxUser {
    home: string;
    name: string;
    shell: string;
    uid: number;
    username: string;
}

interface SystemAppStatus {
    installed: boolean;
    name: string;
    version?: string;
    versions?: string[];
}

const systemAppNames: Record<string, string> = {
    nginx: "Nginx",
    mariadb: "MariaDB",
    php: "PHP",
    redis: "Redis",
    docker: "Docker",
    podman: "Podman",
    node: "Node.js",
};

export default function UsersRoute() {
    const { settings } = useApp();
    const { addTab: openUserTerminal } = useTerminal();
    const params = useParams<{ username?: string; section?: string }>();
    const autoUsername = (settings.users_auto_username ?? "false") === "true";
    const routeUsername = params.username ?? "";
    const activeSection = userSection(params.section);
    const [users, setUsers] = useState<LinuxUser[]>([]);
    const [selectedUser, setSelectedUser] = useState<LinuxUser | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [isRefreshing, setIsRefreshing] = useState(false);
    const [isDeleting, setIsDeleting] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [userApps, setUserApps] = useState<string[]>([]);
    const [appsError, setAppsError] = useState<string | null>(null);
    const [appsLoading, setAppsLoading] = useState(false);
    const [systemApps, setSystemApps] = useState<SystemAppStatus[]>([]);
    const [systemAppsError, setSystemAppsError] = useState<string | null>(null);
    const [systemAppsLoading, setSystemAppsLoading] = useState(false);
    const [addAppOpen, setAddAppOpen] = useState(false);
    const [addAppMode, setAddAppMode] = useState<"upload" | "git">("upload");
    const [addAppName, setAddAppName] = useState("");
    const [repository, setRepository] = useState("");
    const [appArchive, setAppArchive] = useState<File | null>(null);
    const [addAppSaving, setAddAppSaving] = useState(false);
    const [addAppError, setAddAppError] = useState("");

    // Modal state
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [password, setPassword] = useState("");
    const [confirmPassword, setConfirmPassword] = useState("");
    const [showPassword, setShowPassword] = useState(false);
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [modalError, setModalError] = useState<string | null>(null);
    const [username, setUsername] = useState("");
    const [createdUser, setCreatedUser] = useState<{ username: string; password: string } | null>(null);

    const generateRandomPassword = () => {
        const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=";
        const length = 16;
        let result = "";
        for (let i = 0; i < length; i++) {
            const randomIndex = Math.floor(Math.random() * charset.length);
            result += charset[randomIndex];
        }
        setPassword(result);
        setConfirmPassword(result);
        setShowPassword(true);
    };

    useEffect(() => {
        if (!isModalOpen) return;

        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === "Escape") {
                handleCloseModal();
            }
        };

        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    }, [isModalOpen]);

    const fetchUsers = async (showRefresh = false) => {
        if (showRefresh) setIsRefreshing(true);
        else setIsLoading(true);
        setError(null);
        try {
            const response = await fetch("/post/user/list");
            if (!response.ok) {
                throw new Error("Failed to fetch users");
            }
            const data = await response.json();
            if (data.status === "ok") {
                const list = data.users || [];
                setUsers(list);
                
                // Set default selected user
                if (list.length > 0) {
                    setSelectedUser((prev) => {
                        const routed = list.find((u: LinuxUser) => u.username === routeUsername);
                        if (routed) return routed;
                        const exists = list.find((u: LinuxUser) => u.username === prev?.username);
                        return exists || list[0];
                    });
                } else {
                    setSelectedUser(null);
                }
            } else {
                throw new Error(data.message || "Failed to load users");
            }
        } catch (err: any) {
            setError(err.message || "An unexpected error occurred");
        } finally {
            setIsLoading(false);
            setIsRefreshing(false);
        }
    };

    useEffect(() => {
        fetchUsers();
    }, []);

    useEffect(() => {
        if (!routeUsername) return;
        const routed = users.find((user) => user.username === routeUsername);
        if (routed) setSelectedUser(routed);
    }, [routeUsername, users]);

    useEffect(() => {
        if (activeSection !== "apps" || !selectedUser) return;

        const controller = new AbortController();
        setAppsLoading(true);
        setAppsError(null);
        fetch(`/post/user/apps?username=${encodeURIComponent(selectedUser.username)}`, { signal: controller.signal })
            .then(async (response) => {
                if (!response.ok) throw new Error((await response.text()) || "Failed to load user apps");
                return response.json();
            })
            .then((data) => setUserApps(data.apps || []))
            .catch((requestError) => {
                if (requestError.name !== "AbortError") setAppsError(requestError.message || "Failed to load user apps");
            })
            .finally(() => {
                if (!controller.signal.aborted) setAppsLoading(false);
            });

        return () => controller.abort();
    }, [activeSection, selectedUser]);

    useEffect(() => {
        if (activeSection !== "overview") return;

        const controller = new AbortController();
        setSystemAppsLoading(true);
        setSystemAppsError(null);
        fetch("/post/apps", { cache: "no-store", signal: controller.signal })
            .then(async (response) => {
                if (!response.ok) throw new Error((await response.text()) || "Failed to load system apps");
                return response.json();
            })
            .then((data) => setSystemApps(data.apps || []))
            .catch((requestError) => {
                if (requestError.name !== "AbortError") setSystemAppsError(requestError.message || "Failed to load system apps");
            })
            .finally(() => {
                if (!controller.signal.aborted) setSystemAppsLoading(false);
            });

        return () => controller.abort();
    }, [activeSection]);

    const submitApp = async (event: FormEvent) => {
        event.preventDefault();
        if (!selectedUser) return;
        setAddAppSaving(true); setAddAppError("");
        try {
            let response: Response;
            const endpoint = `/post/user/apps?username=${encodeURIComponent(selectedUser.username)}`;
            if (addAppMode === "upload") {
                if (!appArchive) throw new Error("ZIP file is required");
                const body = new FormData(); body.append("name", addAppName.trim()); body.append("file", appArchive);
                response = await fetch(endpoint, { method: "POST", body });
            } else {
                response = await fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: addAppName.trim(), repository: repository.trim() }) });
            }
            if (!response.ok) throw new Error((await response.text()) || "Failed to add app");
            setAddAppOpen(false); setAddAppName(""); setRepository(""); setAppArchive(null);
            setUserApps((current) => [...current, addAppName.trim()].sort());
        } catch (requestError) { setAddAppError(requestError instanceof Error ? requestError.message : "Failed to add app"); }
        finally { setAddAppSaving(false); }
    };

    const handleCreateUser = async (e: FormEvent) => {
        e.preventDefault();
        setModalError(null);

        if (!autoUsername && !username.trim()) {
            setModalError("Username is required");
            return;
        }

        if (!password.trim()) {
            setModalError("Password is required");
            return;
        }

        if (password !== confirmPassword) {
            setModalError("Passwords do not match");
            return;
        }

        setIsSubmitting(true);
        try {
            const response = await fetch("/post/user/add", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify({ password, username: autoUsername ? "" : username.trim() }),
            });

            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || "Failed to create user");
            }

            const data = await response.json();
            if (data.status === "ok") {
                setCreatedUser({
                    username: data.username,
                    password: password,
                });
                setPassword("");
                setConfirmPassword("");
                setUsername("");
                fetchUsers();
            } else {
                throw new Error(data.message || "Failed to create user");
            }
        } catch (err: any) {
            setModalError(err.message || "An error occurred during user creation");
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleCloseModal = () => {
        setIsModalOpen(false);
        setCreatedUser(null);
        setPassword("");
        setConfirmPassword("");
        setUsername("");
        setShowPassword(false);
        setModalError(null);
    };

    const handleDeleteUser = async (username: string) => {
        if (!window.confirm(`Are you sure you want to delete user "${username}"? All files in their home folder will be permanently deleted.`)) {
            return;
        }

        setIsDeleting(true);
        try {
            const response = await fetch("/post/user/delete", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify({ username }),
            });

            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || "Failed to delete user");
            }

            const data = await response.json();
            if (data.status === "ok") {
                fetchUsers();
            } else {
                throw new Error(data.message || "Failed to delete user");
            }
        } catch (err: any) {
            alert(err.message || "Could not delete user.");
        } finally {
            setIsDeleting(false);
        }
    };

    return (
        <DashboardLayout
            title="Linux users"
            description="Manage local system accounts, home folders, shells, and system access."
            fullWidth={true}
        >
            <div className="grid grid-cols-1 md:grid-cols-[280px_1fr] h-[calc(100vh-56px)] overflow-hidden">
                {/* Left Sidebar - Users List */}
                <aside className="border-r border-border bg-card/60 flex flex-col h-full overflow-hidden select-none">
                    <div className="flex h-10 items-center justify-between px-3 border-b border-border bg-muted/20">
                        <span className="text-xs font-semibold text-muted-foreground">
                            Users List
                        </span>
                        <div className="flex items-center gap-1">
                            <button
                                onClick={() => fetchUsers(true)}
                                className="p-1 rounded-none text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                                title="Refresh Users"
                                disabled={isLoading || isRefreshing}
                            >
                                <RefreshCw className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`} />
                            </button>
                            <button
                                onClick={() => setIsModalOpen(true)}
                                className="p-1 rounded-none text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                                title="Create User"
                            >
                                <Plus className="h-3.5 w-3.5" />
                            </button>
                        </div>
                    </div>

                    <div className="flex-1 overflow-y-auto py-2 px-2 space-y-1">
                        {isLoading && users.length === 0 ? (
                            <div className="flex items-center justify-center py-12">
                                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                            </div>
                        ) : error ? (
                            <div className="text-xs text-destructive p-3 text-center">
                                <AlertCircle className="h-5 w-5 mx-auto mb-2 text-destructive" />
                                <span>{error}</span>
                            </div>
                        ) : users.length === 0 ? (
                            <div className="text-xs text-muted-foreground p-4 text-center">
                                No home users found.
                            </div>
                        ) : (
                            users.map((u) => {
                                const isSelected = selectedUser?.username === u.username;
                                return (
                                    <div key={u.username}>
                                    <Link
                                        to={`/users/${encodeURIComponent(u.username)}/overview`}
                                        className={`flex items-center gap-2 py-1.5 px-2.5 rounded-none hover:bg-muted/60 transition-colors text-xs ${
                                            isSelected
                                                ? "bg-primary/10 text-primary font-semibold"
                                                : "text-foreground/90"
                                        }`}
                                    >
                                        <User className={`h-3.5 w-3.5 shrink-0 ${isSelected ? "text-primary" : "text-muted-foreground"}`} />
                                        <span className="truncate flex-1 min-w-0">{u.username}</span>
                                    </Link>
                                    {isSelected ? (
                                        <nav className="ml-5 border-l border-border py-1 pl-2">
                                            <UserSubItem username={u.username} section="overview" active={activeSection === "overview"} icon={LayoutDashboard} label="Overview" />
                                            <UserSubItem username={u.username} section="files" active={activeSection === "files"} icon={Folder} label="Files" />
                                            <UserSubItem username={u.username} section="apps" active={activeSection === "apps"} icon={Boxes} label="Apps" />
                                            <button
                                                type="button"
                                                onClick={() => openUserTerminal(u.username)}
                                                className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-[11px] text-muted-foreground hover:text-foreground"
                                            >
                                                <Terminal className="h-3.5 w-3.5" />
                                                Terminal
                                            </button>
                                        </nav>
                                    ) : null}
                                    </div>
                                );
                            })
                        )}
                    </div>
                </aside>

                {/* Right Panel - User Details Dashboard */}
                <main className="bg-background flex flex-col h-full overflow-hidden">
                    {selectedUser ? (
                        <div className="flex-1 overflow-y-auto p-6 space-y-6">
                            {/* Profile Header */}
                            <div className="flex items-center gap-3 border-b border-border pb-5">
                                <div className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
                                    <h2 className="mr-1 shrink-0 text-2xl font-bold tracking-tight text-foreground">
                                        {selectedUser.username}
                                    </h2>
                                    <UserInfoBox icon={Shield} label="UID" value={String(selectedUser.uid)} />
                                    <UserInfoBox icon={Home} label="Home" value={selectedUser.home} />
                                    <UserInfoBox icon={Terminal} label="Shell" value={selectedUser.shell || "/bin/bash"} />
                                </div>

                                {selectedUser.uid !== 0 && (
                                    <Button
                                        size="sm"
                                        variant="outline"
                                        className="text-destructive border-destructive/20 hover:bg-destructive/10 gap-1.5 shrink-0 rounded-none"
                                        onClick={() => handleDeleteUser(selectedUser.username)}
                                        disabled={isDeleting}
                                    >
                                        {isDeleting ? (
                                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                        ) : (
                                            <Trash2 className="h-3.5 w-3.5" />
                                        )}
                                        Delete User
                                    </Button>
                                )}
                            </div>

                            {activeSection === "overview" ? (
                            <div className="border border-border bg-card">
                                <div className="border-b border-border px-4 py-3">
                                    <h3 className="text-sm font-semibold">System Apps</h3>
                                    <p className="mt-1 text-xs text-muted-foreground">Installation status and detected versions from the system Apps route.</p>
                                </div>
                                {systemAppsLoading ? (
                                    <div className="flex items-center justify-center p-10"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
                                ) : systemAppsError ? (
                                    <p className="p-4 text-xs text-destructive">{systemAppsError}</p>
                                ) : (
                                    <div className="divide-y divide-border">
                                        {systemApps.map((app) => (
                                            <Link key={app.name} to={`/apps/${encodeURIComponent(app.name)}`} className="flex items-center gap-3 px-4 py-3 text-sm transition-colors hover:bg-muted/50">
                                                <Boxes className="h-4 w-4 shrink-0 text-primary" />
                                                <span className="min-w-0 flex-1 truncate font-medium">{systemAppNames[app.name] || app.name}</span>
                                                <span className="font-mono text-xs text-muted-foreground">{app.version || app.versions?.join(", ") || "—"}</span>
                                                <span className={`inline-flex w-28 items-center gap-1.5 text-xs ${app.installed ? "text-emerald-600 dark:text-emerald-400" : "text-muted-foreground"}`}>
                                                    {app.installed ? <CheckCircle2 className="h-3.5 w-3.5" /> : <XCircle className="h-3.5 w-3.5" />}
                                                    {app.installed ? "Installed" : "Not installed"}
                                                </span>
                                            </Link>
                                        ))}
                                    </div>
                                )}
                            </div>
                            ) : activeSection === "files" ? (
                                <div className="rounded-md border border-border bg-card p-5">
                                    <h3 className="text-sm font-semibold">User Files</h3>
                                    <p className="mt-2 font-mono text-xs text-muted-foreground">{selectedUser.home}</p>
                                    <Button variant="outline" size="sm" asChild className="mt-4">
                                        <Link to={`/files?path=${encodeURIComponent(selectedUser.home)}`}>Open File Explorer</Link>
                                    </Button>
                                </div>
                            ) : (
                                <div className="border border-border bg-card">
                                    <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
                                        <div><h3 className="text-sm font-semibold">User Apps</h3><p className="mt-1 font-mono text-[11px] text-muted-foreground">{selectedUser.home}/htdocs</p></div>
                                        <Button size="sm" className="gap-2" onClick={() => setAddAppOpen(true)}><Plus className="h-4 w-4" />Add App</Button>
                                    </div>
                                    {appsLoading ? (
                                        <div className="flex items-center justify-center p-10"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
                                    ) : appsError ? (
                                        <p className="p-4 text-xs text-destructive">{appsError}</p>
                                    ) : userApps.length === 0 ? (
                                        <p className="p-4 text-xs text-muted-foreground">No apps found.</p>
                                    ) : (
                                        <div className="divide-y divide-border">
                                            {userApps.map((app) => (
                                                <details key={app} className="group">
                                                    <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
                                                        <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
                                                        <Boxes className="h-4 w-4 shrink-0 text-primary" />
                                                        <span className="min-w-0 flex-1 truncate font-medium" title={app}>{app}</span>
                                                    </summary>
                                                    <div className="border-t border-border bg-muted/20 px-10 py-5">
                                                        <p className="text-xs font-medium text-foreground">Coming soon</p>
                                                        <p className="mt-1 text-xs text-muted-foreground">App details and configuration will be available here.</p>
                                                    </div>
                                                </details>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            )}
                        </div>
                    ) : (
                        /* Empty state */
                        <div className="flex-grow flex flex-col items-center justify-center p-8 text-muted-foreground">
                            <User className="h-12 w-12 text-muted-foreground/30 mb-2 shrink-0" />
                            <p className="text-sm font-medium">Select a user from the list to view profile details.</p>
                        </div>
                    )}
                </main>
            </div>

            {/* Modal */}
            {addAppOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
                    <form onSubmit={submitApp} className="w-full max-w-md border border-border bg-card shadow-lg">
                        <div className="flex items-center justify-between border-b border-border px-5 py-4"><h3 className="font-semibold">Add App</h3><button type="button" onClick={() => setAddAppOpen(false)}><X className="h-4 w-4" /></button></div>
                        <div className="space-y-4 p-5">
                            <div className="grid grid-cols-2 gap-2">
                                <Button type="button" variant={addAppMode === "upload" ? "default" : "outline"} onClick={() => setAddAppMode("upload")} className="gap-2"><Upload className="h-4 w-4" />Upload ZIP</Button>
                                <Button type="button" variant={addAppMode === "git" ? "default" : "outline"} onClick={() => setAddAppMode("git")} className="gap-2"><GitBranch className="h-4 w-4" />Git Clone</Button>
                            </div>
                            <label className="block space-y-1.5 text-xs font-medium"><span>App Name</span><input required value={addAppName} onChange={(event) => setAddAppName(event.target.value)} placeholder="my-app" className="h-9 w-full border border-input bg-background px-3 text-sm outline-none focus:border-primary" /></label>
                            {addAppMode === "upload" ? (
                                <label className="block space-y-1.5 text-xs font-medium"><span>ZIP Archive</span><input required type="file" accept=".zip,application/zip" onChange={(event) => setAppArchive(event.target.files?.[0] || null)} className="block w-full text-xs text-muted-foreground file:mr-3 file:border file:border-input file:bg-background file:px-3 file:py-2 file:text-xs" /></label>
                            ) : (
                                <label className="block space-y-1.5 text-xs font-medium"><span>Repository URL</span><input required value={repository} onChange={(event) => setRepository(event.target.value)} placeholder="https://github.com/example/app.git" className="h-9 w-full border border-input bg-background px-3 text-sm outline-none focus:border-primary" /></label>
                            )}
                            {addAppError ? <p className="text-xs text-destructive">{addAppError}</p> : null}
                        </div>
                        <div className="flex justify-end gap-2 border-t border-border px-5 py-4"><Button type="button" variant="outline" onClick={() => setAddAppOpen(false)}>Cancel</Button><Button type="submit" disabled={addAppSaving}>{addAppSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : "Add App"}</Button></div>
                    </form>
                </div>
            )}

            {isModalOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-background/80 backdrop-blur-sm animate-in fade-in duration-200">
                    <div className="relative w-full max-w-md overflow-hidden rounded-none border border-border bg-card shadow-lg animate-in zoom-in-95 duration-200">
                        {/* Header */}
                        <div className="flex items-center justify-between border-b border-border px-6 py-4">
                            <h3 className="text-lg font-semibold tracking-tight">Create Linux User</h3>
                            <button
                                onClick={handleCloseModal}
                                className="rounded-none opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2"
                            >
                                <X className="h-4 w-4" />
                                <span className="sr-only">Close</span>
                            </button>
                        </div>

                        {/* Content */}
                        <div className="p-6">
                            {createdUser ? (
                                <div className="space-y-4">
                                    <div className="flex items-center gap-3 rounded-none bg-emerald-500/10 border border-emerald-500/20 p-3 text-sm text-emerald-600 dark:text-emerald-400">
                                        <CheckCircle className="h-5 w-5 shrink-0" />
                                        <span>User created successfully!</span>
                                    </div>

                                    <div className="rounded-none bg-muted p-4 space-y-3">
                                        <div>
                                            <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">Username</label>
                                            <span className="text-base font-mono font-bold select-all block mt-1">{createdUser.username}</span>
                                        </div>
                                        <div>
                                            <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">Password</label>
                                            <span className="text-base font-mono font-bold select-all block mt-1">{createdUser.password}</span>
                                        </div>
                                    </div>

                                    <div className="text-xs text-muted-foreground bg-amber-500/10 border border-amber-500/20 rounded-none p-2.5">
                                        Please record this username and password. You will need them to log in to this account.
                                    </div>

                                    <Button onClick={handleCloseModal} className="w-full mt-4 rounded-none">
                                        Done
                                    </Button>
                                </div>
                            ) : (
                                <form onSubmit={handleCreateUser} className="space-y-4">
                                    {autoUsername ? (
                                    <div className="rounded-none border border-blue-500/20 bg-blue-500/5 p-3 text-xs text-blue-600 dark:text-blue-400 leading-normal">
                                        <strong>Username Notice:</strong> The username will be generated automatically using the prefix <code className="font-mono bg-blue-500/10 px-1 rounded-none">user-</code> followed by 8 random characters (e.g. <code className="font-mono bg-blue-500/10 px-1 rounded-none">user-ax9h2b7m</code>).
                                    </div>
                                    ) : (
                                        <div className="space-y-1">
                                            <label htmlFor="username" className="text-sm font-medium">Username</label>
                                            <input
                                                id="username"
                                                value={username}
                                                onChange={(event) => setUsername(event.target.value)}
                                                placeholder="Enter Linux username"
                                                className="flex h-9 w-full rounded-none border border-input bg-transparent px-3 py-1 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                                                required
                                            />
                                        </div>
                                    )}

                                    {modalError && (
                                        <div className="rounded-none border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive">
                                            {modalError}
                                        </div>
                                    )}

                                    <div className="space-y-1">
                                        <div className="flex items-center justify-between">
                                            <label htmlFor="password" className="text-sm font-medium">Password</label>
                                            <button
                                                type="button"
                                                onClick={generateRandomPassword}
                                                className="text-xs text-primary hover:underline"
                                            >
                                                Generate password
                                            </button>
                                        </div>
                                        <div className="relative">
                                            <Key className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
                                            <input
                                                id="password"
                                                type={showPassword ? "text" : "password"}
                                                className="flex h-9 w-full rounded-none border border-input bg-transparent pl-9 pr-10 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                                                placeholder="Enter account password"
                                                value={password}
                                                onChange={(e) => setPassword(e.target.value)}
                                                required
                                            />
                                            <button
                                                type="button"
                                                onClick={() => setShowPassword((prev) => !prev)}
                                                className="absolute right-3 top-2.5 text-muted-foreground hover:text-foreground"
                                            >
                                                {showPassword ? (
                                                    <EyeOff className="h-4 w-4" />
                                                ) : (
                                                    <Eye className="h-4 w-4" />
                                                )}
                                            </button>
                                        </div>
                                    </div>

                                    <div className="space-y-1">
                                        <label htmlFor="confirmPassword" className="text-sm font-medium">Confirm Password</label>
                                        <div className="relative">
                                            <Key className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
                                            <input
                                                id="confirmPassword"
                                                type={showPassword ? "text" : "password"}
                                                className="flex h-9 w-full rounded-none border border-input bg-transparent pl-9 pr-10 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                                                placeholder="Confirm account password"
                                                value={confirmPassword}
                                                onChange={(e) => setConfirmPassword(e.target.value)}
                                                required
                                            />
                                        </div>
                                    </div>

                                    <div className="flex justify-end gap-3 pt-4 border-t border-border mt-6">
                                        <Button
                                            type="button"
                                            variant="outline"
                                            onClick={handleCloseModal}
                                            disabled={isSubmitting}
                                            className="rounded-none"
                                        >
                                            Cancel
                                        </Button>
                                        <Button
                                            type="submit"
                                            disabled={isSubmitting}
                                            className="gap-2 rounded-none"
                                        >
                                            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
                                            Create User
                                        </Button>
                                    </div>
                                </form>
                            )}
                        </div>
                    </div>
                </div>
            )}
        </DashboardLayout>
    );
}

// Simple AlertCircle fallback since we need it in root.tsx & user.tsx
function AlertCircle({ className }: { className?: string }) {
    return (
        <span className={className}>⚠️</span>
    );
}

type UserSection = "overview" | "files" | "apps";

function userSection(section?: string): UserSection {
    return section === "files" || section === "apps" ? section : "overview";
}

function UserSubItem({ username, section, active, icon: Icon, label }: {
    username: string;
    section: UserSection;
    active: boolean;
    icon: typeof User;
    label: string;
}) {
    return (
        <Link
            to={`/users/${encodeURIComponent(username)}/${section}`}
            className={`flex items-center gap-2 px-2 py-1.5 text-[11px] ${
                active ? "font-semibold text-primary" : "text-muted-foreground hover:text-foreground"
            }`}
        >
            <Icon className="h-3.5 w-3.5" />
            {label}
        </Link>
    );
}

function UserInfoBox({ icon: Icon, label, value }: {
    icon: typeof User;
    label: string;
    value: string;
}) {
    return (
        <div className="flex h-6 min-w-0 max-w-full items-center gap-1.5 border border-border bg-card/40 px-2">
            <Icon className="h-3 w-3 shrink-0 text-primary" />
            <span className="text-[9px] font-semibold uppercase tracking-wide text-muted-foreground">{label}</span>
            <span className="max-w-48 truncate font-mono text-[10px] text-foreground" title={value}>{value}</span>
        </div>
    );
}
