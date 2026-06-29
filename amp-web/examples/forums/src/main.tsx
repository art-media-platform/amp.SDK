import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { AmpProvider, AmpWebClient } from '@art-media-platform/web';
import { App } from './App';
import './styles/forums.css';

const client = new AmpWebClient({
  vaultUrl: import.meta.env.VITE_AMP_VAULT_URL ?? 'http://localhost:5193',
  planetTag: import.meta.env.VITE_AMP_PLANET_TAG ?? '',
});

async function boot() {
  // Embedded in the Unity host: exchange the injected memberToken for a session — no second
  // login screen (AD-app-forums.md §6.5).  A pure browser has no window.__amp and logs in normally.
  if (typeof window !== 'undefined' && window.__amp?.embed && window.__amp.memberToken) {
    try {
      await client.login({ Scheme: 'memberToken', MemberToken: window.__amp.memberToken });
    } catch (err) {
      console.warn('amp embed SSO failed; falling back to manual login', err);
    }
  }

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <AmpProvider client={client}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </AmpProvider>
    </React.StrictMode>,
  );
}

void boot();
