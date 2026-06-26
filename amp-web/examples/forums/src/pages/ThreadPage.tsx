import { useEffect, useRef } from 'react';
import { useParams } from 'react-router-dom';
import { useAmpQuery, useAmpAuth } from '@art-media-platform/web';
import { ATTR_POST, PostStatus, authorUID } from '../forums-attrs';
import type { Post } from '../forums-attrs';
import { useForumsApi } from '../hooks/useForumsApi';
import { Composer } from '../components/Composer';
import { PostCard } from '../components/PostCard';

export function ThreadPage() {
  const { topicID = '' } = useParams();
  const { member, isAuthenticated } = useAmpAuth();
  const { data: posts, loading, error, refetch } = useAmpQuery<Post>(topicID, ATTR_POST);
  const { reply, moderate, recordView, markRead } = useForumsApi();

  const viewed = useRef(false);
  const lastMarked = useRef('');

  // One view per open.
  useEffect(() => {
    if (topicID && !viewed.current) {
      viewed.current = true;
      recordView(topicID).catch(() => {});
    }
  }, [topicID, recordView]);

  // Mark read at the latest post (only when it advances).
  useEffect(() => {
    if (!isAuthenticated || posts.length === 0) return;
    const last = posts[posts.length - 1];
    if (last._EditID && last._EditID !== lastMarked.current) {
      lastMarked.current = last._EditID;
      markRead(topicID, last._EditID).catch(() => {});
    }
  }, [isAuthenticated, posts, topicID, markRead]);

  async function onReply(body: string) {
    await reply(topicID, body);
    await refetch();
  }
  async function onRemove(postID: string) {
    await moderate(topicID, postID, PostStatus.Removed);
    await refetch();
  }

  return (
    <div className="thread">
      {error && <div className="forums-error">{error.message}</div>}
      {loading && posts.length === 0 && <div className="forums-empty">Loading…</div>}

      <ol className="post-list">
        {posts.map(p => (
          <PostCard
            key={p._ItemID}
            post={p}
            canModerate={isAuthenticated && member != null && authorUID(p.Author) === member.ID}
            onRemove={() => onRemove(p._ItemID)}
          />
        ))}
      </ol>

      {isAuthenticated
        ? <Composer placeholder="Write a reply…" submitLabel="Reply" onSubmit={onReply} />
        : <div className="forums-empty">Log in to reply.</div>}
    </div>
  );
}
