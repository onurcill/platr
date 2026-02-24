package grpcclient

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"google.golang.org/grpc"
	rpb_v1     "google.golang.org/grpc/reflection/grpc_reflection_v1"
	rpb_v1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

// reflectStream is a common interface for v1 and v1alpha streams.
type reflectStream interface {
	Send(name string, symbol string) error
	ListServices() ([]string, error)
	FileContainingSymbol(symbol string) (*descriptorpb.FileDescriptorProto, error)
	Close()
}

// ReflectionClient tries v1 first, falls back to v1alpha.
type ReflectionClient struct {
	conn *grpc.ClientConn
}

func NewReflectionClient(conn *grpc.ClientConn) *ReflectionClient {
	return &ReflectionClient{conn: conn}
}

// ListServices returns all service names from the server.
func (rc *ReflectionClient) ListServices(ctx context.Context) ([]string, error) {
	// Try v1 first
	if names, err := rc.listServicesV1(ctx); err == nil {
		log.Printf("[reflection] v1 OK, %d services", len(names))
		return names, nil
	} else {
		log.Printf("[reflection] v1 failed: %v — trying v1alpha", err)
	}
	// Fall back to v1alpha
	names, err := rc.listServicesV1Alpha(ctx)
	if err != nil {
		log.Printf("[reflection] v1alpha also failed: %v", err)
		return nil, fmt.Errorf("reflection not supported (v1: tried, v1alpha: %w)", err)
	}
	log.Printf("[reflection] v1alpha OK, %d services", len(names))
	return names, nil
}

// DescribeService fetches the service descriptor and builds method info.
func (rc *ReflectionClient) DescribeService(ctx context.Context, serviceName string) (*ServiceDescriptor, error) {
	fds, err := rc.fileContainingSymbol(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	pkg := fds.GetPackage()
	msgs := map[string]*descriptorpb.DescriptorProto{}
	for _, msg := range fds.GetMessageType() {
		collectMessages(pkg, msg, msgs)
	}

	for _, svc := range fds.GetService() {
		fqn := pkg + "." + svc.GetName()
		if fqn != serviceName && svc.GetName() != serviceName {
			continue
		}
		sd := &ServiceDescriptor{
			Name:    fqn,
			Package: pkg,
		}
		for _, m := range svc.GetMethod() {
			md := &MethodDescriptor{
				Name:            m.GetName(),
				FullName:        fqn + "/" + m.GetName(),
				ClientStreaming: m.GetClientStreaming(),
				ServerStreaming: m.GetServerStreaming(),
				InputType:       m.GetInputType(),
				OutputType:      m.GetOutputType(),
			}
			if schema, ok := buildSchema(trimLeadingDot(m.GetInputType()), msgs, 0); ok {
				md.InputSchema = schema
			}
			if schema, ok := buildSchema(trimLeadingDot(m.GetOutputType()), msgs, 0); ok {
				md.OutputSchema = schema
			}
			sd.Methods = append(sd.Methods, md)
		}
		return sd, nil
	}
	return nil, fmt.Errorf("service %q not found in descriptor", serviceName)
}

// fileContainingSymbol fetches FileDescriptorProto, trying v1 then v1alpha.
func (rc *ReflectionClient) fileContainingSymbol(ctx context.Context, symbol string) (*descriptorpb.FileDescriptorProto, error) {
	if fds, err := rc.fileContainingSymbolV1(ctx, symbol); err == nil {
		return fds, nil
	} else {
		log.Printf("[reflection] fileContainingSymbol v1 failed for %s: %v", symbol, err)
	}
	return rc.fileContainingSymbolV1Alpha(ctx, symbol)
}

// ── v1 ────────────────────────────────────────────────────────────────────────

func (rc *ReflectionClient) listServicesV1(ctx context.Context) ([]string, error) {
	stub := rpb_v1.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("v1 stream: %w", err)
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1.ServerReflectionRequest{
		MessageRequest: &rpb_v1.ServerReflectionRequest_ListServices{ListServices: ""},
	}); err != nil {
		return nil, fmt.Errorf("v1 send: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("v1 recv: %w", err)
	}

	ls, ok := resp.MessageResponse.(*rpb_v1.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		return nil, fmt.Errorf("v1 unexpected response: %T", resp.MessageResponse)
	}

	names := make([]string, 0, len(ls.ListServicesResponse.Service))
	for _, s := range ls.ListServicesResponse.Service {
		names = append(names, s.Name)
	}
	return names, nil
}

func (rc *ReflectionClient) fileContainingSymbolV1(ctx context.Context, symbol string) (*descriptorpb.FileDescriptorProto, error) {
	stub := rpb_v1.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("v1 stream: %w", err)
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1.ServerReflectionRequest{
		MessageRequest: &rpb_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	}); err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	fdr, ok := resp.MessageResponse.(*rpb_v1.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("v1 unexpected response: %T", resp.MessageResponse)
	}

	if len(fdr.FileDescriptorResponse.FileDescriptorProto) == 0 {
		return nil, fmt.Errorf("v1 empty descriptor")
	}

	var fdp descriptorpb.FileDescriptorProto
	if err := proto.Unmarshal(fdr.FileDescriptorResponse.FileDescriptorProto[0], &fdp); err != nil {
		return nil, err
	}
	return &fdp, nil
}

// ── v1alpha ───────────────────────────────────────────────────────────────────

func (rc *ReflectionClient) listServicesV1Alpha(ctx context.Context) ([]string, error) {
	stub := rpb_v1alpha.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("server reflection not supported")
		}
		return nil, fmt.Errorf("v1alpha stream: %w", err)
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1alpha.ServerReflectionRequest{
		MessageRequest: &rpb_v1alpha.ServerReflectionRequest_ListServices{ListServices: ""},
	}); err != nil {
		return nil, fmt.Errorf("v1alpha send: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("v1alpha recv: %w", err)
	}

	ls, ok := resp.MessageResponse.(*rpb_v1alpha.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		return nil, fmt.Errorf("v1alpha unexpected response: %T", resp.MessageResponse)
	}

	names := make([]string, 0, len(ls.ListServicesResponse.Service))
	for _, s := range ls.ListServicesResponse.Service {
		names = append(names, s.Name)
	}
	return names, nil
}

func (rc *ReflectionClient) fileContainingSymbolV1Alpha(ctx context.Context, symbol string) (*descriptorpb.FileDescriptorProto, error) {
	stub := rpb_v1alpha.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("v1alpha stream: %w", err)
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1alpha.ServerReflectionRequest{
		MessageRequest: &rpb_v1alpha.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	}); err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	fdr, ok := resp.MessageResponse.(*rpb_v1alpha.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("v1alpha unexpected: %T", resp.MessageResponse)
	}

	if len(fdr.FileDescriptorResponse.FileDescriptorProto) == 0 {
		return nil, fmt.Errorf("v1alpha empty descriptor")
	}

	var fdp descriptorpb.FileDescriptorProto
	if err := proto.Unmarshal(fdr.FileDescriptorResponse.FileDescriptorProto[0], &fdp); err != nil {
		return nil, err
	}
	return &fdp, nil
}

// ── Schema helpers ────────────────────────────────────────────────────────────

func BuildSchema(typeName string, msgs map[string]*descriptorpb.DescriptorProto, depth int) (*MessageSchema, bool) {
	return buildSchema(typeName, msgs, depth)
}

func buildSchema(typeName string, msgs map[string]*descriptorpb.DescriptorProto, depth int) (*MessageSchema, bool) {
	if depth > 6 {
		return nil, false
	}
	msg, ok := msgs[typeName]
	if !ok {
		// Try without leading dot
		msg, ok = msgs[strings.TrimPrefix(typeName, ".")]
		if !ok {
			// Try suffix match (e.g. "GetCompanyRequest" matches "evrimx.proto.GetCompanyRequest")
			for k, v := range msgs {
				if strings.HasSuffix(k, "."+typeName) || k == typeName {
					msg = v
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, false
		}
	}
	schema := &MessageSchema{
		Name:   typeName,
		Fields: map[string]*FieldSchema{},
	}
	for _, f := range msg.GetField() {
		fs := &FieldSchema{
			Type:     protoFieldType(f),
			Repeated: f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
			Optional: f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
		}
		if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			nested, _ := buildSchema(trimLeadingDot(f.GetTypeName()), msgs, depth+1)
			fs.Nested = nested
		}
		schema.Fields[f.GetName()] = fs
	}
	return schema, true
}

func CollectMessages(pkg string, msg *descriptorpb.DescriptorProto, out map[string]*descriptorpb.DescriptorProto) {
	collectMessages(pkg, msg, out)
}

func collectMessages(pkg string, msg *descriptorpb.DescriptorProto, out map[string]*descriptorpb.DescriptorProto) {
	var key string
	if pkg == "" {
		key = msg.GetName()
	} else {
		key = pkg + "." + msg.GetName()
	}
	out[key] = msg
	for _, nested := range msg.GetNestedType() {
		collectMessages(key, nested, out)
	}
}

func protoFieldType(f *descriptorpb.FieldDescriptorProto) string {
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "bytes"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "bool"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "int32"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "int64"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32, descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "uint32"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return "uint64"
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "float"
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "double"
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return "enum:" + f.GetTypeName()
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		return "message:" + f.GetTypeName()
	default:
		return "unknown"
	}
}

func TrimLeadingDot(s string) string { return trimLeadingDot(s) }
func trimLeadingDot(s string) string {
	if len(s) > 0 && s[0] == '.' {
		return s[1:]
	}
	return s
}

// FileDescriptorForSymbol returns the FileDescriptorProto chain (symbol + its imports)
// needed to resolve a type. Tries v1 then v1alpha.
func (rc *ReflectionClient) FileDescriptorForSymbol(ctx context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	// Try v1 first
	if fdps, err := rc.fileChainV1(ctx, symbol); err == nil {
		return fdps, nil
	}
	return rc.fileChainV1Alpha(ctx, symbol)
}

func (rc *ReflectionClient) fileChainV1(ctx context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	stub := rpb_v1.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1.ServerReflectionRequest{
		MessageRequest: &rpb_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	}); err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	fdr, ok := resp.MessageResponse.(*rpb_v1.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response: %T", resp.MessageResponse)
	}

	var out []*descriptorpb.FileDescriptorProto
	for _, b := range fdr.FileDescriptorResponse.FileDescriptorProto {
		var fdp descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(b, &fdp); err == nil {
			out = append(out, &fdp)
		}
	}
	return out, nil
}

func (rc *ReflectionClient) fileChainV1Alpha(ctx context.Context, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	stub := rpb_v1alpha.NewServerReflectionClient(rc.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.CloseSend()

	if err := stream.Send(&rpb_v1alpha.ServerReflectionRequest{
		MessageRequest: &rpb_v1alpha.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	}); err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	fdr, ok := resp.MessageResponse.(*rpb_v1alpha.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response: %T", resp.MessageResponse)
	}

	var out []*descriptorpb.FileDescriptorProto
	for _, b := range fdr.FileDescriptorResponse.FileDescriptorProto {
		var fdp descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(b, &fdp); err == nil {
			out = append(out, &fdp)
		}
	}
	return out, nil
}
