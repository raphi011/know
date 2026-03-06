// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useShowToolbarTitle } from "./use-h1-visibility";

type IntersectionCallback = (entries: IntersectionObserverEntry[]) => void;

let observerCallback: IntersectionCallback;
let observerDisconnect: ReturnType<typeof vi.fn>;

beforeEach(() => {
  observerDisconnect = vi.fn();

  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(cb: IntersectionCallback) {
        observerCallback = cb;
      }
      observe() {}
      disconnect = observerDisconnect;
    },
  );
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useShowToolbarTitle", () => {
  it("returns true when disabled (edit mode)", () => {
    const { result } = renderHook(() => useShowToolbarTitle(false));
    expect(result.current).toBe(true);
  });

  it("returns false initially when h1 exists (h1 assumed visible)", () => {
    document.body.appendChild(document.createElement("h1"));
    const { result } = renderHook(() => useShowToolbarTitle(true));
    // h1Visible starts as true → !true = false (toolbar title hidden)
    expect(result.current).toBe(false);
    document.body.innerHTML = "";
  });

  it("returns true when h1 is scrolled out of view", () => {
    document.body.appendChild(document.createElement("h1"));
    const { result } = renderHook(() => useShowToolbarTitle(true));

    act(() => {
      observerCallback([
        { isIntersecting: false } as IntersectionObserverEntry,
      ]);
    });

    expect(result.current).toBe(true);
    document.body.innerHTML = "";
  });

  it("returns false when h1 is intersecting", () => {
    document.body.appendChild(document.createElement("h1"));
    const { result } = renderHook(() => useShowToolbarTitle(true));

    act(() => {
      observerCallback([
        { isIntersecting: true } as IntersectionObserverEntry,
      ]);
    });

    expect(result.current).toBe(false);
    document.body.innerHTML = "";
  });

  it("returns true when document has no h1", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useShowToolbarTitle(true));

    // requestAnimationFrame defers the state update
    act(() => {
      vi.runAllTimers();
    });

    // After rAF fires: h1Visible=false → !false = true
    expect(result.current).toBe(true);
    vi.useRealTimers();
  });

  it("disconnects observer on unmount", () => {
    document.body.appendChild(document.createElement("h1"));
    const { unmount } = renderHook(() => useShowToolbarTitle(true));

    unmount();

    expect(observerDisconnect).toHaveBeenCalled();
    document.body.innerHTML = "";
  });
});
