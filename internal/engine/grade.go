package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// gradeRules reads the session's audit stream and lays the challenge's rule
// grading into the scorecard (Story 3.3). It is best-effort: if the audit stream
// can't be read, objective scoring still stands (rule grading is additive), so a
// transient read error never wipes a legitimate score.
func (e *Engine) gradeRules(ctx context.Context, sessionID, sessionDir string, ch *v1alpha1.Challenge, card *verify.Scorecard) {
	env, err := e.Provider.Environment(ctx, sessionID, sessionDir)
	if err != nil {
		return
	}
	events, err := readAllAuditEvents(ctx, env)
	if err != nil {
		return
	}

	violations := rules.Grade(ch.Rules, events)
	card.RuleViolations = violations
	for _, v := range violations {
		if v.Fail {
			card.Failed = true
		}
		card.RulePenalty += v.Points
	}
}

// readAllAuditEvents drains the session's audit stream from the beginning and
// decodes it into events. Verify is a quiescent, one-shot read (the player asked
// to be graded), so we read the whole session's history once; exempt engine and
// controller events are filtered later by attribution.
func readAllAuditEvents(ctx context.Context, env api.Environment) ([]rules.AuditEvent, error) {
	var all []rules.AuditEvent
	var cursor api.AuditCursor
	for {
		raw, next, err := env.AuditEvents(ctx, cursor)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 || next == cursor {
			break
		}
		all = append(all, decodeAuditLines(raw)...)
		cursor = next
	}
	return all, nil
}

func decodeAuditLines(raw []byte) []rules.AuditEvent {
	var out []rules.AuditEvent
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev rules.AuditEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // tolerate a non-event or partial line
		}
		out = append(out, ev)
	}
	return out
}
