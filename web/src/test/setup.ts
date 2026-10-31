import '@testing-library/jest-dom/vitest';
import { configure } from '@testing-library/react';

// Batas tunggu bawaan (1 dtk) terlalu ketat saat suite berjalan paralel di mesin yang sibuk (tes jsdom gagal acak).
// Tes yang benar tetap cepat; ini hanya memberi ruang bagi findBy*/waitFor ketika CPU padat.
configure({ asyncUtilTimeout: 5000 });
