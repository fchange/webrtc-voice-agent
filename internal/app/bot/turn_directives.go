package bot

import (
	"context"
	"strings"
	"sync"
)

type endCallDirective struct {
	Reason  string
	Message string
}

type turnDirectives struct {
	mu      sync.Mutex
	endCall *endCallDirective
}

type turnDirectivesContextKey struct{}

func newTurnDirectives() *turnDirectives {
	return &turnDirectives{}
}

func withTurnDirectives(ctx context.Context, directives *turnDirectives) context.Context {
	if directives == nil {
		return ctx
	}
	return context.WithValue(ctx, turnDirectivesContextKey{}, directives)
}

func turnDirectivesFromContext(ctx context.Context) *turnDirectives {
	directives, _ := ctx.Value(turnDirectivesContextKey{}).(*turnDirectives)
	return directives
}

func (d *turnDirectives) RequestEndCall(reason string, message string) {
	if d == nil {
		return
	}

	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	if reason == "" {
		reason = "bot_farewell"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.endCall = &endCallDirective{
		Reason:  reason,
		Message: message,
	}
}

func (d *turnDirectives) EndCall() (endCallDirective, bool) {
	if d == nil {
		return endCallDirective{}, false
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.endCall == nil {
		return endCallDirective{}, false
	}
	return *d.endCall, true
}
