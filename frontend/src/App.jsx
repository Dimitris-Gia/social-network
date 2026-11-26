// Owner: B — react-router routes, layout with Navbar
// Route protection wired by A (week 2, Mon).
import { Routes, Route } from "react-router-dom";
import Navbar from "./components/layout/Navbar.jsx";
import ProtectedRoute from "./components/layout/ProtectedRoute.jsx";
import LoginPage from "./pages/LoginPage.jsx";
import RegisterPage from "./pages/RegisterPage.jsx";
import FeedPage from "./pages/FeedPage.jsx";
import ProfilePage from "./pages/ProfilePage.jsx";
import UsersPage from "./pages/UsersPage.jsx";
import ChatPage from "./pages/ChatPage.jsx";
import GroupsPage from "./pages/GroupsPage.jsx";
import GroupPage from "./pages/GroupPage.jsx";
import NotFoundPage from "./pages/NotFoundPage.jsx";

export default function App() {
  return (
    <>
      <Navbar />
      <main>
        <Routes>
          {/* Public: LoginPage and RegisterPage redirect to "/" themselves
              if you're already logged in. */}
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />

          {/* Everything else needs a session. A route without a path is a
              "layout route": ProtectedRoute runs first and renders the
              matching child through <Outlet /> only if you're logged in. */}
          <Route element={<ProtectedRoute />}>
            <Route path="/" element={<FeedPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/users/:id" element={<ProfilePage />} />
            <Route path="/chat" element={<ChatPage />} />
            <Route path="/groups" element={<GroupsPage />} />
            <Route path="/groups/:id" element={<GroupPage />} />
          </Route>

          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
    </>
  );
}