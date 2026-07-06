import { useState } from "react";
import {
    Globe,
    Plus,
    Search,
    Power,
    Trash2,
    ShieldCheck,
    ShieldAlert,
    ExternalLink,
    Settings,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";
import { runtime } from "../../runtime";

interface VHostItem {
    id: string;
    domain: string;
    target: string;
    type: "static" | "proxy";
    ssl: boolean;
    enabled: boolean;
}

export default function UserVHosts() {
    const [searchQuery, setSearchQuery] = useState("");
    const defaultUserHome = `/home/${runtime.username || "user"}`;
    const [vhosts, setVhosts] = useState<VHostItem[]>([
        {
            id: "1",
            domain: `${runtime.username || "my"}-app.com`,
            target: `${defaultUserHome}/public_html`,
            type: "static",
            ssl: true,
            enabled: true,
        },
        {
            id: "2",
            domain: `api.${runtime.username || "my"}-app.com`,
            target: "http://127.0.0.1:3000",
            type: "proxy",
            ssl: false,
            enabled: true,
        },
    ]);

    const handleToggleStatus = (id: string) => {
        setVhosts((prev) =>
            prev.map((vh) => (vh.id === id ? { ...vh, enabled: !vh.enabled } : vh))
        );
    };

    const handleDeleteVHost = (id: string) => {
        if (window.confirm("Are you sure you want to delete this virtual host?")) {
            setVhosts((prev) => prev.filter((vh) => vh.id !== id));
        }
    };

    const filteredVHosts = vhosts.filter((vh) =>
        vh.domain.toLowerCase().includes(searchQuery.toLowerCase())
    );

    return (
        <DashboardLayout
            title="Virtual Hosts"
            description="Manage your custom domains, Nginx reverse proxy mappings, and SSL certificates."
            actions={
                <Button className="gap-2">
                    <Plus className="h-4 w-4" />
                    Create VHost
                </Button>
            }
        >
            <div className="space-y-6">
                {/* Search Toolbar */}
                <div className="flex items-center gap-3 bg-card border border-border rounded-lg p-3 shadow-sm max-w-md">
                    <Search className="h-4 w-4 text-muted-foreground shrink-0" />
                    <input
                        type="text"
                        placeholder="Search domains..."
                        className="bg-transparent text-sm border-none outline-none w-full placeholder:text-muted-foreground"
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                    />
                </div>

                {/* VHosts Table */}
                <div className="border border-border rounded-lg bg-card overflow-hidden shadow-md">
                    <div className="min-w-full">
                        {/* Table Header */}
                        <div className="grid grid-cols-[1.5fr_1.8fr_100px_100px_100px_120px] items-center border-b border-border bg-muted/30 px-6 py-3 text-xs font-semibold text-muted-foreground">
                            <span>Domain Name</span>
                            <span>Target Path / Proxy Address</span>
                            <span>Type</span>
                            <span>SSL Status</span>
                            <span>Status</span>
                            <span className="text-right">Actions</span>
                        </div>

                        {/* Table Body */}
                        {filteredVHosts.length === 0 ? (
                            <div className="flex flex-col items-center justify-center py-16 gap-3 text-muted-foreground">
                                <Globe className="h-10 w-10 text-muted-foreground/50 shrink-0" />
                                <span className="text-sm font-medium">No virtual hosts found.</span>
                            </div>
                        ) : (
                            <div className="divide-y divide-border">
                                {filteredVHosts.map((vh) => (
                                    <div
                                        key={vh.id}
                                        className={`grid grid-cols-[1.5fr_1.8fr_100px_100px_100px_120px] items-center px-6 py-4 text-sm hover:bg-muted/10 transition-colors ${
                                            !vh.enabled ? "opacity-60" : ""
                                        }`}
                                    >
                                        {/* Domain */}
                                        <div className="flex items-center gap-2.5 min-w-0">
                                            <Globe className="h-4 w-4 text-primary shrink-0" />
                                            <span className="font-medium text-foreground truncate select-all">
                                                {vh.domain}
                                            </span>
                                            {vh.enabled && (
                                                <a
                                                    href={`http://${vh.domain}`}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="text-muted-foreground hover:text-primary transition-colors shrink-0"
                                                    title="Open site"
                                                >
                                                    <ExternalLink className="h-3.5 w-3.5" />
                                                </a>
                                            )}
                                        </div>

                                        {/* Target */}
                                        <span className="font-mono text-xs text-muted-foreground truncate select-all">
                                            {vh.target}
                                        </span>

                                        {/* Type */}
                                        <span className="capitalize text-xs text-foreground font-medium">
                                            {vh.type}
                                        </span>

                                        {/* SSL */}
                                        <div>
                                            {vh.ssl ? (
                                                <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded text-[11px] font-medium border border-emerald-500/20">
                                                    <ShieldCheck className="h-3 w-3 shrink-0" />
                                                    Active
                                                </span>
                                            ) : (
                                                <span className="inline-flex items-center gap-1 text-muted-foreground bg-muted px-2 py-0.5 rounded text-[11px] font-medium border border-border">
                                                    <ShieldAlert className="h-3 w-3 shrink-0" />
                                                    None
                                                </span>
                                            )}
                                        </div>

                                        {/* Status */}
                                        <div>
                                            {vh.enabled ? (
                                                <span className="inline-flex items-center text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded text-[11px] font-medium border border-emerald-500/20">
                                                    Enabled
                                                </span>
                                            ) : (
                                                <span className="inline-flex items-center text-muted-foreground bg-muted px-2 py-0.5 rounded text-[11px] font-medium border border-border">
                                                    Disabled
                                                </span>
                                            )}
                                        </div>

                                        {/* Actions */}
                                        <div className="flex items-center justify-end gap-1">
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                className={`h-8 w-8 p-0 ${
                                                    vh.enabled
                                                        ? "text-amber-500 hover:text-amber-600 hover:bg-amber-500/10"
                                                        : "text-emerald-500 hover:text-emerald-600 hover:bg-emerald-500/10"
                                                }`}
                                                title={vh.enabled ? "Disable host" : "Enable host"}
                                                onClick={() => handleToggleStatus(vh.id)}
                                            >
                                                <Power className="h-4 w-4" />
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                className="h-8 w-8 p-0 text-muted-foreground hover:text-foreground"
                                                title="Configure host"
                                            >
                                                <Settings className="h-4 w-4" />
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                className="h-8 w-8 p-0 text-destructive hover:text-destructive/90 hover:bg-destructive/10"
                                                title="Delete host"
                                                onClick={() => handleDeleteVHost(vh.id)}
                                            >
                                                <Trash2 className="h-4 w-4" />
                                            </Button>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </DashboardLayout>
    );
}
