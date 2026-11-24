type Handle = ReturnType<typeof setTimeout>;
type Timers = {
  set: (action: () => void, delay: number) => Handle;
  clear: (handle: Handle) => void;
};

export function createTrailingWrite(
  write: () => void,
  delay = 250,
  timers: Timers = { set: (fn, ms) => setTimeout(fn, ms), clear: (handle) => clearTimeout(handle) },
) {
  let pending = false;
  let handle: Handle | undefined;

  function flush() {
    if (handle !== undefined) {
      timers.clear(handle);
      handle = undefined;
    }
    if (!pending) return;
    pending = false;
    write();
  }
  function mark() {
    pending = true;
    if (handle !== undefined) timers.clear(handle);
    handle = timers.set(flush, delay);
  }
  function cancel() {
    if (handle !== undefined) timers.clear(handle);
    handle = undefined;
    pending = false;
  }
  return { mark, flush, cancel };
}
