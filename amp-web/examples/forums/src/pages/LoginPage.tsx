import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAmpAuth, useAmpClient } from '@art-media-platform/web';

// Minimal shape of an injected EIP-1193 wallet (e.g. MetaMask). Not an SDK type.
interface EthProvider {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}

export function LoginPage() {
  const { login, isAuthenticated } = useAmpAuth();
  const client = useAmpClient();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (isAuthenticated) navigate('/');
  }, [isAuthenticated, navigate]);

  async function emailLogin() {
    setBusy(true);
    setErr(null);
    try {
      await login({ Scheme: 'email', Email: email, Password: password });
      navigate('/');
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function walletLogin() {
    setBusy(true);
    setErr(null);
    try {
      const eth = (window as unknown as { ethereum?: EthProvider }).ethereum;
      if (!eth) throw new Error('No browser wallet found.');
      const accounts = await eth.request({ method: 'eth_requestAccounts' }) as string[];
      const address = accounts[0];
      const challenge = await client.getWalletChallenge(address);
      const signature = await eth.request({ method: 'personal_sign', params: [challenge.Message, address] }) as string;
      await login({ Scheme: 'wallet', Address: address, Signature: signature, Nonce: challenge.Nonce });
      navigate('/');
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <h1>Log in</h1>
      <div className="login-card">
        <input className="input" type="email" placeholder="Email" value={email} onChange={e => setEmail(e.target.value)} />
        <input className="input" type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)} />
        <button className="btn" disabled={busy} onClick={emailLogin}>Log in with email</button>
        <div className="login-or">or</div>
        <button className="btn btn-secondary" disabled={busy} onClick={walletLogin}>Connect wallet</button>
        {err && <div className="forums-error">{err}</div>}
      </div>
      <p className="login-hint">Browse freely — logging in is only needed to post.</p>
    </div>
  );
}
