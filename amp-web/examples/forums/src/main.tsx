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

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AmpProvider client={client}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </AmpProvider>
  </React.StrictMode>,
);
