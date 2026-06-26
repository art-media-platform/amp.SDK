import { Link } from 'react-router-dom';
import { useAmpQuery, useAmpAuth } from '@art-media-platform/web';
import { BOARD, ATTR_TOPIC } from '../forums-attrs';
import type { Topic } from '../forums-attrs';

export function BoardPage() {
  const { isAuthenticated } = useAmpAuth();
  const { data: topics, loading, error } = useAmpQuery<Topic>(BOARD, ATTR_TOPIC);

  return (
    <div className="board">
      <div className="board-head">
        <h1>Discussions</h1>
        {isAuthenticated
          ? <Link className="btn" to="/new">New Topic</Link>
          : <Link className="btn" to="/login">Log in to post</Link>}
      </div>

      {error && <div className="forums-error">{error.message}</div>}
      {loading && topics.length === 0 && <div className="forums-empty">Loading…</div>}
      {!loading && topics.length === 0 && (
        <div className="forums-empty">No topics yet. Be the first to post.</div>
      )}

      <ul className="topic-list">
        {topics.map(t => (
          <li key={t._ItemID} className="topic-row">
            <Link to={`/t/${t._ItemID}`} className="topic-title">
              {t.Pinned ? '📌 ' : ''}{t.Title || '(untitled)'}
            </Link>
            <div className="topic-meta">
              <span>{t.ReplyCount ?? 0} replies</span>
              <span>{t.ViewCount ?? 0} views</span>
              {t.Locked ? <span className="tag-locked">locked</span> : null}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
