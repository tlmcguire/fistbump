// Wraps window.api (the preload bridge): unwraps { ok, data } and throws ApiError on failure.
export class ApiError extends Error {
  constructor(error, status) {
    super(error?.message || 'Request failed');
    this.code = error?.code || 'unknown';
    this.details = error?.details || null;
    this.status = status || 0;
  }
}

function wrap(fn) {
  return async (...args) => {
    const r = await fn(...args);
    if (r && r.ok) return r.data;
    throw new ApiError(r?.error, r?.status);
  };
}

export const api = {};
for (const [group, methods] of Object.entries(window.api || {})) {
  api[group] = {};
  for (const [name, fn] of Object.entries(methods)) api[group][name] = wrap(fn);
}
