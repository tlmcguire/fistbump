// Requests handed from the app shell (menus, shortcuts, file drops) to a view that may not be mounted
// yet. The view takes the request when it mounts or is shown again.
let pending = null;

export function request(view, action, data) {
  pending = { view, action, data };
  window.dispatchEvent(new CustomEvent('fistbump:request', { detail: view }));
}

// take returns and clears the pending request for a view, if any.
export function take(view) {
  if (!pending || pending.view !== view) return null;
  const p = pending;
  pending = null;
  return p;
}
