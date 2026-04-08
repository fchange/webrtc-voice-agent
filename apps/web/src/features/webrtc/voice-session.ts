import {
  DataChannelTypes,
  type SignalingEnvelope,
  type SignalingICECandidatePayload,
  type SignalingSDPPayload,
  type SignalingSession,
} from '../protocol/messages';
import { SignalingClient } from '../signaling/signaling-client';

export type LocalSessionSource = {
  stream: MediaStream;
  start?: () => Promise<void> | void;
  stop?: () => Promise<void> | void;
};

export class VoiceSessionController {
  private socket: WebSocket | null = null;
  private peerConnection: RTCPeerConnection | null = null;
  private localStream: MediaStream | null = null;
  private controlChannel: RTCDataChannel | null = null;
  private pendingRemoteICE: RTCIceCandidateInit[] = [];
  private localSourceStop: (() => Promise<void> | void) | null = null;

  constructor(private readonly signaling: SignalingClient) {}

  async bootstrap() {
    return this.signaling.createSession();
  }

  async prepareDemoSource(demoURL: string): Promise<LocalSessionSource> {
    const audioContext = new AudioContext();
    await audioContext.resume();

    const response = await fetch(demoURL);
    if (!response.ok) {
      await audioContext.close();
      throw new Error(`failed to load demo audio: ${response.status}`);
    }

    const audioBuffer = await audioContext.decodeAudioData(await response.arrayBuffer());
    const destination = audioContext.createMediaStreamDestination();
    const gainNode = audioContext.createGain();
    gainNode.gain.value = 1;
    gainNode.connect(destination);
    gainNode.connect(audioContext.destination);

    let sourceNode: AudioBufferSourceNode | null = null;

    return {
      stream: destination.stream,
      start: async () => {
        if (sourceNode !== null) {
          return;
        }
        sourceNode = audioContext.createBufferSource();
        sourceNode.buffer = audioBuffer;
        sourceNode.connect(gainNode);
        sourceNode.start();
      },
      stop: async () => {
        if (sourceNode !== null) {
          sourceNode.stop();
          sourceNode.disconnect();
          sourceNode = null;
        }
        await audioContext.close();
      },
    };
  }

  async connect(
    session: SignalingSession,
    handlers: {
      onEvent: (message: string) => void;
      onRemoteStream?: (stream: MediaStream) => void;
    },
    localSource?: LocalSessionSource,
  ) {
    const source = localSource ?? (await this.getMicrophoneSource());
    const stream = source.stream;
    this.localStream = stream;
    this.localSourceStop = source.stop ?? null;

    const peer = new RTCPeerConnection({
      iceServers: session.ice_urls.map((url) => ({ urls: url })),
    });
    this.peerConnection = peer;

    stream.getTracks().forEach((track) => peer.addTrack(track, stream));

    peer.ontrack = (event) => {
      const [remoteStream] = event.streams;
      if (remoteStream) {
        handlers.onRemoteStream?.(remoteStream);
        handlers.onEvent('Remote audio stream attached.');
      }
    };

    peer.onicecandidate = (event) => {
      if (!event.candidate || !this.socket) {
        return;
      }
      this.signaling.send(this.socket, {
        version: 'v1alpha1',
        type: 'session.ice_candidate',
        session_id: session.session_id,
        payload: {
          candidate: event.candidate.candidate,
          sdp_mid: event.candidate.sdpMid ?? undefined,
          sdp_mline_index: event.candidate.sdpMLineIndex ?? undefined,
        },
      });
    };

    peer.onconnectionstatechange = () => {
      handlers.onEvent(`Peer connection state: ${peer.connectionState}`);
    };

    const control = peer.createDataChannel('control');
    this.controlChannel = control;
    control.onopen = () => {
      handlers.onEvent('Control data channel open.');
    };
    control.onmessage = (event) => {
      handlers.onEvent(`DataChannel message: ${event.data}`);
    };

    this.socket = this.signaling.connect(
      session.session_id,
      {
        onOpen: async () => {
          handlers.onEvent('WebSocket signaling attached.');
          const offer = await peer.createOffer();
          await peer.setLocalDescription(offer);
          this.signaling.send(this.socket!, {
            version: 'v1alpha1',
            type: 'session.offer',
            session_id: session.session_id,
            payload: {
              sdp: offer.sdp,
              type: offer.type,
            },
          });
          handlers.onEvent('Local offer sent.');
          if (source.start) {
            await source.start();
            handlers.onEvent('Local source playback started.');
          }
        },
        onClose: () => {
          handlers.onEvent('WebSocket signaling closed.');
        },
        onMessage: async (message) => {
          await this.handleSignalMessage(message as SignalingEnvelope, handlers.onEvent);
        },
      },
      session.signaling_ws_url,
    );
  }

  interrupt(sessionID: string): string {
    if (this.controlChannel?.readyState === 'open') {
      this.controlChannel.send(
        JSON.stringify({
          version: 'v1alpha1',
          type: DataChannelTypes.TurnInterruptHint,
          session_id: sessionID,
          payload: { reason: 'user_barge_in' },
        }),
      );
      return `Sent ${DataChannelTypes.TurnInterruptHint} for ${sessionID}; bot must confirm before promoting to turn.interrupt.`;
    }

    return `Control channel not open yet for ${sessionID}.`;
  }

  end(sessionID: string): string {
    if (this.socket) {
      this.signaling.send(this.socket, {
        version: 'v1alpha1',
        type: 'session.close',
        session_id: sessionID,
        payload: { reason: 'user_requested' },
      });
    }
    this.close();
    return `End requested for ${sessionID}. Session resources released locally.`;
  }

  private async handleSignalMessage(message: SignalingEnvelope, onEvent: (message: string) => void) {
    if (!this.peerConnection) {
      return;
    }

    switch (message.type) {
      case 'session.attached':
        return;
      case 'session.answer': {
        const payload = message.payload as SignalingSDPPayload;
        await this.peerConnection.setRemoteDescription({
          type: 'answer',
          sdp: payload.sdp,
        });
        for (const candidate of this.pendingRemoteICE) {
          await this.peerConnection.addIceCandidate(candidate);
        }
        this.pendingRemoteICE = [];
        onEvent('Remote answer applied.');
        return;
      }
      case 'session.ice_candidate': {
        const payload = message.payload as SignalingICECandidatePayload;
        if (!payload.candidate) {
          return;
        }
        const candidate = {
          candidate: payload.candidate,
          sdpMid: payload.sdp_mid,
          sdpMLineIndex: payload.sdp_mline_index,
        };
        if (!this.peerConnection.remoteDescription) {
          this.pendingRemoteICE.push(candidate);
          onEvent('Remote ICE candidate queued.');
          return;
        }
        await this.peerConnection.addIceCandidate(candidate);
        onEvent('Remote ICE candidate applied.');
        return;
      }
      case 'session.error':
        onEvent(`Signal error: ${JSON.stringify(message.payload)}`);
        return;
      default:
        onEvent(`Signal event: ${message.type}`);
    }
  }

  close() {
    if (this.localSourceStop) {
      void this.localSourceStop();
      this.localSourceStop = null;
    }
    this.controlChannel?.close();
    this.controlChannel = null;
    this.peerConnection?.close();
    this.peerConnection = null;
    this.socket?.close();
    this.socket = null;
    this.localStream?.getTracks().forEach((track) => track.stop());
    this.localStream = null;
    this.pendingRemoteICE = [];
  }

  private async getMicrophoneSource(): Promise<LocalSessionSource> {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
      video: false,
    });

    return { stream };
  }
}
