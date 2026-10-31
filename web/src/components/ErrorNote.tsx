import { ApiError } from '../api/client';

// Pesan untuk pengguna dibuat di sisi UI (bukan menampilkan teks server mentah), dengan error_id untuk dilaporkan.
export function describeError(e: unknown): { message: string; errorId?: string } {
  if (e instanceof ApiError) {
    if (e.status === 401) return { message: 'Your session has ended. Sign in again.', errorId: e.errorId };
    if (e.status === 403) return { message: 'You do not have permission to do that.', errorId: e.errorId };
    if (e.status === 404) return { message: 'We could not find that.', errorId: e.errorId };
    if (e.status >= 500) return { message: 'The platform hit an unexpected error. Your data is safe.', errorId: e.errorId };
    // Penolakan validasi membawa alasan sebenarnya di fields; teks umum di detail tidak menolong pengguna.
    if (e.fields.length > 0) return { message: e.fields.map((f) => f.message).join(' '), errorId: e.errorId };
    return { message: e.detail || 'The request was rejected.', errorId: e.errorId };
  }
  return { message: 'The platform did not answer. Check your connection and try again.' };
}
