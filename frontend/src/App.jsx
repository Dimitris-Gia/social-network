// Owner: B — react-router routes, layout with Navbar
import { Routes, Route } from "react-router-dom";
import Navbar from "./components/layout/Navbar.jsx";
import LoginPage from "./pages/LoginPage.jsx";
import RegisterPage from "./pages/RegisterPage.jsx";
import FeedPage from "./pages/FeedPage.jsx";
import ProfilePage from "./pages/ProfilePage.jsx";
import UsersPage from "./pages/UsersPage.jsx";
import ChatPage from "./pages/ChatPage.jsx";
import GroupsPage from "./pages/GroupsPage.jsx";
import GroupPage from "./pages/GroupPage.jsx";
import NotFoundPage from "./pages/NotFoundPage.jsx";

// TODO (A): wrap private routes in <ProtectedRoute>
export default function App() {
  return (
    <>
      <Navbar />
      <main>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/" element={<FeedPage />} />
          <Route path="/users" element={<UsersPage />} />
          <Route path="/users/:id" element={<ProfilePage />} />
          <Route path="/chat" element={<ChatPage />} />
          <Route path="/groups" element={<GroupsPage />} />
          <Route path="/groups/:id" element={<GroupPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
    </>
  );
}
