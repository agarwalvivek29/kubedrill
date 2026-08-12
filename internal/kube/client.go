// Package kube builds Kubernetes clients from a session kubeconfig. Callers
// get a dynamic client + a RESTMapper so they can operate on any kind
// (built-in or CRD) without compile-time type knowledge (AD-3: access is
// kubeconfig-shaped, resolved here from bytes).
package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// Client bundles the handles the engine needs to read and apply objects.
type Client struct {
	Dyn    dynamic.Interface
	Mapper meta.RESTMapper
}

// FromKubeconfig builds a Client from raw kubeconfig bytes.
func FromKubeconfig(kubeconfig []byte) (*Client, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("kube: rest config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))
	return &Client{Dyn: dyn, Mapper: mapper}, nil
}

// ResourceFor resolves an apiVersion+kind to a namespaced/cluster-scoped
// dynamic resource interface. namespace is ignored for cluster-scoped kinds.
func (c *Client) ResourceFor(apiVersion, kind, namespace string) (dynamic.ResourceInterface, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("kube: parse apiVersion %q: %w", apiVersion, err)
	}
	mapping, err := c.Mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
	if err != nil {
		return nil, fmt.Errorf("kube: no REST mapping for %s/%s: %w", apiVersion, kind, err)
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if namespace == "" {
			namespace = "default"
		}
		return c.Dyn.Resource(mapping.Resource).Namespace(namespace), nil
	}
	return c.Dyn.Resource(mapping.Resource), nil
}

// DefaultAPIVersion supplies a sensible apiVersion for a bare kind when a
// challenge omits it, covering the common built-ins used in challenges.
func DefaultAPIVersion(kind string) string {
	switch kind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
		return "apps/v1"
	case "Pod", "Service", "ConfigMap", "Secret", "Namespace", "Node",
		"PersistentVolumeClaim", "PersistentVolume", "ServiceAccount", "Endpoints":
		return "v1"
	case "Job", "CronJob":
		return "batch/v1"
	case "Ingress", "NetworkPolicy":
		return "networking.k8s.io/v1"
	case "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding":
		return "rbac.authorization.k8s.io/v1"
	default:
		return ""
	}
}
