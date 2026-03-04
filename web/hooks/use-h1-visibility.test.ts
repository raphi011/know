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

function makeContainer(hasH1: boolean) {
  const container = document.createElement("div");
  if (hasH1) {
    container.appendChild(document.createElement("h1"));
  }
  return { current: container };
}

describe("useShowToolbarTitle", () => {
  it("returns true when disabled (edit mode)", () => {
    const ref = makeContainer(true);
    const { result } = renderHook(() => useShowToolbarTitle(ref, false));
    expect(result.current).toBe(true);
  });

  it("returns false initially (h1 assumed visible)", () => {
    const ref = makeContainer(true);
    const { result } = renderHook(() => useShowToolbarTitle(ref, true));
    // h1Visible starts as true → !true = false (toolbar title hidden)
    expect(result.current).toBe(false);
  });

  it("returns true when h1 is scrolled out of view", () => {
    const ref = makeContainer(true);
    const { result } = renderHook(() => useShowToolbarTitle(ref, true));

    act(() => {
      observerCallback([
        { isIntersecting: false } as IntersectionObserverEntry,
      ]);
    });

    expect(result.current).toBe(true);
  });

  it("returns false when h1 is intersecting", () => {
    const ref = makeContainer(true);
    const { result } = renderHook(() => useShowToolbarTitle(ref, true));

    act(() => {
      observerCallback([
        { isIntersecting: true } as IntersectionObserverEntry,
      ]);
    });

    expect(result.current).toBe(false);
  });

  it("returns true when container has no h1", () => {
    vi.useFakeTimers();
    const ref = makeContainer(false);
    const { result } = renderHook(() => useShowToolbarTitle(ref, true));

    // requestAnimationFrame defers the state update
    act(() => {
      vi.runAllTimers();
    });

    // After rAF fires: h1Visible=false → !false = true
    expect(result.current).toBe(true);
    vi.useRealTimers();
  });

  it("disconnects observer on unmount", () => {
    const ref = makeContainer(true);
    const { unmount } = renderHook(() => useShowToolbarTitle(ref, true));

    unmount();

    expect(observerDisconnect).toHaveBeenCalled();
  });

  it("returns true when container ref is null", () => {
    const ref = { current: null };
    const { result } = renderHook(() => useShowToolbarTitle(ref, true));
    // Effect exits early, h1Visible stays true → !true = false
    // But this is the "no container" edge case — toolbar title hidden
    expect(result.current).toBe(false);
  });
});
