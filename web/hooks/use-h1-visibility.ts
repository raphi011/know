import { useEffect, useState, type RefObject } from "react";

/**
 * Returns `true` when the toolbar title should be shown — i.e. when the
 * first <h1> inside the scroll container is not intersecting the container.
 *
 * - `root` is set to the scroll container (not the viewport)
 * - When `enabled` is false (edit mode), always returns `true`
 * - When no H1 exists in the container, returns `true`
 */
function useShowToolbarTitle(
  containerRef: RefObject<HTMLElement | null>,
  enabled: boolean,
): boolean {
  const [h1Visible, setH1Visible] = useState(true);

  useEffect(() => {
    if (!enabled) return;

    const container = containerRef.current;
    if (!container) return;

    const h1 = container.querySelector("h1");
    if (!h1) {
      // Defer to avoid synchronous setState in effect body (react-hooks/set-state-in-effect).
      // The one-frame delay is imperceptible since the toolbar uses a 200ms opacity transition.
      const frame = requestAnimationFrame(() => setH1Visible(false));
      return () => cancelAnimationFrame(frame);
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry) setH1Visible(entry.isIntersecting);
      },
      { root: container, threshold: 0 },
    );

    observer.observe(h1);
    return () => observer.disconnect();
  }, [containerRef, enabled]);

  if (!enabled) return true;

  return !h1Visible;
}

export { useShowToolbarTitle };
