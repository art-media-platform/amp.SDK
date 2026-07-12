# SIWE Primer — Wallet Login Without the Web3 Homework

amp's `'wallet'` login scheme is **EIP-4361, "Sign-In with Ethereum"
(SIWE)**. You do not need tokens, gas, a chain connection, or any web3
stack — a wallet here is just a keypair with a browser UI. This page
gives you the mental model plus a drop-in `useWalletLogin()` hook.

## Why a Wallet Login at All

- **No password database.** The member proves control of a keypair by
  signing a one-time challenge; the server verifies the signature.
  Nothing secret is stored server-side, nothing to breach.
- **Deterministic identity.** The MemberID derives from
  `eth:<lowercase-address>` — the same wallet always resolves to the
  same member (`SECURITY-amp-web-SDK.md`, "Identity & login").
- **Self-provisioning.** First wallet login auto-provisions a private
  home planet — no operator step (`docs/get-a-backend.md`).

## The Flow (Three Round-Trips)

```
1. wallet  →  app     eth_requestAccounts        → address
2. app     →  node    getWalletChallenge(address) → { Nonce, Message }
3. wallet  →  app     personal_sign(Message)      → signature
   app     →  node    login({ Scheme:'wallet', Address, Signature, Nonce })
```

The `Message` is a human-readable EIP-4361 text the **server** builds,
bound to its own domain (anti-phishing) and to a single-use `Nonce`
(anti-replay). The wallet shows it to the user before signing. The
canonical serialization stays server-side — never construct the message
yourself.

## `useWalletLogin()` — Drop-In Hook

The SDK deliberately ships no wallet code (wallet UX is app-land; the
brand belongs in your UI). This reference implementation is yours to
paste and own — `src/useWalletLogin.ts`:

```tsx
import { useCallback, useEffect, useState } from 'react';
import { useAmpAuth, useAmpClient } from '@art-media-platform/web';

// EIP-1193 provider surface + EIP-6963 announcement (untyped in the DOM libs).
interface Eip1193Provider {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}
export interface DiscoveredWallet {
  info: { uuid: string; name: string; icon: string; rdns: string };
  provider: Eip1193Provider;
}

/**
 * Wallet (SIWE) login: EIP-6963 wallet discovery + the challenge/sign/
 * submit round-trip.  `wallets` lists every announced extension (render
 * name + icon and let the user pick); `loginWith(wallet)` completes the
 * amp login.  Falls back to window.ethereum when nothing announces.
 */
export function useWalletLogin() {
  const { login } = useAmpAuth();
  const client = useAmpClient();
  const [wallets, setWallets] = useState<DiscoveredWallet[]>([]);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    const seen = new Map<string, DiscoveredWallet>();
    const onAnnounce = (evt: Event) => {
      const wallet = (evt as CustomEvent<DiscoveredWallet>).detail;
      seen.set(wallet.info.rdns, wallet);
      setWallets([...seen.values()]);
    };
    window.addEventListener('eip6963:announceProvider', onAnnounce);
    window.dispatchEvent(new Event('eip6963:requestProvider'));
    return () =>
      window.removeEventListener('eip6963:announceProvider', onAnnounce);
  }, []);

  const loginWith = useCallback(
    async (wallet?: DiscoveredWallet) => {
      const provider =
        wallet?.provider ??
        (window as { ethereum?: Eip1193Provider }).ethereum;
      if (!provider) throw new Error('no EVM wallet extension found');
      setPending(true);
      try {
        const [address] = (await provider.request({
          method: 'eth_requestAccounts',
        })) as string[];
        const challenge = await client.getWalletChallenge(address);
        const signature = (await provider.request({
          method: 'personal_sign',
          params: [challenge.Message, address],
        })) as string;
        return await login({
          Scheme: 'wallet',
          Address: address,
          Signature: signature,
          Nonce: challenge.Nonce,
        });
      } finally {
        setPending(false);
      }
    },
    [client, login],
  );

  return { wallets, loginWith, pending };
}
```

Usage:

```tsx
const { wallets, loginWith, pending } = useWalletLogin();

return wallets.length > 0 ? (
  <ul>
    {wallets.map(w => (
      <li key={w.info.rdns}>
        <button disabled={pending} onClick={() => void loginWith(w)}>
          <img src={w.info.icon} alt="" width={16} /> {w.info.name}
        </button>
      </li>
    ))}
  </ul>
) : (
  <button disabled={pending} onClick={() => void loginWith()}>
    Sign in with wallet
  </button>
);
```

A packaged `useWalletLogin` export is a candidate for a future SDK
revision; until then the hook above is the supported pattern.

## Common Questions

- **Does this cost gas / touch a chain?** No. `personal_sign` is an
  offline signature; nothing is transacted anywhere.
- **Which wallets work?** Any EVM wallet extension (MetaMask, Coinbase
  Wallet, Rainbow, …) — they all speak EIP-1193 `personal_sign`, and
  EIP-6963 is how multiple extensions coexist without fighting over
  `window.ethereum`.
- **What about DID login?** `did:pkh:eip155:…` resolves to the **same
  member** as a wallet login over that address; `did:key` covers
  Ed25519 keys. Same challenge/sign/submit shape — SKILL §5.1.
- **Mobile / no extension?** WalletConnect-style transports also
  surface an EIP-1193 provider; hand it to `loginWith` unchanged.
