import { t as textUI } from "../i18n/runtime";
import { ApiError } from '../api/client';
// Pesan untuk pengguna dibuat di sisi UI (bukan menampilkan teks server mentah), dengan error_id untuk dilaporkan.
export function describeError(e: unknown): {
    message: string;
    errorId?: string;
} {
    if (e instanceof ApiError) {
        if (e.status === 401)
            return { message: textUI("m_176c077ea16e"), errorId: e.errorId };
        if (e.status === 403)
            return { message: textUI("m_5d6c52bee53d"), errorId: e.errorId };
        if (e.status === 404)
            return { message: textUI("m_f22846bedfd8"), errorId: e.errorId };
        if (e.status >= 500)
            return { message: textUI("m_568ab9d976c0"), errorId: e.errorId };
        // Penolakan validasi membawa alasan sebenarnya di fields; teks umum di detail tidak menolong pengguna.
        if (e.fields.length > 0)
            return { message: e.fields.map((f) => f.message).join(' '), errorId: e.errorId };
        return { message: e.detail || textUI("m_10178671a688"), errorId: e.errorId };
    }
    return { message: textUI("m_c44d6e4d7c76") };
}
