import { Link } from "react-router-dom";
import { CornerUpRight, Globe, Server, Trash2 } from "lucide-react";

import type { Section } from "./shared";

type Item = {
    key: Section;
    href: string;
    label: string;
    icon: typeof Globe;
    count?: number;
    tone?: "warn" | "err";
};

export default function VHostsSidebar({
    section,
    tenantCount,
    systemCount,
    redirectCount,
    orphanCount,
}: {
    section: Section;
    tenantCount: number;
    systemCount: number;
    redirectCount: number;
    orphanCount: number;
}) {
    const items: Item[] = [
        { key: "tenant", href: "/vhosts", label: "Tenant", icon: Globe, count: tenantCount },
        { key: "system", href: "/vhosts/system", label: "System", icon: Server, count: systemCount },
        { key: "redirects", href: "/vhosts/redirects", label: "Redirects", icon: CornerUpRight, count: redirectCount },
        { key: "orphans", href: "/vhosts/orphans", label: "Orphans", icon: Trash2, count: orphanCount, tone: orphanCount > 0 ? "err" : undefined },
    ];
    return (
        <aside className="flex h-full flex-col gap-1 overflow-y-auto border-r border-border bg-card/60 p-2">
            {items.map((it) => (
                <NavItem key={it.key} active={section === it.key} item={it} />
            ))}
        </aside>
    );
}

function NavItem({ active, item }: { active: boolean; item: Item }) {
    const Icon = item.icon;
    const badgeCls =
        item.tone === "err"
            ? "bg-destructive/15 text-destructive"
            : item.tone === "warn"
              ? "bg-amber-500/15 text-amber-600 dark:text-amber-400"
              : active
                ? "bg-primary/15 text-primary"
                : "bg-muted text-muted-foreground";
    return (
        <Link
            to={item.href}
            className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-xs font-semibold ${
                active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
        >
            <Icon className="h-4 w-4" />
            <span className="flex-1">{item.label}</span>
            {item.count !== undefined ? (
                <span className={`min-w-5 rounded-full px-1.5 py-0.5 text-center text-[10px] font-bold tabular-nums ${badgeCls}`}>
                    {item.count}
                </span>
            ) : null}
        </Link>
    );
}
