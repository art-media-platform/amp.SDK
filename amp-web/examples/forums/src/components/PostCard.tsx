import { Link } from 'react-router-dom';
import type { AmpItemMeta } from '@art-media-platform/web';
import { authorUID, PostStatus } from '../forums-attrs';
import type { Post } from '../forums-attrs';
import { shortID } from '../util';

export function PostCard({ post, canModerate, onRemove }: {
  post: Post & AmpItemMeta;
  canModerate: boolean;
  onRemove: () => void;
}) {
  const removed = post.Status === PostStatus.Removed;
  const author = authorUID(post.Author);

  return (
    <li className="post-card">
      <div className="post-head">
        {author
          ? <Link to={`/u/${author}`} className="post-author">{shortID(author)}</Link>
          : <span className="post-author">unknown</span>}
        {post.EditedAt ? <span className="post-edited">edited</span> : null}
      </div>
      <div className="post-body">
        {removed
          ? <em className="post-removed">[removed]</em>
          // Dogfood: bodies render as authored HTML. Server-side sanitization is a
          // noted cross-cutting fast-follow (AD-app-forums.md §13).
          : <span dangerouslySetInnerHTML={{ __html: post.BodyHTML || '' }} />}
      </div>
      {canModerate && !removed && (
        <div className="post-actions">
          <button className="btn-link danger" onClick={onRemove}>Remove</button>
        </div>
      )}
    </li>
  );
}
