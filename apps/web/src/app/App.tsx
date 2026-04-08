import { useEffect, useMemo, useRef, useState } from 'react';
import { createDefaultClientConfig, SignalingClient } from '../features/signaling/signaling-client';
import { type LocalSessionSource, VoiceSessionController } from '../features/webrtc/voice-session';

type TimelineItem = {
  id: number;
  label: string;
};

export function App() {
  const nextTimelineID = useRef(2);
  const [timeline, setTimeline] = useState<TimelineItem[]>([
    { id: 1, label: 'Phase 0 scaffold ready. WebRTC signaling and bot peer are the next milestones.' },
  ]);
  const [sessionId, setSessionId] = useState<string>('');
  const [status, setStatus] = useState<string>('idle');
  const [wsState, setWSState] = useState<string>('disconnected');
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);

  const signaling = useMemo(() => new SignalingClient(createDefaultClientConfig()), []);
  const controller = useMemo(() => new VoiceSessionController(signaling), [signaling]);

  useEffect(() => {
    return () => {
      controller.close();
    };
  }, [controller]);

  function appendTimeline(label: string) {
    const id = nextTimelineID.current;
    nextTimelineID.current += 1;
    setTimeline((items) => [{ id, label }, ...items]);
  }

  async function startSession(localSource?: LocalSessionSource) {
    setStatus('creating_session');

    try {
      const session = await signaling.createSession();
      setSessionId(session.session_id);
      setStatus('negotiating');
      await controller.connect(session, {
        onEvent: (message) => {
          if (message.includes('WebSocket signaling attached')) {
            setWSState('connected');
          }
          if (message.includes('WebSocket signaling closed')) {
            setWSState('closed');
          }
          if (message.includes('Remote answer applied')) {
            setStatus('connected');
          }
          appendTimeline(message);
        },
        onRemoteStream: (stream) => {
          if (remoteAudioRef.current) {
            remoteAudioRef.current.srcObject = stream;
          }
        },
      }, localSource);
      appendTimeline(`Session created: ${session.session_id}`);
    } catch (error) {
      if (localSource?.stop) {
        await localSource.stop();
      }
      setStatus('error');
      appendTimeline(`Create session failed: ${(error as Error).message}`);
    }
  }

  async function createSession() {
    await startSession();
  }

  async function createDemoSession() {
    const demoSource = await controller.prepareDemoSource('/demo.wav');
    await startSession(demoSource);
  }

  function interrupt() {
    const next = controller.interrupt(sessionId || 'pending_session');
    setStatus('interrupt_requested');
    appendTimeline(next);
  }

  function endSession() {
    const next = controller.end(sessionId || 'pending_session');
    setStatus('ending');
    setWSState('closed');
    appendTimeline(next);
  }

  return (
    <main className="shell">
      <section className="hero">
        <p className="eyebrow">WebRTC Voice Bot</p>
        <h1>Open source real-time voice bot groundwork</h1>
        <p className="lede">
          This client is intentionally small. It exists to host microphone, signaling,
          PeerConnection, DataChannel, and session UI concerns without coupling them together too
          early.
        </p>
      </section>

      <section className="grid">
        <article className="card">
          <h2>Session</h2>
          <dl className="meta">
            <div>
              <dt>Status</dt>
              <dd>{status}</dd>
            </div>
            <div>
              <dt>Session ID</dt>
              <dd>{sessionId || 'not created'}</dd>
            </div>
            <div>
              <dt>Signal URL</dt>
              <dd>{createDefaultClientConfig().httpBaseURL}</dd>
            </div>
            <div>
              <dt>WS State</dt>
              <dd>{wsState}</dd>
            </div>
          </dl>

          <div className="actions">
            <button onClick={createSession}>Create Session</button>
            <button onClick={createDemoSession}>Create Demo Session</button>
            <button onClick={interrupt} disabled={!sessionId}>
              Interrupt
            </button>
            <button onClick={endSession} disabled={!sessionId}>
              End
            </button>
          </div>
        </article>

        <article className="card">
          <h2>Module Boundary</h2>
          <ul className="stack">
            <li>`signaling-client.ts` owns session bootstrap and protocol transport.</li>
            <li>`voice-session.ts` owns media capture, PeerConnection, signaling apply, and interrupt/end semantics.</li>
            <li>Demo mode can inject `/demo.wav` into the upstream WebRTC track without using the microphone.</li>
          </ul>
        </article>
      </section>

      <section className="card">
        <h2>Remote Audio</h2>
        <audio ref={remoteAudioRef} autoPlay playsInline controls />
      </section>

      <section className="card">
        <h2>Timeline</h2>
        <ul className="timeline">
          {timeline.map((item) => (
            <li key={item.id}>{item.label}</li>
          ))}
        </ul>
      </section>
    </main>
  );
}
