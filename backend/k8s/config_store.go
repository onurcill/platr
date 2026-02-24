package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeconfigEntry is the public metadata for an uploaded kubeconfig.
type KubeconfigEntry struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Contexts      []ContextInfo `json:"contexts"`
	ActiveContext string        `json:"activeContext"`
	UploadedAt    time.Time     `json:"uploadedAt"`
}

// ContextInfo is a single context within a kubeconfig.
type ContextInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	User    string `json:"user"`
}

type configEntry struct {
	KubeconfigEntry
	rawConfig     *clientcmdapi.Config
	activeClient  *kubernetes.Clientset
	activeRestCfg *rest.Config
}

// ConfigStore manages multiple uploaded kubeconfigs.
type ConfigStore struct {
	mu      sync.RWMutex
	entries map[string]*configEntry
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{entries: make(map[string]*configEntry)}
}

// Add parses and stores a kubeconfig. Returns the entry metadata.
func (s *ConfigStore) Add(name string, data []byte) (*KubeconfigEntry, error) {
	cfg, err := clientcmd.Load(data)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	contexts := make([]ContextInfo, 0, len(cfg.Contexts))
	for ctxName, ctx := range cfg.Contexts {
		contexts = append(contexts, ContextInfo{
			Name:    ctxName,
			Cluster: ctx.Cluster,
			User:    ctx.AuthInfo,
		})
	}

	activeCtx := cfg.CurrentContext
	if activeCtx == "" && len(contexts) > 0 {
		activeCtx = contexts[0].Name
	}

	id := generateID()
	entry := &configEntry{
		KubeconfigEntry: KubeconfigEntry{
			ID:            id,
			Name:          name,
			Contexts:      contexts,
			ActiveContext: activeCtx,
			UploadedAt:    time.Now(),
		},
		rawConfig: cfg,
	}

	if err := entry.buildClient(activeCtx); err != nil {
		return nil, fmt.Errorf("connect to cluster: %w", err)
	}

	s.mu.Lock()
	s.entries[id] = entry
	s.mu.Unlock()

	meta := entry.KubeconfigEntry
	return &meta, nil
}

// Remove deletes a stored kubeconfig.
func (s *ConfigStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return fmt.Errorf("kubeconfig %q not found", id)
	}
	delete(s.entries, id)
	return nil
}

// List returns all stored kubeconfig metadata.
func (s *ConfigStore) List() []*KubeconfigEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*KubeconfigEntry, 0, len(s.entries))
	for _, e := range s.entries {
		meta := e.KubeconfigEntry
		out = append(out, &meta)
	}
	return out
}

// SwitchContext switches the active context for an entry.
func (s *ConfigStore) SwitchContext(id, contextName string) (*KubeconfigEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("kubeconfig %q not found", id)
	}
	if err := entry.buildClient(contextName); err != nil {
		return nil, err
	}
	entry.ActiveContext = contextName
	meta := entry.KubeconfigEntry
	return &meta, nil
}

// GetManager returns a Manager for the given kubeconfig entry ID.
func (s *ConfigStore) GetManager(id string) (*Manager, error) {
	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kubeconfig %q not found", id)
	}
	return &Manager{
		client:     entry.activeClient,
		restConfig: entry.activeRestCfg,
		forwards:   make(map[string]*ForwardedService),
	}, nil
}

// buildClient (re)creates the k8s client for the given context.
func (e *configEntry) buildClient(contextName string) error {
	if _, ok := e.rawConfig.Contexts[contextName]; !ok {
		return fmt.Errorf("context %q not found", contextName)
	}

	clientConfig := clientcmd.NewDefaultClientConfig(
		*e.rawConfig,
		&clientcmd.ConfigOverrides{CurrentContext: contextName},
	)
	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("build k8s client: %w", err)
	}

	// Quick reachability check
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err != nil {
		return fmt.Errorf("cluster unreachable: %w", err)
	}

	e.activeClient = client
	e.activeRestCfg = restCfg
	return nil
}
