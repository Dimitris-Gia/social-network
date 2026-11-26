// Owner: B — mounts <App/> inside the providers
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App.jsx";
import { AuthProvider } from "./context/AuthContext.jsx";
import { WebSocketProvider } from "./context/WebSocketContext.jsx";
import "./index.css";

// AuthProvider sits inside BrowserRouter (so auth code can navigate) and
// outside App (so every page and the Navbar can call useAuth()).
// WebSocketProvider sits inside AuthProvider: it needs to know who is logged in.
ReactDOM.createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <WebSocketProvider>
          <App />
        </WebSocketProvider>
      </AuthProvider>
    </BrowserRouter>
  </React.StrictMode>
);