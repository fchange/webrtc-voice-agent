package errors

type Code string

const (
	CodeUnauthorized   Code = "unauthorized"
	CodeInvalidMessage Code = "invalid_message"
	CodeInvalidState   Code = "invalid_state"
	CodeSessionMissing Code = "session_not_found"
	CodeConflict       Code = "conflict"
	CodeInternal       Code = "internal"
)
