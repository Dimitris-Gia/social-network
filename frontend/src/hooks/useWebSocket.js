// Owner: B — reads WebSocketContext
// Usage in any component: const { send, subscribe, status } = useWebSocket();
//   send(type, data)  -> true if sent, false if the connection is not open
//   status            -> "connecting" | "open" | "reconnecting" | "closed" | "idle"
// To react to server frames, prefer useSocketEvent (it cleans up for you).
import { useContext } from "react";
import { WebSocketContext } from "../context/WebSocketContext.jsx";

export default function useWebSocket() {
  const ctx = useContext(WebSocketContext);
  if (!ctx) {
    throw new Error("useWebSocket must be used inside <WebSocketProvider>");
  }
  return ctx;
}