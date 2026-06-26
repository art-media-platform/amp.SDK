import { Routes, Route } from 'react-router-dom';
import { Nav } from './components/Nav';
import { BoardPage } from './pages/BoardPage';
import { ThreadPage } from './pages/ThreadPage';
import { NewTopicPage } from './pages/NewTopicPage';
import { ProfilePage } from './pages/ProfilePage';
import { LoginPage } from './pages/LoginPage';
import { PmPage } from './pages/PmPage';

export function App() {
  return (
    <div className="forums-app">
      <Nav />
      <main className="forums-main">
        <Routes>
          <Route path="/" element={<BoardPage />} />
          <Route path="/t/:topicID" element={<ThreadPage />} />
          <Route path="/new" element={<NewTopicPage />} />
          <Route path="/u/:memberID" element={<ProfilePage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/pm" element={<PmPage />} />
        </Routes>
      </main>
    </div>
  );
}
