import { Link, useNavigate } from 'react-router-dom';
import { useAmpAuth } from '@art-media-platform/web';

export function Nav() {
  const { member, isAuthenticated, logout } = useAmpAuth();
  const navigate = useNavigate();

  return (
    <header className="forums-nav">
      <Link to="/" className="forums-brand">AMP&nbsp;Forums</Link>
      <nav className="forums-nav-links">
        <Link to="/pm">Messages</Link>
        {isAuthenticated && member ? (
          <>
            <Link to={`/u/${member.ID}`}>{member.DisplayName || 'Profile'}</Link>
            <button className="btn-link" onClick={() => logout()}>Log&nbsp;out</button>
          </>
        ) : (
          <button className="btn" onClick={() => navigate('/login')}>Log&nbsp;in</button>
        )}
      </nav>
    </header>
  );
}
