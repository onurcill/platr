package grpcclient

// ServiceDescriptor holds metadata for a gRPC service and its methods.
type ServiceDescriptor struct {
	Name    string              `json:"name"`
	Package string              `json:"package"`
	Methods []*MethodDescriptor `json:"methods"`
}

// MethodDescriptor holds metadata for a single gRPC method.
type MethodDescriptor struct {
	Name            string         `json:"name"`
	FullName        string         `json:"fullName"`
	ClientStreaming bool           `json:"clientStreaming"`
	ServerStreaming bool           `json:"serverStreaming"`
	InputType       string         `json:"inputType"`
	OutputType      string         `json:"outputType"`
	InputSchema     *MessageSchema `json:"inputSchema,omitempty"`
	OutputSchema    *MessageSchema `json:"outputSchema,omitempty"`
}

// MessageSchema describes the fields of a proto message.
type MessageSchema struct {
	Name   string                  `json:"name"`
	Fields map[string]*FieldSchema `json:"fields"`
}

// FieldSchema describes a single field within a proto message.
type FieldSchema struct {
	Type     string         `json:"type"`
	Repeated bool           `json:"repeated"`
	Optional bool           `json:"optional"`
	Nested   *MessageSchema `json:"nested,omitempty"`
}
