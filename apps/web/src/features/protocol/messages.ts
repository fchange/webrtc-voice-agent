export type SignalingSession = {
  session_id: string;
  signaling_ws_url: string;
  ice_urls: string[];
  token_hint?: string;
};

export type SignalingEnvelope = {
  version: 'v1alpha1';
  type: string;
  session_id?: string;
  trace_id?: string;
  payload?: unknown;
};

export type SignalingSDPPayload = {
  sdp: string;
  type: 'offer' | 'answer';
};

export type SignalingICECandidatePayload = {
  candidate: string;
  sdp_mid?: string;
  sdp_mline_index?: number;
};

export type DataChannelMessage = {
  version: 'v1alpha1';
  type: string;
  session_id: string;
  turn_id?: number;
  request_id?: string;
  payload?: Record<string, unknown>;
};

export const DataChannelTypes = {
  SessionReady: 'session.ready',
  SessionEnding: 'session.ending',
  TurnStarted: 'turn.started',
  TurnInterruptHint: 'turn.interrupt_hint',
  TurnInterrupt: 'turn.interrupt',
  TurnCancelled: 'turn.cancelled',
  TurnEndOfUtterance: 'turn.end_of_utterance',
  TurnCompleted: 'turn.completed',
  BotSpeakingStarted: 'bot.speaking.started',
  BotSpeakingStopped: 'bot.speaking.stopped',
  VADStarted: 'vad.started',
  VADStopped: 'vad.stopped',
  ASRPartial: 'asr.partial',
  ASRFinal: 'asr.final',
  Error: 'error',
} as const;
