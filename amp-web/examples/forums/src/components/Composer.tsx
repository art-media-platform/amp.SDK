import { useState } from 'react';

export function Composer({ placeholder, submitLabel, onSubmit }: {
  placeholder: string;
  submitLabel: string;
  onSubmit: (body: string) => Promise<void>;
}) {
  const [body, setBody] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function submit() {
    if (!body.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      await onSubmit(body);
      setBody('');
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="composer">
      <textarea
        className="composer-input"
        placeholder={placeholder}
        value={body}
        rows={4}
        onChange={e => setBody(e.target.value)}
      />
      {err && <div className="forums-error">{err}</div>}
      <button className="btn" disabled={busy || !body.trim()} onClick={submit}>
        {busy ? '…' : submitLabel}
      </button>
    </div>
  );
}
