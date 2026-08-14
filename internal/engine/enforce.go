package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
)

// minVAPMinor is the first Kubernetes minor where ValidatingAdmissionPolicy is
// GA (1.30). Below it, enforcement degrades to audit-grading with a warning.
const minVAPMinor = 30

// applyEnforcement generates and applies the ValidatingAdmissionPolicies for a
// challenge's `enforce: true` rules using the ENGINE identity (so the engine —
// exempt in the policy — is never blocked). It returns the names of the applied
// policies for the tamper snapshot. On a cluster too old for VAP it applies
// nothing and warns; the rules still grade from the audit log (Story 3.3).
func (e *Engine) applyEnforcement(ctx context.Context, c *kube.Client, ch *v1alpha1.Challenge, prog Progressf) ([]string, error) {
	objs := rules.EnforcementPolicies(ch)
	if len(objs) == 0 {
		return nil, nil
	}
	if minor, ok := serverMinorAtLeast(c, minVAPMinor); !ok {
		prog("enforce: Kubernetes 1.%d has no ValidatingAdmissionPolicy (GA in 1.%d); enforced rules will be graded from the audit log only", minor, minVAPMinor)
		return nil, nil
	}

	var applied []string
	for i := range objs {
		o := &objs[i]
		ri, err := c.ResourceFor(o.GetAPIVersion(), o.GetKind(), "")
		if err != nil {
			return applied, fmt.Errorf("enforce: resolve %s: %w", o.GetKind(), err)
		}
		if _, err := ri.Create(ctx, o, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return applied, fmt.Errorf("enforce: create %s %q: %w", o.GetKind(), o.GetName(), err)
		}
		if o.GetKind() == "ValidatingAdmissionPolicy" {
			applied = append(applied, o.GetName())
		}
	}
	prog("enforce: %d live admission guardrail(s) active", len(applied))
	return applied, nil
}

// serverMinorAtLeast reports whether the cluster's minor version is >= want,
// returning the detected minor for messaging.
func serverMinorAtLeast(c *kube.Client, want int) (int, bool) {
	info, err := c.Typed.Discovery().ServerVersion()
	if err != nil {
		// Unknown version — assume a modern cluster (kubedrill provisions 1.30+).
		return want, true
	}
	minor := digits(info.Minor)
	n, err := strconv.Atoi(minor)
	if err != nil {
		return want, true
	}
	return n, n >= want
}

// digits strips non-digit suffixes kubeadm sometimes adds (e.g. "30+").
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}

// checkEnforcementTamper compares the recorded enforced-policy snapshot against
// the live cluster and charges a tamper (fail) violation for any policy the
// player removed — deleting a live guardrail is an integrity breach (AD-5).
func (e *Engine) checkEnforcementTamper(ctx context.Context, c *kube.Client, enforced []string, card *verify.Scorecard) {
	if len(enforced) == 0 {
		return
	}
	ri, err := c.ResourceFor("admissionregistration.k8s.io/v1", "ValidatingAdmissionPolicy", "")
	if err != nil {
		return // can't check — leave objective score intact
	}
	for _, name := range enforced {
		if _, err := ri.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			card.Failed = true
			card.RuleViolations = append(card.RuleViolations, rules.Violation{
				RuleID:  "integrity",
				Type:    "tamper",
				Fail:    true,
				Message: fmt.Sprintf("removed the live enforcement policy %q — integrity breach", name),
			})
		}
	}
}
