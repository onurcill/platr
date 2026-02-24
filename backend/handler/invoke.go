package handler

import (
	"context"
	"encoding/json"

	"grpc-inspector/auth"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"

	"grpc-inspector/db"
	"grpc-inspector/grpcclient"
)

// InvokeHandler handles unary and streaming gRPC invocations.
type InvokeHandler struct {
	connHandler *ConnectionHandler
	db          *db.DB
	upgrader    websocket.Upgrader
}

func (h *InvokeHandler) getRole(workspaceID, userID string) string {
	role, _ := h.db.GetMemberRole(workspaceID, userID)
	return role
}

func NewInvokeHandler(connHandler *ConnectionHandler, database *db.DB) *InvokeHandler {
	return &InvokeHandler{
		connHandler: connHandler,
		db:          database,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// protoServiceCache: connID -> serviceName -> ServiceDescriptor
var protoServiceCache = map[string]map[string]*grpcclient.ServiceDescriptor{}

// protoFileCache: connID -> list of FileDescriptorProto (for dynamic invocation)
var protoFileCache = map[string][]*descriptorpb.FileDescriptorProto{}

// RegisterProtoServices caches uploaded proto descriptors for a connection.
func RegisterProtoServices(connID string, services []*grpcclient.ServiceDescriptor) {
	if protoServiceCache[connID] == nil {
		protoServiceCache[connID] = map[string]*grpcclient.ServiceDescriptor{}
	}
	for _, svc := range services {
		protoServiceCache[connID][svc.Name] = svc
	}
}

// RegisterProtoFiles caches raw FileDescriptorProtos for dynamic invocation.
func RegisterProtoFiles(connID string, fdps []*descriptorpb.FileDescriptorProto) {
	protoFileCache[connID] = fdps
}

// resolveServiceDescriptor checks proto cache first, then reflection.
// Handles leading dot and missing package prefix via suffix match.
func resolveServiceDescriptor(ctx context.Context, connID string, serviceName string, mc *grpcclient.ManagedConn) (*grpcclient.ServiceDescriptor, error) {
	// Normalize: remove leading dot
	clean := strings.TrimPrefix(serviceName, ".")

	if connCache, ok := protoServiceCache[connID]; ok {
		// Exact match
		if svc, ok := connCache[clean]; ok {
			return svc, nil
		}
		// Suffix match: ".AccountManagementGrpcService" matches "evrimx.proto.AccountManagementGrpcService"
		for fullName, svc := range connCache {
			if strings.HasSuffix(fullName, "."+clean) || fullName == clean {
				return svc, nil
			}
		}
	}
	rc := grpcclient.NewReflectionClient(mc.Get())
	return rc.DescribeService(ctx, clean)
}

// resolveMessageDescriptor builds a protoreflect.MessageDescriptor.
// Tries proto file cache first, then reflection.
func resolveMessageDescriptor(ctx context.Context, connID string, typeName string, mc *grpcclient.ManagedConn) (protoreflect.MessageDescriptor, error) {
	log.Printf("🔍 resolveMessage: connId=%s type=%s fileCacheLen=%d", connID, typeName, len(protoFileCache[connID]))
	// 1. Try proto file cache
	if fdps, ok := protoFileCache[connID]; ok && len(fdps) > 0 {
		reg := &protoFileRegistry{files: map[string]protoreflect.FileDescriptor{}}
		// Sort: files with no unresolved imports first (topological order)
		sorted := topoSortFDPs(fdps)
		for _, fdp := range sorted {
			fd, err := protodesc.NewFile(fdp, reg)
			if err != nil {
				continue
			}
			reg.files[fdp.GetName()] = fd
		}
		for _, fd := range reg.files {
			if md := findMessageInFile(fd, typeName); md != nil {
				return md, nil
			}
		}
	}

	// 2. Fall back to reflection
	rc := grpcclient.NewReflectionClient(mc.Get())
	fds, err := rc.FileDescriptorForSymbol(ctx, typeName)
	if err != nil {
		return nil, fmt.Errorf("reflection for %s: %w", typeName, err)
	}

	reg := &protoFileRegistry{files: map[string]protoreflect.FileDescriptor{}}
	for _, fdp := range fds {
		fd, err := protodesc.NewFile(fdp, reg)
		if err != nil {
			continue
		}
		reg.files[fdp.GetName()] = fd
	}
	for _, fd := range reg.files {
		if md := findMessageInFile(fd, typeName); md != nil {
			return md, nil
		}
	}
	return nil, fmt.Errorf("message %s not found", typeName)
}

// ── Types ─────────────────────────────────────────────────────────────────────

type unaryRequest struct {
	Service  string            `json:"service"`
	Method   string            `json:"method"`
	Payload  json.RawMessage   `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

type unaryResponse struct {
	HistoryID string            `json:"historyId"`
	Status    string            `json:"status"`
	Headers   map[string]string `json:"headers"`
	Trailers  map[string]string `json:"trailers"`
	Payload   json.RawMessage   `json:"payload,omitempty"`
	Error     string            `json:"error,omitempty"`
	DurationMs int64            `json:"durationMs"`
}

// ── Unary ─────────────────────────────────────────────────────────────────────

func (h *InvokeHandler) Unary(w http.ResponseWriter, r *http.Request) {
	// ── Quota check ───────────────────────────────────────────────────────────
	if h.db != nil {
		claims := auth.GetClaims(r.Context())
		if claims != nil {
			if quotaErr := h.db.CheckInvocationQuota(claims.UserID); quotaErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": quotaErr.Error(),
					"quota": quotaErr,
					"upgradeRequired": true,
				})
				return
			}
		}
	}

	connID := mux.Vars(r)["id"]
	mc, err := h.connHandler.Pool().Get(connID)
	if err != nil {
		jsonError(w, "connection not found", http.StatusNotFound)
		return
	}

	var req unaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve {{variable}} placeholders using active environment
	envID := r.URL.Query().Get("envId")
	log.Printf("[invoke] envId=%q", envID)
	var envVars []*db.EnvVar
	if envID != "" && h.db != nil {
		if env, err := h.db.GetEnvironment(envID); err == nil && env != nil {
			envVars = env.Variables
			log.Printf("[invoke] loaded %d env vars from env %s", len(envVars), envID)
		} else {
			log.Printf("[invoke] env load failed: %v", err)
		}
	}
	// Always resolve, even with no env vars (removes unresolved placeholders)
	for k, v := range req.Metadata {
		req.Metadata[k] = db.ResolveVariables(v, envVars)
	}
	payloadStr := db.ResolveVariables(string(req.Payload), envVars)
	req.Payload = json.RawMessage(payloadStr)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	md := metadata.New(mc.Options.Metadata)
	for k, v := range req.Metadata {
		md.Set(k, v)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	log.Printf("[invoke] metadata keys: conn=%v req=%v merged=%v", mc.Options.Metadata, req.Metadata, md)

	svcDesc, err := resolveServiceDescriptor(ctx, connID, req.Service, mc)
	if err != nil {
		jsonError(w, "reflect: "+err.Error(), http.StatusBadGateway)
		return
	}
	log.Printf("📌 svcDesc.Name=%q req.Service=%q fullMethod will be=/%s/%s", svcDesc.Name, req.Service, svcDesc.Name, req.Method)

	var methodDesc *grpcclient.MethodDescriptor
	for _, m := range svcDesc.Methods {
		if m.Name == req.Method {
			methodDesc = m
			break
		}
	}
	if methodDesc == nil {
		jsonError(w, fmt.Sprintf("method %s not found", req.Method), http.StatusBadRequest)
		return
	}

	inputMsg, err := buildMsg(ctx, connID, grpcclient.TrimLeadingDot(methodDesc.InputType), req.Payload, mc)
	if err != nil {
		jsonError(w, "build request: "+err.Error(), http.StatusBadRequest)
		return
	}

	outputMsg, err := buildMsg(ctx, connID, grpcclient.TrimLeadingDot(methodDesc.OutputType), nil, mc)
	if err != nil {
		jsonError(w, "build output type: "+err.Error(), http.StatusBadRequest)
		return
	}

	var headerMD, trailerMD metadata.MD
	// Use the resolved service name, trimming any leading dot
	fullMethod := fmt.Sprintf("/%s/%s", strings.TrimPrefix(svcDesc.Name, "."), req.Method)

	start := time.Now()
	callErr := mc.Get().Invoke(ctx, fullMethod, inputMsg, outputMsg,
		grpc.Header(&headerMD),
		grpc.Trailer(&trailerMD),
	)
	elapsed := time.Since(start)

	status := "OK"
	var payloadBytes json.RawMessage
	if callErr != nil {
		log.Printf("[invoke] call error: %v | fullMethod=%s | metadataKeys=%v", callErr, fullMethod, md)
		status = callErr.Error()
	} else {
		b, _ := protojson.Marshal(outputMsg)
		payloadBytes = json.RawMessage(b)
	}

	// ── Increment usage ──────────────────────────────────────────────────────
	if h.db != nil {
		if claims := auth.GetClaims(r.Context()); claims != nil {
			_ = h.db.IncrementUsage(claims.UserID, 1)
		}
	}

	hID := generateID()
	wsID := r.URL.Query().Get("workspaceId")
	PersistHistory(h.db, r, &db.HistoryEntry{
		ID:           hID,
		WorkspaceID:  wsID,
		ConnID:       mux.Vars(r)["id"],
		ConnAddress:  mc.Options.Address,
		Service:      req.Service,
		Method:       req.Method,
		RequestBody:  req.Payload,
		ResponseBody: payloadBytes,
		Status:       status,
		DurationMs:   elapsed.Milliseconds(),
	})

	jsonOK(w, unaryResponse{
		HistoryID:  hID,
		Status:     status,
		Headers:    flattenMD(headerMD),
		Trailers:   flattenMD(trailerMD),
		Payload:    payloadBytes,
		DurationMs: elapsed.Milliseconds(),
	})
}

// ── Streaming ─────────────────────────────────────────────────────────────────

type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type streamInitMsg struct {
	Service  string            `json:"service"`
	Method   string            `json:"method"`
	Payload  json.RawMessage   `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

func (h *InvokeHandler) Stream(w http.ResponseWriter, r *http.Request) {
	connID := mux.Vars(r)["id"]
	mc, err := h.connHandler.Pool().Get(connID)
	if err != nil {
		http.Error(w, "connection not found", http.StatusNotFound)
		return
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	var initMsg streamInitMsg
	if err := ws.ReadJSON(&initMsg); err != nil {
		sendWSError(ws, "read init: "+err.Error())
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	md := metadata.New(mc.Options.Metadata)
	for k, v := range initMsg.Metadata {
		md.Set(k, v)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	svcDesc, err := resolveServiceDescriptor(ctx, connID, initMsg.Service, mc)
	if err != nil {
		sendWSError(ws, "reflect: "+err.Error())
		return
	}

	var methodDesc *grpcclient.MethodDescriptor
	for _, m := range svcDesc.Methods {
		if m.Name == initMsg.Method {
			methodDesc = m
			break
		}
	}
	if methodDesc == nil {
		sendWSError(ws, fmt.Sprintf("method %s not found", initMsg.Method))
		return
	}

	// Use the resolved service name, trimming any leading dot
	fullMethod := fmt.Sprintf("/%s/%s", strings.TrimPrefix(svcDesc.Name, "."), initMsg.Method)
	streamDesc := &grpc.StreamDesc{
		ServerStreams: methodDesc.ServerStreaming,
		ClientStreams: methodDesc.ClientStreaming,
	}

	stream, err := mc.Get().NewStream(ctx, streamDesc, fullMethod)
	if err != nil {
		sendWSError(ws, "open stream: "+err.Error())
		return
	}

	// Send first payload if provided
	if len(initMsg.Payload) > 0 && string(initMsg.Payload) != "null" {
		inputMsg, err := buildMsg(ctx, connID, grpcclient.TrimLeadingDot(methodDesc.InputType), initMsg.Payload, mc)
		if err != nil {
			sendWSError(ws, "build msg: "+err.Error())
			return
		}
		if err := stream.SendMsg(inputMsg); err != nil {
			sendWSError(ws, "send: "+err.Error())
			return
		}
	}

	if !methodDesc.ClientStreaming {
		stream.CloseSend()
	}

	// Client streaming: read from WS and send to stream
	if methodDesc.ClientStreaming {
		go func() {
			for {
				var frame wsMessage
				if err := ws.ReadJSON(&frame); err != nil {
					return
				}
				if frame.Type == "end" {
					stream.CloseSend()
					return
				}
				inputMsg, err := buildMsg(ctx, connID, grpcclient.TrimLeadingDot(methodDesc.InputType), frame.Payload, mc)
				if err != nil {
					sendWSError(ws, err.Error())
					return
				}
				if err := stream.SendMsg(inputMsg); err != nil {
					return
				}
			}
		}()
	}

	// Read from stream and send to WS
	outputType := grpcclient.TrimLeadingDot(methodDesc.OutputType)
	for {
		outputMsg, err := buildMsg(ctx, connID, outputType, nil, mc)
		if err != nil {
			sendWSError(ws, err.Error())
			return
		}
		err = stream.RecvMsg(outputMsg)
		if err == io.EOF {
			_ = ws.WriteJSON(wsMessage{Type: "end"})
			return
		}
		if err != nil {
			sendWSError(ws, err.Error())
			return
		}
		b, _ := protojson.Marshal(outputMsg)
		if err := ws.WriteJSON(wsMessage{Type: "message", Payload: json.RawMessage(b)}); err != nil {
			return
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildMsg(ctx context.Context, connID string, typeName string, payload json.RawMessage, mc *grpcclient.ManagedConn) (*dynamicpb.Message, error) {
	msgDesc, err := resolveMessageDescriptor(ctx, connID, typeName, mc)
	if err != nil {
		return nil, err
	}
	msg := dynamicpb.NewMessage(msgDesc)
	if len(payload) > 0 && string(payload) != "null" {
		if err := protojson.Unmarshal(payload, msg); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}
	return msg, nil
}

func sendWSError(ws *websocket.Conn, msg string) {
	_ = ws.WriteJSON(wsMessage{Type: "error", Error: msg})
}

func flattenMD(md metadata.MD) map[string]string {
	out := map[string]string{}
	for k, vs := range md {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

type protoFileRegistry struct {
	files map[string]protoreflect.FileDescriptor
}

func (r *protoFileRegistry) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, ok := r.files[path]; ok {
		return fd, nil
	}
	return protoregistry.GlobalFiles.FindFileByPath(path)
}

func (r *protoFileRegistry) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	for _, fd := range r.files {
		if d := findDescriptorInFile(fd, name); d != nil {
			return d, nil
		}
	}
	return protoregistry.GlobalFiles.FindDescriptorByName(name)
}

func findDescriptorInFile(fd protoreflect.FileDescriptor, name protoreflect.FullName) protoreflect.Descriptor {
	msgs := fd.Messages()
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if md.FullName() == name {
			return md
		}
	}
	enums := fd.Enums()
	for i := 0; i < enums.Len(); i++ {
		ed := enums.Get(i)
		if ed.FullName() == name {
			return ed
		}
	}
	return nil
}

func findMessageInFile(fd protoreflect.FileDescriptor, fullName string) protoreflect.MessageDescriptor {
	msgs := fd.Messages()
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if string(md.FullName()) == fullName {
			return md
		}
		if nested := findInNestedMessages(md, fullName); nested != nil {
			return nested
		}
	}
	return nil
}

func findInNestedMessages(parent protoreflect.MessageDescriptor, fullName string) protoreflect.MessageDescriptor {
	nested := parent.Messages()
	for i := 0; i < nested.Len(); i++ {
		md := nested.Get(i)
		if string(md.FullName()) == fullName {
			return md
		}
		if n := findInNestedMessages(md, fullName); n != nil {
			return n
		}
	}
	return nil
}

// topoSortFDPs sorts FileDescriptorProtos so dependencies come before dependents.
func topoSortFDPs(fdps []*descriptorpb.FileDescriptorProto) []*descriptorpb.FileDescriptorProto {
	byName := map[string]*descriptorpb.FileDescriptorProto{}
	for _, f := range fdps {
		byName[f.GetName()] = f
	}

	visited := map[string]bool{}
	var sorted []*descriptorpb.FileDescriptorProto

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		if f, ok := byName[name]; ok {
			for _, dep := range f.GetDependency() {
				visit(dep)
			}
			sorted = append(sorted, f)
		}
	}

	for _, f := range fdps {
		visit(f.GetName())
	}
	return sorted
}

// RegisterProtoServicesFromFDPs rebuilds service descriptors from raw FileDescriptorProtos.
// Used on startup to restore cached proto state from DB.
func RegisterProtoServicesFromFDPs(connID string, fdps []*descriptorpb.FileDescriptorProto) {
	if protoServiceCache[connID] == nil {
		protoServiceCache[connID] = map[string]*grpcclient.ServiceDescriptor{}
	}
	msgLookup := map[string]*descriptorpb.DescriptorProto{}
	for _, fdp := range fdps {
		pkg := fdp.GetPackage()
		for _, msg := range fdp.GetMessageType() {
			grpcclient.CollectMessages(pkg, msg, msgLookup)
		}
	}
	for _, fdp := range fdps {
		pkg := fdp.GetPackage()
		for _, svc := range fdp.GetService() {
			var fqn string
			if pkg == "" {
				fqn = svc.GetName()
			} else {
				fqn = pkg + "." + svc.GetName()
			}
			sd := &grpcclient.ServiceDescriptor{Name: fqn, Package: pkg}
			for _, m := range svc.GetMethod() {
				md := &grpcclient.MethodDescriptor{
					Name:            m.GetName(),
					FullName:        fqn + "/" + m.GetName(),
					ClientStreaming: m.GetClientStreaming(),
					ServerStreaming: m.GetServerStreaming(),
					InputType:       m.GetInputType(),
					OutputType:      m.GetOutputType(),
				}
				if schema, ok := grpcclient.BuildSchema(grpcclient.TrimLeadingDot(m.GetInputType()), msgLookup, 0); ok {
					md.InputSchema = schema
				}
				if schema, ok := grpcclient.BuildSchema(grpcclient.TrimLeadingDot(m.GetOutputType()), msgLookup, 0); ok {
					md.OutputSchema = schema
				}
				sd.Methods = append(sd.Methods, md)
			}
			protoServiceCache[connID][fqn] = sd
		}
	}
}

// GetCachedServices returns proto-uploaded services for a connection (no reflection needed).
func GetCachedServices(connID string) []*grpcclient.ServiceDescriptor {
	cache, ok := protoServiceCache[connID]
	if !ok {
		return nil
	}
	out := make([]*grpcclient.ServiceDescriptor, 0, len(cache))
	for _, svc := range cache {
		out = append(out, svc)
	}
	return out
}
