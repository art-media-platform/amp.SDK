# Your First Channel

A from-zero tutorial: a React app that logs in with a wallet, writes a
note to a channel, and watches it update live. Every block below is
copy-paste-complete. You need a node URL (`docs/get-a-backend.md`) and a
browser wallet extension (any EVM wallet — `docs/siwe-primer.md`
explains the login mechanics).

## 1. Scaffold and Install

```bash
npm create vite@latest my-amp-app -- --template react-ts
cd my-amp-app
npm install
unzip ~/Downloads/amp-web-SDK-v260.zip -d .   # → ./amp-web-SDK, INSIDE the project
npm install ./amp-web-SDK
```

(The bundle must sit inside the project — see
`docs/install-troubleshooting.md` if imports fail.)

Create `.env.local`:

```env
VITE_AMP_VAULT_URL=https://prod.plan.tools
VITE_AMP_PLANET_TAG=
```

`VITE_AMP_VAULT_URL` is your operated node. Leave `VITE_AMP_PLANET_TAG`
empty to write to the home planet your wallet login auto-provisions; put
the handed planet tag there once you've accepted the deploy's invite
(SKILL §4.7).

## 2. Wire the Provider

Replace `src/main.tsx`:

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { AmpProvider, AmpWebClient } from '@art-media-platform/web';
import App from './App';

const client = new AmpWebClient({
  vaultUrl:  import.meta.env.VITE_AMP_VAULT_URL,
  planetTag: import.meta.env.VITE_AMP_PLANET_TAG ?? '',
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AmpProvider client={client}>
      <App />
    </AmpProvider>
  </React.StrictMode>,
);
```

## 3. Login, Write, Read Live

Replace `src/App.tsx`:

```tsx
import { useState } from 'react';
import {
  useAmpAuth,
  useAmpClient,
  useAmpMutation,
  useAmpQuery,
} from '@art-media-platform/web';

// Minimal EIP-1193 wallet glue (the SDK ships no wallet code — this is
// your app's; docs/siwe-primer.md covers multi-wallet discovery).
declare global {
  interface Window {
    ethereum?: {
      request(args: { method: string; params?: unknown[] }): Promise<unknown>;
    };
  }
}

interface Note {
  text: string;
}

export default function App() {
  const { member, login, logout, isAuthenticated, loading } = useAmpAuth();
  const client = useAmpClient();
  const { create } = useAmpMutation();
  const { data: notes, loading: reading, error } =
    useAmpQuery<Note>('my-first-channel', 'notes');
  const [draft, setDraft] = useState('');

  async function walletLogin() {
    if (!window.ethereum) throw new Error('no wallet extension found');
    const accounts = (await window.ethereum.request({
      method: 'eth_requestAccounts',
    })) as string[];
    const address = accounts[0];
    const challenge = await client.getWalletChallenge(address);
    const signature = (await window.ethereum.request({
      method: 'personal_sign',
      params: [challenge.Message, address],
    })) as string;
    await login({
      Scheme: 'wallet',
      Address: address,
      Signature: signature,
      Nonce: challenge.Nonce,
    });
  }

  async function addNote() {
    await create('my-first-channel', 'notes', { text: draft });
    setDraft('');   // the live subscription re-renders the list
  }

  if (loading) return <p>Loading…</p>;
  if (!isAuthenticated) {
    return <button onClick={() => void walletLogin()}>Sign in with wallet</button>;
  }
  return (
    <main>
      <p>
        Signed in as {member?.DisplayName ?? member?.ID}{' '}
        <button onClick={() => void logout()}>Sign out</button>
      </p>
      <input value={draft} onChange={e => setDraft(e.target.value)} />
      <button disabled={!draft} onClick={() => void addNote()}>Add note</button>
      {error && <p>read failed: {error.message}</p>}
      {reading ? <p>Reading…</p> : (
        <ul>
          {notes.map(note => <li key={note._ItemID}>{note.text}</li>)}
        </ul>
      )}
    </main>
  );
}
```

## 4. Run It

```bash
npm run dev      # http://localhost:5173
```

Sign in (the wallet prompts once for the SIWE signature — first login
auto-provisions your home planet), type a note, Add. The note appears in
the list via the WebSocket subscription, not a refetch — open a second
tab, add a note there, and watch both update.

## What Just Happened

- `'my-first-channel'` / `'notes'` were canonized server-side to
  `tag.UID`s on first use (SKILL §5.8) — no schema, no migration.
- `create(...)` sent one signed, encrypted TxMsg; the vault journaled it
  without being able to read it (`SECURITY-amp-web-SDK.md`).
- `useAmpQuery` subscribed to exactly that `(channel, attr)` cell
  (`docs/concepts-primer.md`) once logged in; anonymous reads are
  fetch-only.

## Where Next

- Batch writes and verb-RPC: SKILL §5.3.
- A real app built this way: `examples/forums/`.
- A `403` on write means the planet tag names a planet you're not a
  member of yet — accept the deploy's invite (SKILL §4.7) or clear
  `VITE_AMP_PLANET_TAG` to use your home planet.
