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
import { Button } from "_layouts/_components/ui/button";
import DefaultLayout from "_layouts";
import Api from "_utils/api";
import { runtime } from "runtime";

type LoginStatus = "idle" | "loading" | "success" | "error";

export default function LoginRoute() {
    const [status, setStatus] = useState<LoginStatus>("idle");
    const [message, setMessage] = useState("");
    const isLoading = status === "loading";

    useEffect(() => {
        document.title = "Login | Ppt Server Panel";
        const isLoggedIn = localStorage.getItem("is_logged_in") === "true";
        if (isLoggedIn) {
            window.location.href = "/";
        }
    }, []);

    const handleInputChange = () => {
        if (status === "error") {
            setStatus("idle");
            setMessage("");
        }
    };

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
                if (response.status === 401) {
                    throw new Error("Invalid username or password");
                }
                if ([502, 503, 504].includes(response.status)) {
                    throw new Error("The server is temporarily unavailable. Please try again shortly");
                }
                const contentType = response.headers.get("content-type") ?? "";
                const errorMsg = contentType.includes("text/plain") ? (await response.text()).trim() : "";
                throw new Error(errorMsg && errorMsg.length <= 200 ? errorMsg : `Login failed (${response.status})`);
            }

            setStatus("success");
            localStorage.setItem("is_logged_in", "true");
            setTimeout(() => {
                window.location.href = "/";
            }, 800);
        } catch (error: any) {
            setStatus("error");
            setMessage(
                error instanceof TypeError
                    ? "Unable to reach the server. Please try again shortly"
                    : error.message || "Unable to login",
            );
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
                                        Ppt Server Panel
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
                                        onChange={handleInputChange}
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
                                        onChange={handleInputChange}
                                    />
                                </span>
                            </label>

                            <Button
                                className={`h-11 w-full gap-2 transition-all duration-300 ${
                                    status === "success"
                                        ? "bg-emerald-600 hover:bg-emerald-600 text-white disabled:opacity-100"
                                        : status === "error"
                                        ? "bg-destructive hover:bg-destructive/90 text-destructive-foreground"
                                        : ""
                                }`}
                                disabled={isLoading || status === "success"}
                                type="submit"
                            >
                                {status === "loading" && (
                                    <Loader2 className="h-4 w-4 animate-spin shrink-0" aria-hidden="true" />
                                )}
                                {status === "success" && (
                                    <ShieldCheck className="h-4 w-4 shrink-0" aria-hidden="true" />
                                )}
                                {(status === "idle" || status === "error") && (
                                    <ArrowRight className="h-4 w-4 shrink-0" aria-hidden="true" />
                                )}
                                
                                {status === "idle" && "Sign in"}
                                {status === "loading" && "Signing in..."}
                                {status === "success" && "Login successful. Redirecting..."}
                                {status === "error" && `${message || "Login failed"} - Try again`}
                            </Button>
                        </form>
                    </div>
                </section>
            </main>
        </DefaultLayout>
    );
}
