// useLeaderboard subscribes to the live board for a run over WebSocket and keeps
// a React state copy. It seeds from REST so the table paints immediately, then
// the socket takes over for live updates. Auto-reconnects with backoff if the
// connection drops (e.g. a leaderboard-service pod rolls).

import { useEffect, useRef, useState } from 'react';
import { fetchLeaderboard, wsURL } from '../api/client';
import type { BoardMessage, LeaderboardEntry } from '../types';

export type ConnState = 'connecting' | 'live' | 'reconnecting' | 'idle';

export function useLeaderboard(runID: string | null) {
  const [entries, setEntries] = useState<LeaderboardEntry[]>([]);
  const [conn, setConn] = useState<ConnState>('idle');
  const socketRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);
  const closedRef = useRef(false);

  useEffect(() => {
    if (!runID) {
      setEntries([]);
      setConn('idle');
      return;
    }
    closedRef.current = false;

    // Seed from REST.
    fetchLeaderboard(runID)
      .then(setEntries)
      .catch(() => {/* socket will fill in */});

    let timer: number | undefined;

    const connect = () => {
      setConn(retryRef.current === 0 ? 'connecting' : 'reconnecting');
      const ws = new WebSocket(wsURL(runID));
      socketRef.current = ws;

      ws.onopen = () => {
        retryRef.current = 0;
        setConn('live');
      };
      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data as string) as BoardMessage;
          if (msg.type === 'leaderboard' && msg.run_id === runID) {
            setEntries(msg.entries ?? []);
          }
        } catch {
          // ignore malformed frame
        }
      };
      ws.onclose = () => {
        if (closedRef.current) return;
        const backoff = Math.min(1000 * 2 ** retryRef.current, 10000);
        retryRef.current += 1;
        setConn('reconnecting');
        timer = window.setTimeout(connect, backoff);
      };
      ws.onerror = () => ws.close();
    };

    connect();

    return () => {
      closedRef.current = true;
      if (timer) window.clearTimeout(timer);
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [runID]);

  return { entries, conn };
}
