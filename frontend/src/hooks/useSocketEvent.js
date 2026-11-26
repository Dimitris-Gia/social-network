// Owner: B — run a function for every server frame of one type
//
//   useSocketEvent("private_message", (msg) => setMessages((m) => [...m, msg]));
//
// It subscribes when the component mounts and unsubscribes when it unmounts.
// `handler` can be a fresh function on every render: the latest one is always
// the one called, without re-subscribing.
//
// The synthetic type "ws:reconnected" fires when the connection comes back
// after a drop. Frames sent while offline are lost, so re-fetch your data then.
import { useEffect, useRef } from "react";
import useWebSocket from "./useWebSocket.js";
 
export default function useSocketEvent(type, handler) {
  const { subscribe } = useWebSocket();
  const latest = useRef(handler);
 
  useEffect(() => {
    latest.current = handler;
  });
 
  useEffect(
    () => subscribe(type, (data) => latest.current(data)),
    [subscribe, type]
  );
}
 