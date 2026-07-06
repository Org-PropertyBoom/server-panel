import { useState, useRef, useEffect } from "react";
import {
    Bot,
    Send,
    MessageSquare,
    Plus,
    RefreshCw,
    Loader2,
    Sparkles,
    User,
} from "lucide-react";

import DashboardLayout from "_layouts/dashboard";
import { Button } from "_layouts/_components/ui/button";

interface ChatMessage {
    id: string;
    sender: "user" | "agent";
    text: string;
    timestamp: Date;
}

interface ChatSession {
    id: string;
    title: string;
    icon: any;
    messages: ChatMessage[];
}

export default function AgentRoute() {
    const [sessions, setSessions] = useState<ChatSession[]>([
        {
            id: "1",
            title: "General Assistant",
            icon: Sparkles,
            messages: [
                {
                    id: "m1",
                    sender: "agent",
                    text: "Hello! I am your MThan VPS AI Assistant. I can help you monitor server metrics, draft Nginx virtual host configs, check active modules, or manage system users. What would you like to do today?",
                    timestamp: new Date(),
                },
            ],
        },
        {
            id: "2",
            title: "Configure Nginx Reverse Proxy",
            icon: MessageSquare,
            messages: [
                {
                    id: "m2",
                    sender: "agent",
                    text: "I can help configure a reverse proxy. What is the target domain and backend port?",
                    timestamp: new Date(Date.now() - 3600000),
                },
            ],
        },
    ]);

    const [activeSessionId, setActiveSessionId] = useState<string>("1");
    const [inputText, setInputText] = useState("");
    const [isThinking, setIsThinking] = useState(false);
    
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const activeSession = sessions.find((s) => s.id === activeSessionId) || sessions[0];

    const scrollToBottom = () => {
        messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    };

    useEffect(() => {
        scrollToBottom();
    }, [activeSession.messages, isThinking]);

    const handleSendMessage = (e: React.FormEvent) => {
        e.preventDefault();
        if (!inputText.trim()) return;

        const userMsg: ChatMessage = {
            id: String(Date.now()),
            sender: "user",
            text: inputText,
            timestamp: new Date(),
        };

        const updatedMessages = [...activeSession.messages, userMsg];

        setSessions((prev) =>
            prev.map((s) => (s.id === activeSessionId ? { ...s, messages: updatedMessages } : s))
        );

        const promptText = inputText.toLowerCase();
        setInputText("");
        setIsThinking(true);

        // Generate response
        setTimeout(() => {
            let replyText = "I've logged your request regarding that system action. I can help automate this task or guide you to the correct menu in the panel.";
            
            if (promptText.includes("hello") || promptText.includes("hi")) {
                replyText = "Hello! How can I assist you with your server administration today?";
            } else if (promptText.includes("nginx") || promptText.includes("vhost") || promptText.includes("domain")) {
                replyText = "To configure web domains or proxy routes, you can head over to the VHosts tab. For root configurations, you can inspect `/etc/nginx` inside the Files explorer.";
            } else if (promptText.includes("user") || promptText.includes("password")) {
                replyText = "You can manage standard Linux system users (adding accounts, configuring environments) inside the Users section. Superusers can also manage passwords via terminal.";
            } else if (promptText.includes("module") || promptText.includes("mariadb") || promptText.includes("redis") || promptText.includes("php")) {
                replyText = "Active system services like MariaDB, Redis, and PHP-FPM can be checked, stopped, or restarted inside the Modules tab.";
            } else if (promptText.includes("help") || promptText.includes("what can you do")) {
                replyText = "I can guide you on how to manage Virtual Hosts, monitor system Modules (Nginx, MariaDB, PHP-FPM, Redis), edit system configurations in Files, and add or manage Linux users.";
            }

            const agentMsg: ChatMessage = {
                id: String(Date.now() + 1),
                sender: "agent",
                text: replyText,
                timestamp: new Date(),
            };

            setSessions((prev) =>
                prev.map((s) =>
                    s.id === activeSessionId
                        ? { ...s, messages: [...updatedMessages, agentMsg] }
                        : s
                )
            );
            setIsThinking(false);
        }, 1000);
    };

    const handleCreateSession = () => {
        const newId = String(Date.now());
        const newSession: ChatSession = {
            id: newId,
            title: `New Session ${sessions.length + 1}`,
            icon: MessageSquare,
            messages: [
                {
                    id: String(Date.now() + 1),
                    sender: "agent",
                    text: "How can I help you with this new task?",
                    timestamp: new Date(),
                },
            ],
        };
        setSessions((prev) => [...prev, newSession]);
        setActiveSessionId(newId);
    };

    return (
        <DashboardLayout
            title="AI Agent"
            description="Manage your VPS configuration and services using natural language assistant."
            fullWidth={true}
        >
            <div className="grid grid-cols-1 md:grid-cols-[280px_1fr] border border-border rounded-lg bg-card overflow-hidden h-[calc(100vh-220px)] shadow-lg">
                {/* Left Sidebar - Chat Sessions */}
                <aside className="border-r border-border bg-card/60 flex flex-col h-full overflow-hidden select-none">
                    <div className="flex h-10 items-center justify-between px-3 border-b border-border bg-muted/20">
                        <span className="text-xs font-semibold text-muted-foreground">
                            Chat Sessions
                        </span>
                        <button
                            onClick={handleCreateSession}
                            className="p-1 rounded text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
                            title="New Session"
                        >
                            <Plus className="h-3.5 w-3.5" />
                        </button>
                    </div>

                    <div className="flex-1 overflow-y-auto py-2 px-2 space-y-1">
                        {sessions.map((s) => {
                            const isSelected = activeSessionId === s.id;
                            const Icon = s.icon;
                            return (
                                <div
                                    key={s.id}
                                    className={`flex items-center gap-2.5 py-2 px-2.5 rounded-md cursor-pointer hover:bg-muted/60 transition-colors text-xs ${
                                        isSelected
                                            ? "bg-primary/10 text-primary font-semibold"
                                            : "text-foreground/90"
                                    }`}
                                    onClick={() => setActiveSessionId(s.id)}
                                >
                                    <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                                    <span className="truncate flex-1 min-w-0">{s.title}</span>
                                </div>
                            );
                        })}
                    </div>
                </aside>

                {/* Right Chat Panel */}
                <main className="flex flex-col h-full overflow-hidden bg-background">
                    {/* Chat Area */}
                    <div className="flex-1 overflow-y-auto p-6 space-y-4">
                        {activeSession.messages.map((msg) => {
                            const isAgent = msg.sender === "agent";
                            return (
                                <div
                                    key={msg.id}
                                    className={`flex gap-3 max-w-2xl ${
                                        isAgent ? "mr-auto" : "ml-auto flex-row-reverse"
                                    }`}
                                >
                                    {/* Avatar */}
                                    <div className={`h-8 w-8 rounded-full border border-border flex items-center justify-center shrink-0 ${
                                        isAgent ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"
                                    }`}>
                                        {isAgent ? (
                                            <Bot className="h-4 w-4" />
                                        ) : (
                                            <User className="h-4 w-4" />
                                        )}
                                    </div>

                                    {/* Bubble */}
                                    <div className={`p-4 rounded-xl border border-border text-sm leading-relaxed ${
                                        isAgent
                                            ? "bg-card text-card-foreground"
                                            : "bg-primary text-primary-foreground border-primary"
                                    }`}>
                                        <p className="whitespace-pre-line">{msg.text}</p>
                                    </div>
                                </div>
                            );
                        })}

                        {/* Thinking State indicator */}
                        {isThinking && (
                            <div className="flex gap-3 mr-auto max-w-2xl animate-pulse">
                                <div className="h-8 w-8 rounded-full border border-border bg-primary/10 text-primary flex items-center justify-center shrink-0">
                                    <Bot className="h-4 w-4" />
                                </div>
                                <div className="p-4 rounded-xl border border-border bg-card text-muted-foreground text-sm flex items-center gap-2">
                                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                    <span>Agent is typing...</span>
                                </div>
                            </div>
                        )}

                        <div ref={messagesEndRef} />
                    </div>

                    {/* Input Area */}
                    <div className="p-4 border-t border-border bg-card/40 shrink-0">
                        <form onSubmit={handleSendMessage} className="flex gap-2 max-w-4xl mx-auto">
                            <input
                                type="text"
                                placeholder="Ask your AI Agent to configure vhosts, inspect files, or manage services..."
                                className="flex-grow bg-card border border-border rounded-lg px-4 py-2 text-sm outline-none placeholder:text-muted-foreground focus:border-primary transition-colors"
                                value={inputText}
                                onChange={(e) => setInputText(e.target.value)}
                                disabled={isThinking}
                            />
                            <Button type="submit" size="icon" disabled={isThinking || !inputText.trim()}>
                                <Send className="h-4 w-4" />
                            </Button>
                        </form>
                    </div>
                </main>
            </div>
        </DashboardLayout>
    );
}
