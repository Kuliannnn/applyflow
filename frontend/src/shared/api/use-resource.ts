import { useEffect, useState } from "react";
import { request } from "./client";
export function useResource<T>(path: string | null, interval = 0) {
  const [value, setValue] = useState<{ path: string; data: T }>(),
    [error, setError] = useState<unknown>(),
    [epoch, reload] = useState(0);
  useEffect(() => {
    if (!path) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const ctrl = new AbortController();
    async function load() {
      try {
        const data = await request<T>(path!, { signal: ctrl.signal });
        if (alive) {
          setValue({ path: path!, data });
          setError(undefined);
        }
      } catch (e) {
        if (alive) setError(e);
      } finally {
        if (alive && interval) timer = setTimeout(load, interval);
      }
    }
    void load();
    return () => {
      alive = false;
      ctrl.abort();
      clearTimeout(timer);
    };
  }, [path, interval, epoch]);
  return {
    data: value?.path === path ? value.data : undefined,
    error,
    reload: () => reload((x) => x + 1),
  };
}
