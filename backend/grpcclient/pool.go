package grpcclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ConnectOptions holds configuration for a gRPC connection.
type ConnectOptions struct {
	Address     string            `json:"address"`      // host:port
	TLS         bool              `json:"tls"`          // use TLS
	Insecure    bool              `json:"insecure"`     // skip TLS verification
	Metadata    map[string]string `json:"metadata"`     // default metadata headers
	DialTimeout int               `json:"dialTimeout"`  // seconds, default 10
}

// ManagedConn wraps a grpc.ClientConn with its options.
type ManagedConn struct {
	ID      string
	Options ConnectOptions
	conn    *grpc.ClientConn
	mu      sync.RWMutex
}

// Get returns the underlying grpc.ClientConn, reconnecting if needed.
func (mc *ManagedConn) Get() *grpc.ClientConn {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.conn
}

// State returns the current connectivity state.
func (mc *ManagedConn) State() string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if mc.conn == nil {
		return "DISCONNECTED"
	}
	return mc.conn.GetState().String()
}

// Close tears down the connection.
func (mc *ManagedConn) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.conn != nil {
		return mc.conn.Close()
	}
	return nil
}

// Pool manages multiple named gRPC connections.
type Pool struct {
	mu    sync.RWMutex
	conns map[string]*ManagedConn
}

func NewPool() *Pool {
	return &Pool{conns: make(map[string]*ManagedConn)}
}

// Connect dials a new gRPC connection and stores it by id.
func (p *Pool) Connect(id string, opts ConnectOptions) (*ManagedConn, error) {
	timeout := time.Duration(opts.DialTimeout) * time.Second
	if opts.DialTimeout == 0 {
		timeout = 10 * time.Second
	}

	dialOpts := []grpc.DialOption{
		grpc.WithBlock(),
	}

	if opts.TLS {
		tlsCfg := &tls.Config{}
		if opts.Insecure {
			tlsCfg.InsecureSkipVerify = true
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	//nolint:staticcheck // grpc.DialContext deprecated in 1.63 but widely used
	conn, err := grpc.DialContext(ctx, opts.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", opts.Address, err)
	}

	mc := &ManagedConn{
		ID:      id,
		Options: opts,
		conn:    conn,
	}

	p.mu.Lock()
	// close existing connection with same id if present
	if old, ok := p.conns[id]; ok {
		_ = old.Close()
	}
	p.conns[id] = mc
	p.mu.Unlock()

	return mc, nil
}

// Get retrieves a connection by id.
func (p *Pool) Get(id string) (*ManagedConn, error) {
	p.mu.RLock()
	mc, ok := p.conns[id]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("connection %q not found", id)
	}
	return mc, nil
}

// List returns all managed connections.
func (p *Pool) List() []*ManagedConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*ManagedConn, 0, len(p.conns))
	for _, mc := range p.conns {
		out = append(out, mc)
	}
	return out
}

// Remove closes and removes a connection.
func (p *Pool) Remove(id string) error {
	p.mu.Lock()
	mc, ok := p.conns[id]
	if ok {
		delete(p.conns, id)
	}
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("connection %q not found", id)
	}
	return mc.Close()
}

// Test checks whether the connection is alive (not SHUTDOWN / TRANSIENT_FAILURE).
func (p *Pool) Test(id string) (bool, string, error) {
	mc, err := p.Get(id)
	if err != nil {
		return false, "", err
	}
	state := mc.conn.GetState()
	ok := state != connectivity.Shutdown && state != connectivity.TransientFailure
	return ok, state.String(), nil
}

// OutgoingMetadata builds a context with the connection's default metadata merged
// with any per-request overrides.
func OutgoingMetadata(base map[string]string, override map[string]string) context.Context {
	md := metadata.New(nil)
	for k, v := range base {
		md.Set(k, v)
	}
	for k, v := range override {
		md.Set(k, v)
	}
	return metadata.NewOutgoingContext(context.Background(), md)
}
