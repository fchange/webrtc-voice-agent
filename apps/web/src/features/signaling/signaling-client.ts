import type { SignalingEnvelope, SignalingSession } from '../protocol/messages';

export type SignalingClientConfig = {
  httpBaseURL: string;
  wsURL: string;
  token: string;
};

export function createDefaultClientConfig(): SignalingClientConfig {
  return {
    httpBaseURL: import.meta.env.VITE_SIGNAL_HTTP_URL ?? 'http://localhost:8080',
    wsURL: import.meta.env.VITE_SIGNAL_WS_URL ?? 'ws://localhost:8080/ws',
    token: import.meta.env.VITE_DEV_TOKEN ?? 'dev-token',
  };
}

export class SignalingClient {
  constructor(private readonly config: SignalingClientConfig) {}

  async createSession(): Promise<SignalingSession> {
    const response = await fetch(`${this.config.httpBaseURL}/v1/sessions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.config.token}`,
      },
      body: JSON.stringify({ client_id: 'web-dev-client' }),
    });

    if (!response.ok) {
      throw new Error(`signal returned ${response.status}`);
    }

    return response.json() as Promise<SignalingSession>;
  }

  connect(
    sessionID: string,
    handlers: {
      onOpen?: () => void;
      onMessage?: (message: unknown) => void;
      onClose?: () => void;
    },
    wsURL = this.config.wsURL,
  ): WebSocket {
    const url = new URL(wsURL);
    url.searchParams.set('session_id', sessionID);
    url.searchParams.set('role', 'client');
    url.searchParams.set('access_token', this.config.token);

    const socket = new WebSocket(url.toString());
    socket.addEventListener('open', () => handlers.onOpen?.());
    socket.addEventListener('close', () => handlers.onClose?.());
    socket.addEventListener('message', (event) => {
      handlers.onMessage?.(JSON.parse(event.data) as SignalingEnvelope);
    });
    return socket;
  }

  send(socket: WebSocket, message: unknown) {
    socket.send(JSON.stringify(message));
  }
}
