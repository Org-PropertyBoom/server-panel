import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { Terminal } from "xterm";
import { toast } from "sonner";
import { copyTerminalSelection } from "./terminal-panel";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// A stand-in for xterm's Terminal: copyTerminalSelection only reads the selection.
const fakeTerm = (selection: string) => ({ getSelection: () => selection }) as unknown as Terminal;

const setClipboard = (value: unknown) =>
    Object.defineProperty(navigator, "clipboard", { value, configurable: true });

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
    if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
    else setClipboard(undefined);
});

describe("copyTerminalSelection", () => {
    it("does nothing when there is no selection", () => {
        const writeText = vi.fn();
        setClipboard({ writeText });
        expect(copyTerminalSelection(fakeTerm(""))).toBe(false);
        expect(writeText).not.toHaveBeenCalled();
        expect(toast.success).not.toHaveBeenCalled();
        expect(toast.error).not.toHaveBeenCalled();
    });

    it("copies the selection and confirms", async () => {
        const writeText = vi.fn().mockResolvedValue(undefined);
        setClipboard({ writeText });
        expect(copyTerminalSelection(fakeTerm("secret-value"))).toBe(true);
        expect(writeText).toHaveBeenCalledWith("secret-value");
        await vi.waitFor(() => expect(toast.success).toHaveBeenCalled());
        expect(toast.error).not.toHaveBeenCalled();
    });

    it("reports a rejected clipboard write", async () => {
        setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
        expect(copyTerminalSelection(fakeTerm("x"))).toBe(true);
        await vi.waitFor(() => expect(toast.error).toHaveBeenCalled());
        expect(toast.success).not.toHaveBeenCalled();
    });

    // The regression. Outside a secure context (plain HTTP: a LAN IP, or `make dev`
    // on :8000) navigator.clipboard is undefined. Reading .writeText off it throws
    // SYNCHRONOUSLY, which .then(onRejected) cannot catch — so the copy failed with
    // no toast at all, the one case the helper exists to make visible.
    it("warns instead of throwing when the clipboard API is unavailable", () => {
        setClipboard(undefined);
        expect(() => copyTerminalSelection(fakeTerm("x"))).not.toThrow();
        expect(copyTerminalSelection(fakeTerm("x"))).toBe(true); // handled: not sent to the shell
        expect(toast.error).toHaveBeenCalled();
        expect(toast.success).not.toHaveBeenCalled();
    });
});
