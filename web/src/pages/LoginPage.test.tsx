import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { LoginPage } from './LoginPage';

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/overview" element={<p>beranda</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const problem = (status: number, extra: object = {}) =>
  new Response(JSON.stringify({ title: 't', detail: 'd', status, error_id: 'err_abc12345', ...extra }), {
    status,
    headers: { 'Content-Type': 'application/problem+json' },
  });

afterEach(() => vi.restoreAllMocks());

describe('LoginPage', () => {
  it('menampilkan teks sesuai desain dan tanpa tombol SSO', () => {
    mount();
    expect(screen.getByRole('heading', { name: 'Sign in to ORCHIS' })).toBeInTheDocument();
    expect(screen.getByText('WELCOME BACK')).toBeInTheDocument();
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.queryByText(/SSO/i)).not.toBeInTheDocument();
  });

  it('memperingatkan Caps Lock hanya saat aktif di kolom kata sandi, lalu hilang saat kehilangan fokus', () => {
    mount();
    const pw = screen.getByLabelText('Password');
    expect(screen.queryByText('Caps Lock is on')).not.toBeInTheDocument();
    fireEvent.keyDown(pw, { key: 'A', modifierCapsLock: true });
    expect(screen.getByRole('status')).toHaveTextContent('Caps Lock is on');
    fireEvent.keyUp(pw, { key: 'A', modifierCapsLock: false });
    expect(screen.queryByText('Caps Lock is on')).not.toBeInTheDocument();
    fireEvent.keyDown(pw, { key: 'A', modifierCapsLock: true });
    fireEvent.blur(pw);
    expect(screen.queryByText('Caps Lock is on')).not.toBeInTheDocument();
  });

  it('ikon kolom hanya hiasan dan tautan lupa kata sandi memakai katalog bahasa', () => {
    const { container } = mount();
    const icons = container.querySelectorAll('.field__lead');
    expect(icons.length).toBe(2);
    icons.forEach((i) => expect(i.getAttribute('aria-hidden')).toBe('true'));
    expect(screen.getByRole('link', { name: 'Forgot password?' })).toHaveAttribute('href', '/forgot');
  });

  it('mengirim kredensial dengan header anti-CSRF lalu pindah ke beranda', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ id: '1', email: 'a@b.c', display_name: 'A', role: 'admin', permissions: [] }), { status: 200 }),
    );
    mount();
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Password'), 'rahasia');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    await waitFor(() => expect(screen.getByText('beranda')).toBeInTheDocument());
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/auth/login');
    expect((init!.headers as Record<string, string>)['X-Requested-With']).toBe('orchis-web');
    expect(JSON.parse(init!.body as string)).toEqual({ email: 'a@b.c', password: 'rahasia', remember_me: false });
  });

  it('menampilkan pesan generik pada 401 tanpa membocorkan sisa percobaan', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(problem(401));
    mount();
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Password'), 'salah');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Email or password is incorrect');
    expect(alert).not.toHaveTextContent(/attempt/i);
  });

  it('menampilkan layar akun terkunci dengan hitung mundur pada 429', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(problem(429, { retry_after_seconds: 905 }));
    mount();
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Password'), 'salah');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    expect(await screen.findByText('Too many attempts')).toBeInTheDocument();
    expect(screen.getByRole('timer')).toHaveTextContent('15:05');
    await userEvent.click(screen.getByRole('button', { name: /back to sign in/i }));
    expect(screen.getByRole('heading', { name: 'Sign in to ORCHIS' })).toBeInTheDocument();
  });

  it('menampilkan error_id saat server error 500', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(problem(500));
    mount();
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Password'), 'x');
    await userEvent.click(screen.getByRole('button', { name: /sign in/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent('err_abc12345');
  });
});
