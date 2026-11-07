import '@fontsource-variable/inter';
import '@fontsource-variable/space-grotesk';
import '@fontsource/ibm-plex-mono/400.css';
import '@fontsource/ibm-plex-mono/600.css';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import {LanguageSession} from './i18n/LanguagePreference';
import { UNAUTHORIZED_EVENT } from './api/client';
import './styles/tokens.css';
import './styles/ui2.css';
import './styles/app.css';
import { initTheme } from './lib/theme';
import { applicationStylesReady, showStylesheetFailure } from './styles-ready';

const client = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false } } });

// Sesi berakhir di tengah pemakaian: anggap tidak masuk lagi (Guard mengalihkan ke login) dan buang data tersimpan.
window.addEventListener(UNAUTHORIZED_EVENT, () => {
  client.setQueryData(['me'], null);
  client.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' });
});

initTheme();
const root = document.getElementById('root')!;
if (await applicationStylesReady()) {
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <LanguageSession /><App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
} else {
  showStylesheetFailure(root);
}
