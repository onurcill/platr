package k8s

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"
)

// K8sService represents a discovered Kubernetes service.
type K8sService struct {
	Name      string     `json:"name"`
	Namespace string     `json:"namespace"`
	ClusterIP string     `json:"clusterIp"`
	Ports     []PortInfo `json:"ports"`
}

// PortInfo describes a single port on a K8s service.
type PortInfo struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	Protocol   string `json:"protocol"`
	TargetPort string `json:"targetPort"`
}

// ForwardedService is an active port-forward session.
type ForwardedService struct {
	ID         string `json:"id"`
	Namespace  string `json:"namespace"`
	Service    string `json:"service"`
	PodName    string `json:"podName"`
	LocalPort  int    `json:"localPort"`
	RemotePort int    `json:"remotePort"`
	Address    string `json:"address"` // localhost:localPort
	stopCh     chan struct{}
}

// Manager handles K8s discovery and port-forwarding.
type Manager struct {
	client     *kubernetes.Clientset
	restConfig *rest.Config
	forwards   map[string]*ForwardedService
	mu         sync.RWMutex
}

// NewManager creates a new K8s manager, auto-detecting in-cluster vs kubeconfig.
func NewManager() (*Manager, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build k8s config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}
	return &Manager{
		client:     client,
		restConfig: cfg,
		forwards:   make(map[string]*ForwardedService),
	}, nil
}

// buildConfig tries in-cluster first, then falls back to kubeconfig.
func buildConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("neither in-cluster nor kubeconfig worked: %w", err)
	}
	return cfg, nil
}

// ListNamespaces returns all namespace names.
func (m *Manager) ListNamespaces(ctx context.Context) ([]string, error) {
	nsList, err := m.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// ListServices returns all services across all (or specified) namespaces.
func (m *Manager) ListServices(ctx context.Context, namespaces []string) ([]K8sService, error) {
	var result []K8sService

	if len(namespaces) == 0 {
		nsList, err := m.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	for _, ns := range namespaces {
		svcList, err := m.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, svc := range svcList.Items {
			if svc.Spec.Type == corev1.ServiceTypeExternalName {
				continue
			}
			k := K8sService{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				ClusterIP: svc.Spec.ClusterIP,
			}
			for _, p := range svc.Spec.Ports {
				k.Ports = append(k.Ports, PortInfo{
					Name:       p.Name,
					Port:       p.Port,
					Protocol:   string(p.Protocol),
					TargetPort: p.TargetPort.String(),
				})
			}
			result = append(result, k)
		}
	}
	return result, nil
}

// Forward starts a port-forward for the given service and remote port.
func (m *Manager) Forward(ctx context.Context, namespace, serviceName string, remotePort int) (*ForwardedService, error) {
	podName, err := m.findPodForService(ctx, namespace, serviceName)
	if err != nil {
		return nil, fmt.Errorf("find pod for %s/%s: %w", namespace, serviceName, err)
	}

	localPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		if err := m.runPortForward(namespace, podName, localPort, remotePort, stopCh, readyCh); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	select {
	case <-readyCh:
		// success
	case err := <-errCh:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-time.After(15 * time.Second):
		close(stopCh)
		return nil, fmt.Errorf("port-forward timed out")
	}

	id := generateID()
	fwd := &ForwardedService{
		ID:         id,
		Namespace:  namespace,
		Service:    serviceName,
		PodName:    podName,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Address:    fmt.Sprintf("localhost:%d", localPort),
		stopCh:     stopCh,
	}

	m.mu.Lock()
	m.forwards[id] = fwd
	m.mu.Unlock()

	return fwd, nil
}

// StopForward stops an active port-forward by ID.
func (m *Manager) StopForward(id string) error {
	m.mu.Lock()
	fwd, ok := m.forwards[id]
	if ok {
		delete(m.forwards, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("forward %q not found", id)
	}
	close(fwd.stopCh)
	return nil
}

// ListForwards returns all active port-forwards.
func (m *Manager) ListForwards() []*ForwardedService {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ForwardedService, 0, len(m.forwards))
	for _, f := range m.forwards {
		out = append(out, f)
	}
	return out
}

// findPodForService finds a running pod backing the given service.
func (m *Manager) findPodForService(ctx context.Context, namespace, serviceName string) (string, error) {
	svc, err := m.client.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get service: %w", err)
	}
	if len(svc.Spec.Selector) == 0 {
		return "", fmt.Errorf("service %s has no pod selector", serviceName)
	}

	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: svc.Spec.Selector})
	pods, err := m.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Status.ContainerStatuses {
			if c.Ready {
				return pod.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no ready pod found for service %s", serviceName)
}

// runPortForward executes the actual port-forward using SPDY.
func (m *Manager) runPortForward(namespace, podName string, localPort, remotePort int, stopCh, readyCh chan struct{}) error {
	cfg := m.restConfig

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return fmt.Errorf("round tripper: %w", err)
	}

	u, err := url.Parse(cfg.Host)
	if err != nil {
		return fmt.Errorf("parse host: %w", err)
	}
	u.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return fmt.Errorf("create port-forwarder: %w", err)
	}

	return fw.ForwardPorts()
}

// freePort finds a free local TCP port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
