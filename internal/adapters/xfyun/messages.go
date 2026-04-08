package xfyun

type requestEnvelope struct {
	Header    requestHeader     `json:"header"`
	Parameter *requestParameter `json:"parameter,omitempty"`
	Payload   requestPayload    `json:"payload"`
}

type requestHeader struct {
	AppID  string `json:"app_id"`
	Status int    `json:"status"`
}

type requestParameter struct {
	IAT requestIAT `json:"iat"`
}

type requestIAT struct {
	Domain   string        `json:"domain"`
	Language string        `json:"language"`
	Accent   string        `json:"accent"`
	EOSMS    int           `json:"eos,omitempty"`
	DWA      string        `json:"dwa,omitempty"`
	Result   requestResult `json:"result"`
}

type requestResult struct {
	Encoding string `json:"encoding"`
	Compress string `json:"compress"`
	Format   string `json:"format"`
}

type requestPayload struct {
	Audio requestAudio `json:"audio"`
}

type requestAudio struct {
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	BitDepth   int    `json:"bit_depth"`
	Seq        int    `json:"seq"`
	Status     int    `json:"status"`
	Audio      string `json:"audio"`
}

type responseEnvelope struct {
	Header  responseHeader  `json:"header"`
	Payload responsePayload `json:"payload"`
}

type responseHeader struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Sid     string `json:"sid"`
	Status  int    `json:"status"`
}

type responsePayload struct {
	Result responseResult `json:"result"`
}

type responseResult struct {
	Compress string `json:"compress"`
	Encoding string `json:"encoding"`
	Format   string `json:"format"`
	Seq      int    `json:"seq"`
	Status   int    `json:"status"`
	Text     string `json:"text"`
	PGS      string `json:"pgs,omitempty"`
	RG       []int  `json:"rg,omitempty"`
}

type decodedResult struct {
	SN int               `json:"sn"`
	LS bool              `json:"ls"`
	WS []decodedWordSlot `json:"ws"`
}

type decodedWordSlot struct {
	CW []decodedCandidateWord `json:"cw"`
}

type decodedCandidateWord struct {
	W string `json:"w"`
}
