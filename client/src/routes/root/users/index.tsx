import { Plus, RefreshCw, Shield, UserCog } from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";

const users = [
    {
        name: "root",
        role: "Superuser",
        shell: "/bin/bash",
        status: "Active",
        uid: 0,
    },
];

export default function UsersRoute() {
    return (
        <DashboardLayout
            title="Linux users"
            description="Manage local system accounts, shells, groups, and access."
            actions={
                <>
                    <Button variant="outline" className="gap-2">
                        <RefreshCw className="h-4 w-4" />
                        Refresh
                    </Button>
                    <Button className="gap-2">
                        <Plus className="h-4 w-4" />
                        Add user
                    </Button>
                </>
            }
        >
            <section className="overflow-hidden rounded-md border border-border bg-card">
                <div className="grid grid-cols-[1.3fr_0.7fr_0.8fr_0.7fr_80px] border-b border-border bg-muted/40 px-4 py-2 text-xs font-medium text-muted-foreground">
                    <span>User</span>
                    <span>UID</span>
                    <span>Shell</span>
                    <span>Status</span>
                    <span className="text-right">Actions</span>
                </div>

                {users.map((user) => (
                    <div
                        key={user.name}
                        className="grid grid-cols-[1.3fr_0.7fr_0.8fr_0.7fr_80px] items-center border-b border-border px-4 py-3 text-sm last:border-b-0"
                    >
                        <div className="flex min-w-0 items-center gap-3">
                            <div className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-background">
                                <Shield className="h-4 w-4 text-muted-foreground" />
                            </div>
                            <div className="min-w-0">
                                <p className="truncate font-medium">{user.name}</p>
                                <p className="truncate text-xs text-muted-foreground">
                                    {user.role}
                                </p>
                            </div>
                        </div>
                        <span className="text-muted-foreground">{user.uid}</span>
                        <span className="truncate text-muted-foreground">
                            {user.shell}
                        </span>
                        <span>
                            <span className="inline-flex rounded-md border border-emerald-500/30 bg-emerald-500/10 px-2 py-1 text-xs font-medium text-emerald-700 dark:text-emerald-300">
                                {user.status}
                            </span>
                        </span>
                        <div className="flex justify-end">
                            <button
                                className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                                title="Manage user"
                                type="button"
                            >
                                <UserCog className="h-4 w-4" />
                            </button>
                        </div>
                    </div>
                ))}
            </section>
        </DashboardLayout>
    );
}
