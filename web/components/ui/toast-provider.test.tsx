// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { ToastProvider, useToast } from "./toast-provider";
import type { ReactNode } from "react";

const wrapper = ({ children }: { children: ReactNode }) => (
  <ToastProvider>{children}</ToastProvider>
);

describe("useToast", () => {
  it("starts with no toasts", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    expect(result.current.toasts).toEqual([]);
  });

  it("adds a toast", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "success", title: "Done" });
    });
    expect(result.current.toasts).toHaveLength(1);
    expect(result.current.toasts[0]!.title).toBe("Done");
  });

  it("dismisses a toast", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "error", title: "Oops" });
    });
    const id = result.current.toasts[0]!.id;
    act(() => {
      result.current.dismiss(id);
    });
    expect(result.current.toasts).toEqual([]);
  });

  it("auto-dismisses after timeout", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "info", title: "Temp", duration: 1000 });
    });
    expect(result.current.toasts).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.toasts).toEqual([]);
    vi.useRealTimers();
  });
});
