import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, UNAUTHORIZED_EVENT } from './client';

afterEach(() => vi.restoreAllMocks());

describe('api client', () => {
  it('401 di jalur biasa memberi tahu aplikasi bahwa sesi berakhir', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify({ title: 'Unauthorized', status: 401 }), { status: 401 }));
    const seen = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, seen);
    await expect(api.get('/api/v1/findings')).rejects.toMatchObject({ status: 401 });
    window.removeEventListener(UNAUTHORIZED_EVENT, seen);
    expect(seen).toHaveBeenCalledTimes(1);
  });
  it('401 dari jalur /auth/ (login salah, token reset) bukan sesi berakhir', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify({ title: 'Unauthorized', status: 401 }), { status: 401 }));
    const seen = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, seen);
    await expect(api.post('/api/v1/auth/login', {})).rejects.toMatchObject({ status: 401 });
    await expect(api.get('/api/v1/auth/me')).rejects.toMatchObject({ status: 401 });
    window.removeEventListener(UNAUTHORIZED_EVENT, seen);
    expect(seen).not.toHaveBeenCalled();
  });
  it('galat lain tidak memicu peristiwa', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ title: 'Forbidden', status: 403 }), { status: 403 }));
    const seen = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, seen);
    await expect(api.get('/api/v1/users')).rejects.toMatchObject({ status: 403 });
    window.removeEventListener(UNAUTHORIZED_EVENT, seen);
    expect(seen).not.toHaveBeenCalled();
  });
});
