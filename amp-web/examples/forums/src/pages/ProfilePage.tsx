import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useAmpQuery, useAmpAuth } from '@art-media-platform/web';
import { PROFILES, ATTR_PROFILE } from '../forums-attrs';
import type { Profile } from '../forums-attrs';
import { useForumsApi } from '../hooks/useForumsApi';
import { shortID } from '../util';

export function ProfilePage() {
  const { memberID = '' } = useParams();
  const { member } = useAmpAuth();
  const isSelf = member?.ID === memberID;
  const { data, loading } = useAmpQuery<Profile>(PROFILES, ATTR_PROFILE, { itemID: memberID });
  const profile = data[0];

  return (
    <div className="profile">
      <h1>{profile?.DisplayName || shortID(memberID)}</h1>
      {loading && <div className="forums-empty">Loading…</div>}
      {profile?.SignatureHTML && (
        <div className="profile-sig" dangerouslySetInnerHTML={{ __html: profile.SignatureHTML }} />
      )}
      {isSelf && <ProfileEditor initial={profile} />}
    </div>
  );
}

function ProfileEditor({ initial }: { initial?: Profile }) {
  const { saveProfile } = useForumsApi();
  const [name, setName] = useState(initial?.DisplayName ?? '');
  const [sig, setSig] = useState(initial?.SignatureHTML ?? '');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    setName(initial?.DisplayName ?? '');
    setSig(initial?.SignatureHTML ?? '');
  }, [initial]);

  async function save() {
    setBusy(true);
    setMsg(null);
    try {
      await saveProfile(name, sig);
      setMsg('Saved.');
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="profile-editor">
      <h2>Edit profile</h2>
      <input className="input" placeholder="Display name" value={name} onChange={e => setName(e.target.value)} />
      <textarea
        className="composer-input"
        placeholder="Signature (HTML)"
        rows={3}
        value={sig}
        onChange={e => setSig(e.target.value)}
      />
      {msg && <div className="forums-note">{msg}</div>}
      <button className="btn" disabled={busy} onClick={save}>{busy ? 'Saving…' : 'Save'}</button>
    </div>
  );
}
