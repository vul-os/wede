const BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080';

let _token = localStorage.getItem('token') || '';

export function setToken(t) {
  _token = t;
  localStorage.setItem('token', t);
}

function authHeaders() {
  return {
    'Content-Type': 'application/json',
    ..._token ? { Authorization: `Bearer ${_token}` } : {},
  };
}

async function request(method, path, body) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: authHeaders(),
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });

  if (res.status === 204) return null;

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    throw new Error(data?.message || data?.error || `HTTP ${res.status}`);
  }

  return data;
}

export const fetchTasks  = ()                => request('GET',    '/tasks');
export const createTask  = (title, priority) => request('POST',   '/tasks', { title, priority });
export const updateTask  = (id, patch)       => request('PATCH',  `/tasks/${id}`, patch);
export const deleteTask  = (id)              => request('DELETE', `/tasks/${id}`);
export const login       = (u, p)            => request('POST',   '/auth/login', { username: u, password: p });
