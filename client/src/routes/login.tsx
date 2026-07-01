import { type FormEvent, useState, useEffect } from "react";
import {
    ArrowRight,
    Loader2,
    LockKeyhole,
    Server,
    ShieldCheck,
    User,
} from "lucide-react";

import ColorModeSwitch from "_components/color-mode-switch";
import { Button } from "_components/ui/button";
import DefaultLayout from "_layouts";
import Api from "_utils/api";
import { runtime } from "runtime";

type LoginStatus = "idle" | "loading" | "success" | "error";

export default function LoginRoute() {
    const [status, setStatus] = useState<LoginStatus>("idle");
    const [message, setMessage] = useState("");
    const isLoading = status === "loading";

    useEffect(() => {
        const isLoggedIn = localStorage.getItem("is_logged_in") === "true";
        if (isLoggedIn) {
            window.location.href = "/";
        }
    }, []);

    async function handleSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();

        const formData = new FormData(event.currentTarget);
        setStatus("loading");
        setMessage("");

        try {
            const response = await fetch(Api.current.login, {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify({
                    username: formData.get("username"),
                    password: formData.get("password"),
                }),
            });

            if (!response.ok) {
                const errorMsg = await response.text();
                throw new Error(errorMsg.trim() || "Login failed");
            }

            setStatus("success");
            setMessage("Login successful. Redirecting...");
            localStorage.setItem("is_logged_in", "true");
            setTimeout(() => {
                window.location.href = "/";
            }, 800);
        } catch (error: any) {
            setStatus("error");
            setMessage(error.message || "Unable to login.");
        }
    }

    return (
        <DefaultLayout>
            <main className="relative flex min-h-screen items-center justify-center bg-background px-6 py-8 text-foreground">
                <div className="absolute right-6 top-6">
                    <ColorModeSwitch />
                </div>

                <section className="grid w-full max-w-4xl overflow-hidden rounded-md border border-border bg-card shadow-sm md:grid-cols-[0.9fr_1.1fr]">
                    <aside className="flex min-h-64 flex-col justify-between border-b border-border bg-muted/40 p-6 md:border-b-0 md:border-r">
                        <div className="space-y-6">
                            <div className="flex items-center gap-3">
                                <div className="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-background">
                                    <Server
                                        className="h-5 w-5"
                                        aria-hidden="true"
                                    />
                                </div>
                                <div>
                                    <p className="text-sm font-semibold">
                                        MThan VPS
                                    </p>
                                    <p className="text-xs text-muted-foreground">
                                        {runtime.mode} access
                                    </p>
                                </div>
                            </div>

                            <div className="space-y-3">
                                <div className="inline-flex items-center gap-2 rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium">
                                    <ShieldCheck
                                        className="h-3.5 w-3.5 text-muted-foreground"
                                        aria-hidden="true"
                                    />
                                    {runtime.isRoot
                                        ? "Root session"
                                        : "User session"}
                                </div>
                                <div className="space-y-2">
                                    <h1 className="text-3xl font-semibold tracking-tight">
                                        Sign in
                                    </h1>
                                    <p className="text-sm leading-6 text-muted-foreground">
                                        Enter your credentials to continue.
                                    </p>
                                </div>
                            </div>
                        </div>

                        <p className="mt-8 text-xs text-muted-foreground">
                            {runtime.username
                                ? `Runtime user: ${runtime.username}`
                                : "Runtime user unavailable"}
                        </p>
                    </aside>

                    <div className="p-6 sm:p-8">
                        <div className="mb-6 space-y-1">
                            <h2 className="text-xl font-semibold tracking-tight">
                                Login
                            </h2>
                            <p className="text-sm text-muted-foreground">
                                {runtime.isRoot
                                    ? "Use root credentials."
                                    : "Use your account credentials."}
                            </p>
                        </div>

                        <form className="space-y-5" onSubmit={handleSubmit}>
                            <label className="block space-y-2">
                                <span className="text-sm font-medium">
                                    Username
                                </span>
                                <span className="relative block">
                                    <User
                                        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
                                        aria-hidden="true"
                                    />
                                    <input
                                        className="h-11 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-1 focus:ring-ring"
                                        name="username"
                                        autoComplete="username"
                                        placeholder="username"
                                        required
                                    />
                                </span>
                            </label>

                            <label className="block space-y-2">
                                <span className="text-sm font-medium">
                                    Password
                                </span>
                                <span className="relative block">
                                    <LockKeyhole
                                        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
                                        aria-hidden="true"
                                    />
                                    <input
                                        className="h-11 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-1 focus:ring-ring"
                                        name="password"
                                        type="password"
                                        autoComplete="current-password"
                                        placeholder="password"
                                        required
                                    />
                                </span>
                            </label>

                            <Button
                                className="h-11 w-full gap-2"
                                disabled={isLoading}
                                type="submit"
                            >
                                {isLoading ? (
                                    <Loader2
                                        className="h-4 w-4 animate-spin"
                                        aria-hidden="true"
                                    />
                                ) : (
                                    <ArrowRight
                                        className="h-4 w-4"
                                        aria-hidden="true"
                                    />
                                )}
                                {isLoading ? "Signing in..." : "Sign in"}
                            </Button>

                            {message !== "" ? (
                                <p
                                    className={statusMessageClassName(status)}
                                    aria-live="polite"
                                >
                                    {message}
                                </p>
                            ) : null}
                        </form>
                    </div>
                </section>
            </main>
        </DefaultLayout>
    );
}

function statusMessageClassName(status: LoginStatus) {
    if (status === "success") {
        return "rounded-md border border-border bg-muted/40 px-3 py-2 text-sm text-foreground";
    }

    if (status === "error") {
        return "rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive";
    }

    return "text-sm text-muted-foreground";
}
