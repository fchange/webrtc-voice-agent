import { useEffect, useMemo, useRef, useState } from 'react';
import { createDefaultClientConfig, SignalingClient } from '../features/signaling/signaling-client';
import { type LocalSessionSource, VoiceSessionController } from '../features/webrtc/voice-session';

type DemoPage = 'call' | 'hotel';

type TimelineItem = {
  id: number;
  label: string;
};

type RoomType = {
  room_type_id: string;
  name: string;
  description: string;
  price_label: string;
  capacity: number;
  available_count: number;
};

type Reservation = {
  reservation_id: string;
  status: string;
  message: string;
  room_type_id: string;
  room_type_name?: string;
  guest_name: string;
  phone_number: string;
  available_count_after: number;
  created_at: string;
};

const hotelBaseURL = import.meta.env.VITE_BOT_HTTP_URL ?? 'http://localhost:8081';
const hotelRefreshIntervalMS = 5000;

function getPageFromHash(): DemoPage {
  return window.location.hash === '#/hotel' ? 'hotel' : 'call';
}

function navigateToPage(nextPage: DemoPage) {
  window.location.hash = nextPage === 'hotel' ? '#/hotel' : '#/call';
}

function formatSessionStatus(value: string): string {
  return (
    {
      idle: '空闲',
      creating_session: '创建中',
      negotiating: '协商中',
      connected: '已连接',
      interrupt_requested: '已请求打断',
      ending: '结束中',
      error: '错误',
    }[value] ?? value
  );
}

function formatWSState(value: string): string {
  return (
    {
      disconnected: '未连接',
      connected: '已连接',
      closed: '已关闭',
    }[value] ?? value
  );
}

function formatServiceStatus(value: string): string {
  return (
    {
      loading: '加载中',
      ready: '就绪',
      error: '错误',
      confirmed: '预订成功',
      sold_out: '已售罄',
      invalid_input: '输入无效',
      failed: '失败',
    }[value] ?? value
  );
}

export function App() {
  const nextTimelineID = useRef(2);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);

  const [page, setPage] = useState<DemoPage>(getPageFromHash);
  const [timeline, setTimeline] = useState<TimelineItem[]>([
    { id: 1, label: '酒店预订 Demo 已就绪，语音会话与库存服务现在可以一起联调。' },
  ]);
  const [sessionId, setSessionId] = useState<string>('');
  const [status, setStatus] = useState<string>('idle');
  const [wsState, setWSState] = useState<string>('disconnected');
  const [roomTypes, setRoomTypes] = useState<RoomType[]>([]);
  const [reservations, setReservations] = useState<Reservation[]>([]);
  const [hotelStatus, setHotelStatus] = useState<string>('loading');
  const [hotelError, setHotelError] = useState<string>('');

  const signaling = useMemo(() => new SignalingClient(createDefaultClientConfig()), []);
  const controller = useMemo(() => new VoiceSessionController(signaling), [signaling]);

  useEffect(() => {
    function syncPageFromHash() {
      setPage(getPageFromHash());
    }

    if (window.location.hash === '') {
      navigateToPage('call');
    }

    window.addEventListener('hashchange', syncPageFromHash);
    return () => {
      window.removeEventListener('hashchange', syncPageFromHash);
    };
  }, []);

  useEffect(() => {
    return () => {
      controller.close();
    };
  }, [controller]);

  useEffect(() => {
    let cancelled = false;

    async function refreshHotelData(quiet = false) {
      if (!quiet) {
        setHotelStatus('loading');
      }

      try {
        const [roomsResponse, reservationsResponse] = await Promise.all([
          fetch(`${hotelBaseURL}/internal/room-types`),
          fetch(`${hotelBaseURL}/internal/reservations?limit=8`),
        ]);

        if (!roomsResponse.ok || !reservationsResponse.ok) {
          throw new Error(`hotel service returned ${roomsResponse.status}/${reservationsResponse.status}`);
        }

        const roomsPayload = (await roomsResponse.json()) as { room_types: RoomType[] };
        const reservationsPayload = (await reservationsResponse.json()) as { reservations: Reservation[] };

        if (cancelled) {
          return;
        }

        setRoomTypes(roomsPayload.room_types);
        setReservations(reservationsPayload.reservations);
        setHotelStatus('ready');
        setHotelError('');
      } catch (error) {
        if (cancelled) {
          return;
        }

        setHotelStatus('error');
        setHotelError((error as Error).message);
      }
    }

    void refreshHotelData();
    const intervalID = window.setInterval(() => {
      void refreshHotelData(true);
    }, hotelRefreshIntervalMS);

    return () => {
      cancelled = true;
      window.clearInterval(intervalID);
    };
  }, []);

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
      await controller.connect(
        session,
        {
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
        },
        localSource,
      );
      appendTimeline(`会话创建成功: ${session.session_id}`);
    } catch (error) {
      if (localSource?.stop) {
        await localSource.stop();
      }
      setStatus('error');
      appendTimeline(`创建会话失败: ${(error as Error).message}`);
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
    <main className="shell shellWide">
      <section className="hero heroCompact">
        <p className="eyebrow">WebRTC HOTEL BOOKING DEMO</p>
        <div className="heroRow">
          <div>
            <h1>{page === 'call' ? '电话预订演示台' : '酒店预订状态面板'}</h1>
            <p className="lede">
              {page === 'call'
                ? '这个页面只保留打电话相关能力，帮助我们聚焦语音会话、远端音频与通话过程。'
                : '这个页面只展示酒店房型库存和最近预订结果，方便现场演示和业务状态观察。'}
            </p>
          </div>
          <nav className="pageNav" aria-label="页面切换">
            <button
              className={page === 'call' ? 'navButton navButtonActive' : 'navButton'}
              onClick={() => navigateToPage('call')}
            >
              打电话页
            </button>
            <button
              className={page === 'hotel' ? 'navButton navButtonActive' : 'navButton'}
              onClick={() => navigateToPage('hotel')}
            >
              酒店状态页
            </button>
          </nav>
        </div>
      </section>

      {page === 'call' ? (
        <>
          <section className="grid callGrid">
            <article className="card cardFeature">
              <div className="sectionHeading">
                <h2>通话控制</h2>
                <p>先建会话，再尝试中断、结束和远端音频验证。</p>
              </div>
              <dl className="meta">
                <div>
                  <dt>状态</dt>
                  <dd>{formatSessionStatus(status)}</dd>
                </div>
                <div>
                  <dt>会话 ID</dt>
                  <dd>{sessionId || '尚未创建'}</dd>
                </div>
                <div>
                  <dt>Signal 地址</dt>
                  <dd>{createDefaultClientConfig().httpBaseURL}</dd>
                </div>
                <div>
                  <dt>WebSocket</dt>
                  <dd>{formatWSState(wsState)}</dd>
                </div>
              </dl>
              <div className="actions">
                <button onClick={createSession}>创建会话</button>
                <button onClick={createDemoSession}>创建演示会话</button>
                <button onClick={interrupt} disabled={!sessionId}>
                  打断
                </button>
                <button onClick={endSession} disabled={!sessionId}>
                  结束
                </button>
              </div>
            </article>

            <article className="card">
              <div className="sectionHeading">
                <h2>通话说明</h2>
                <p>默认只展示关键动作，不让日志占满首屏。</p>
              </div>
              <ul className="stack">
                <li>创建会话后，页面会通过 Signal 与 Bot 完成协商。</li>
                <li>演示会话会把 `/demo.wav` 注入上行音轨，便于快速验证链路。</li>
                <li>真实日志默认折叠在下方，需要时再展开查看。</li>
              </ul>
            </article>
          </section>

          <section className="card">
            <div className="sectionHeading">
              <h2>远端音频</h2>
              <p>用于验证 Bot 下行音频与播放控制。</p>
            </div>
            <audio ref={remoteAudioRef} autoPlay playsInline controls />
          </section>

          <section className="card">
            <details className="logDetails">
              <summary className="logSummary">
                <span>查看通话日志</span>
                <small>默认折叠，点开后再看详细事件流</small>
              </summary>
              <ul className="timeline">
                {timeline.map((item) => (
                  <li key={item.id}>{item.label}</li>
                ))}
              </ul>
            </details>
          </section>
        </>
      ) : (
        <>
          <section className="grid">
            <article className="card">
              <div className="sectionHeading">
                <h2>服务状态</h2>
                <p>当前 bot 内部酒店服务的健康度与统计。</p>
              </div>
              <dl className="meta">
                <div>
                  <dt>状态</dt>
                  <dd>{formatServiceStatus(hotelStatus)}</dd>
                </div>
                <div>
                  <dt>基础地址</dt>
                  <dd>{hotelBaseURL}</dd>
                </div>
                <div>
                  <dt>房型数</dt>
                  <dd>{roomTypes.length}</dd>
                </div>
                <div>
                  <dt>预订数</dt>
                  <dd>{reservations.length}</dd>
                </div>
              </dl>
              {hotelError ? <p className="errorBanner">{hotelError}</p> : null}
            </article>

            <article className="card">
              <div className="sectionHeading">
                <h2>演示口径</h2>
                <p>这一页只负责看状态，不放通话动作。</p>
              </div>
              <ul className="stack">
                <li>房型库存每 5 秒自动刷新一次。</li>
                <li>最近预订倒序展示，方便观察最新变化。</li>
                <li>后续 AI 完成订房后，这一页会直接反映库存扣减与新预订记录。</li>
              </ul>
            </article>
          </section>

          <section className="card">
            <div className="sectionHeading">
              <h2>房型库存</h2>
              <p>当前各房型剩余可订数量。</p>
            </div>
            <div className="roomGrid">
              {roomTypes.map((room) => (
                <article className="roomCard" key={room.room_type_id}>
                  <div className="roomCardHeader">
                    <h3>{room.name}</h3>
                    <span className={room.available_count > 0 ? 'pill pillReady' : 'pill pillMuted'}>
                      {room.available_count > 0 ? `剩余 ${room.available_count} 间` : '已售罄'}
                    </span>
                  </div>
                  <p>{room.description}</p>
                  <dl className="roomMeta">
                    <div>
                      <dt>价格</dt>
                      <dd>{room.price_label}</dd>
                    </div>
                    <div>
                      <dt>入住人数</dt>
                      <dd>{room.capacity} 人</dd>
                    </div>
                  </dl>
                </article>
              ))}
            </div>
          </section>

          <section className="card">
            <div className="sectionHeading">
              <h2>最近预订</h2>
              <p>按最新记录倒序展示，包含售罄和失败结果。</p>
            </div>
            <ul className="reservationList">
              {reservations.length === 0 ? (
                <li className="reservationEmpty">暂时还没有预订记录。</li>
              ) : (
                reservations.map((reservation) => (
                  <li key={reservation.reservation_id} className="reservationItem">
                    <div className="reservationHeader">
                      <strong>{reservation.room_type_name || reservation.room_type_id}</strong>
                      <span className={`pill ${reservation.status === 'confirmed' ? 'pillReady' : 'pillMuted'}`}>
                        {formatServiceStatus(reservation.status)}
                      </span>
                    </div>
                    <p>{reservation.message}</p>
                    <small>
                      {reservation.guest_name || '未知入住人'} · {reservation.phone_number || '缺少手机号'} ·{' '}
                      {new Date(reservation.created_at).toLocaleString()}
                    </small>
                  </li>
                ))
              )}
            </ul>
          </section>
        </>
      )}
    </main>
  );
}
