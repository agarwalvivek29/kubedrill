package verify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"io"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

// defaultProbeNamespace is the vantage a probe runs from unless the author
// chooses another (AD-10).
const defaultProbeNamespace = "kubedrill-system"

// evalProbe runs a script probe as an in-cluster Job at its declared namespace
// and maps the exit code to an Outcome (LLD §10):
//   exit 0      -> Pass
//   exit 1      -> Fail (last stdout line is the user-facing reason)
//   exit 2+ / timeout / infra error -> Errored (never "you failed")
func (e *Evaluator) evalProbe(ctx context.Context, p *v1alpha1.Probe) CheckResult {
	script, err := os.ReadFile(filepath.Join(e.Dir, p.Script))
	if err != nil {
		return CheckResult{Errored, fmt.Sprintf("read probe script %q: %v", p.Script, err)}
	}
	ns := p.Namespace
	if ns == "" {
		ns = defaultProbeNamespace
	}
	timeout := 30 * time.Second
	if p.Timeout != "" {
		if d, perr := time.ParseDuration(p.Timeout); perr == nil {
			timeout = d
		}
	}

	kc := e.Client.Typed
	if err := ensureNamespace(ctx, kc, ns); err != nil {
		return CheckResult{Errored, err.Error()}
	}

	name := "kd-probe-" + randSuffix()
	labels := map[string]string{probeLabel: "true"}

	// Script delivered via ConfigMap, mounted into the Job pod.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Data:       map[string]string{"probe.sh": string(script)},
	}
	if _, err := kc.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		return CheckResult{Errored, fmt.Sprintf("create probe configmap: %v", err)}
	}
	defer func() { _ = kc.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{}) }()

	job := probeJob(name, ns, p.Image, labels, timeout)
	if _, err := kc.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return CheckResult{Errored, fmt.Sprintf("create probe job: %v", err)}
	}
	defer func() {
		bg := metav1.DeletePropagationBackground
		_ = kc.BatchV1().Jobs(ns).Delete(context.Background(), name, metav1.DeleteOptions{PropagationPolicy: &bg})
	}()

	return e.awaitProbe(ctx, ns, name, timeout)
}

// awaitProbe polls for the probe pod to terminate and interprets its exit code.
func (e *Evaluator) awaitProbe(ctx context.Context, ns, name string, timeout time.Duration) CheckResult {
	kc := e.Client.Typed
	deadline := time.Now().Add(timeout + 15*time.Second) // grace for scheduling/pull
	for {
		pods, err := kc.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
		if err == nil {
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.State.Terminated != nil {
						code := cs.State.Terminated.ExitCode
						reason := lastLine(podLogs(ctx, e, ns, pod.Name))
						return interpretExit(code, reason)
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return CheckResult{Errored, "probe did not finish before timeout"}
		}
		select {
		case <-ctx.Done():
			return CheckResult{Errored, "probe cancelled"}
		case <-time.After(2 * time.Second):
		}
	}
}

// interpretExit maps a probe container exit code to an Outcome.
func interpretExit(code int32, reason string) CheckResult {
	switch {
	case code == 0:
		return CheckResult{Pass, ""}
	case code == 1:
		if reason == "" {
			reason = "probe reported failure"
		}
		return CheckResult{Fail, reason}
	default:
		if reason == "" {
			reason = fmt.Sprintf("probe errored (exit %d)", code)
		}
		return CheckResult{Errored, fmt.Sprintf("exit %d: %s", code, reason)}
	}
}

func probeJob(name, ns, image string, labels map[string]string, timeout time.Duration) *batchv1.Job {
	backoff := int32(0)
	deadline := int64(timeout.Seconds()) + 10
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoff,
			ActiveDeadlineSeconds: &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "probe",
						Image:   image,
						Command: []string{"sh", "/probe/probe.sh"},
						VolumeMounts: []corev1.VolumeMount{{
							Name: "script", MountPath: "/probe",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "script",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name},
							},
						},
					}},
				},
			},
		},
	}
}

func ensureNamespace(ctx context.Context, kc kubernetes.Interface, ns string) error {
	_, err := kc.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("check namespace %q: %w", ns, err)
	}
	_, err = kc.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns, Labels: map[string]string{probeLabel: "true"}}},
		metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %q: %w", ns, err)
	}
	return nil
}

// podLogs returns the probe pod's logs (best-effort; empty on error).
func podLogs(ctx context.Context, e *Evaluator, ns, pod string) string {
	req := e.Client.Typed.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{})
	rc, err := req.Stream(ctx)
	if err != nil {
		return ""
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return ""
	}
	return string(b)
}

func randSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}
