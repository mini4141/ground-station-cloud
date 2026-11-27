import { useEffect, useRef, useState } from "react";

/** Calls fn immediately and then every intervalMs; returns the latest result. */
export function usePoll<T>(fn: () => Promise<T>, intervalMs: number, deps: unknown[] = []): T | undefined {
  const [value, setValue] = useState<T>();
  const ref = useRef(fn);
  ref.current = fn;
  useEffect(() => {
    let alive = true;
    const tick = () =>
      ref.current()
        .then((v) => alive && setValue(v))
        .catch(() => undefined);
    tick();
    const id = setInterval(tick, intervalMs);
    return () => {
      alive = false;
      clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [intervalMs, ...deps]);
  return value;
}
