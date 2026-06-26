import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useForumsApi } from '../hooks/useForumsApi';
import { RequireAuth } from '../components/RequireAuth';

export function NewTopicPage() {
  return (
    <RequireAuth>
      <NewTopicForm />
    </RequireAuth>
  );
}

function NewTopicForm() {
  const { createTopic } = useForumsApi();
  const navigate = useNavigate();
  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function submit() {
    if (!title.trim() || !body.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      await createTopic(title, body);
      navigate('/'); // the server mints the node; the board re-queries on mount
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  }

  return (
    <div className="new-topic">
      <h1>New Topic</h1>
      <input
        className="input"
        placeholder="Title"
        value={title}
        onChange={e => setTitle(e.target.value)}
      />
      <textarea
        className="composer-input"
        placeholder="Write your first post…"
        rows={8}
        value={body}
        onChange={e => setBody(e.target.value)}
      />
      {err && <div className="forums-error">{err}</div>}
      <button className="btn" disabled={busy || !title.trim() || !body.trim()} onClick={submit}>
        {busy ? 'Posting…' : 'Create Topic'}
      </button>
    </div>
  );
}
