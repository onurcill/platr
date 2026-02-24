package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/proto"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"

	"grpc-inspector/auth"
	"grpc-inspector/db"
	"grpc-inspector/grpcclient"
)

type ProtoHandler struct {
	connHandler *ConnectionHandler
	db          *db.DB
}

func NewProtoHandler(connHandler *ConnectionHandler, database *db.DB) *ProtoHandler {
	return &ProtoHandler{connHandler: connHandler, db: database}
}

// protoParseResult is returned to the frontend.
type protoParseResult struct {
	Files    []string                        `json:"files"`
	Services []*grpcclient.ServiceDescriptor `json:"services"`
}

// protoParseOutput carries both the API result and raw file descriptors for caching.
type protoParseOutput struct {
	result *protoParseResult
	fdps   []*descriptorpb.FileDescriptorProto
}

// ── Upload (single file) ──────────────────────────────────────────────────────

// POST /api/proto/upload  (multipart: file=<file>, connId=<optional>)
func (h *ProtoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		jsonError(w, "read data: "+err.Error(), http.StatusBadRequest)
		return
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	connID := r.FormValue("connId")
	log.Printf("🔵 proto upload: filename=%s connId=%q", filename, connID)

	var out *protoParseOutput
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".proto" {
		out, err = parseProtoSource(filename, data)
		if err != nil {
			jsonError(w, "parse .proto: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		fds := &descriptorpb.FileDescriptorSet{}
		if err := proto.Unmarshal(data, fds); err != nil {
			jsonError(w, "invalid .bin FileDescriptorSet: "+err.Error(), http.StatusBadRequest)
			return
		}
		out = parseFileDescriptorSet(fds)
	}

	if connID != "" {
		RegisterProtoServices(connID, out.result.Services)
		RegisterProtoFiles(connID, out.fdps)
		log.Printf("✅ proto cache set: connId=%s services=%d files=%d", connID, len(out.result.Services), len(out.fdps))
	} else {
		log.Printf("⚠️ proto upload without connId — cache skipped")
	}

	jsonOK(w, out.result)
}

// ── Upload (multiple files) ───────────────────────────────────────────────────

// POST /api/proto/upload-multi  (multipart: files[]=<files>, connId=<optional>)
func (h *ProtoHandler) UploadMultiple(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		jsonError(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	tmpDir, err := os.MkdirTemp("", "grpc-inspector-multi-*")
	if err != nil {
		jsonError(w, "temp dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	connID := r.FormValue("connId")
	var protoFiles []string

	fileHeaders := r.MultipartForm.File["files[]"]
	if len(fileHeaders) == 0 {
		fileHeaders = r.MultipartForm.File["file"]
	}

	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(f, 5<<20))
		f.Close()

		base := filepath.Base(fh.Filename)
		os.WriteFile(filepath.Join(tmpDir, base), data, 0600)
		if strings.ToLower(filepath.Ext(base)) == ".proto" {
			protoFiles = append(protoFiles, base)
		}
	}

	if len(protoFiles) == 0 {
		jsonError(w, "no .proto files found", http.StatusBadRequest)
		return
	}

	parser := protoparse.Parser{
		ImportPaths:           []string{tmpDir},
		IncludeSourceCodeInfo: false,
	}

	result := &protoParseResult{}
	var allFDPs []*descriptorpb.FileDescriptorProto

	fds, err := parser.ParseFiles(protoFiles...)
	if err != nil {
		fdprotos, err2 := parser.ParseFilesButDoNotLink(protoFiles...)
		if err2 != nil {
			jsonError(w, "parse error: "+err.Error(), http.StatusBadRequest)
			return
		}
		out := buildResultFromProtos(fdprotos)
		if connID != "" {
			RegisterProtoServices(connID, out.result.Services)
			RegisterProtoFiles(connID, out.fdps)
		}
		jsonOK(w, out.result)
		return
	}

	for _, fd := range fds {
		fdp := fd.AsFileDescriptorProto()
		allFDPs = append(allFDPs, fdp)
		result.Files = append(result.Files, fdp.GetName())
		pkg := fdp.GetPackage()
		msgLookup := map[string]*descriptorpb.DescriptorProto{}
		for _, msg := range fdp.GetMessageType() {
			grpcclient.CollectMessages(pkg, msg, msgLookup)
		}
		for _, svc := range fdp.GetService() {
			result.Services = append(result.Services, buildServiceDescriptor(pkg, svc, msgLookup))
		}
	}

	if connID != "" {
		RegisterProtoServices(connID, result.Services)
		RegisterProtoFiles(connID, allFDPs)
		if h.db != nil {
			wsID := r.FormValue("workspaceId")
			connAddr := r.FormValue("connAddress")
			if wsID == "" {
				if claims := auth.GetClaims(r.Context()); claims != nil {
					wsID = r.FormValue("workspaceId")
				}
			}
			if wsID != "" && connAddr != "" {
				dbFiles := make([]db.ProtoFile, len(allFDPs))
				for i, fdp := range allFDPs {
					b, _ := proto.Marshal(fdp)
					dbFiles[i] = db.ProtoFile{ID: generateID(), WorkspaceID: wsID, ConnAddress: connAddr, Filename: fdp.GetName(), Data: b}
				}
				_ = h.db.SaveProtoFiles(wsID, connAddr, dbFiles)
			}
		}
	}
	jsonOK(w, result)
}

// ── Parse (JSON body with base64 binary) ─────────────────────────────────────

// POST /api/proto/parse
func (h *ProtoHandler) Parse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data []byte `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(req.Data, fds); err != nil {
		jsonError(w, "invalid FileDescriptorSet: "+err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, parseFileDescriptorSet(fds).result)
}

// ── Core parsers ──────────────────────────────────────────────────────────────

func parseProtoSource(filename string, data []byte) (*protoParseOutput, error) {
	tmpDir, err := os.MkdirTemp("", "grpc-inspector-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	base := filepath.Base(filename)
	if base == "" || base == "." {
		base = "upload.proto"
	}

	if err := os.WriteFile(filepath.Join(tmpDir, base), data, 0600); err != nil {
		return nil, err
	}

	parser := protoparse.Parser{
		ImportPaths:           []string{tmpDir},
		IncludeSourceCodeInfo: false,
	}

	fds, err := parser.ParseFiles(base)
	if err != nil {
		// Fallback: unlinked parse
		fdprotos, err2 := parser.ParseFilesButDoNotLink(base)
		if err2 != nil {
			return nil, err
		}
		return buildResultFromProtos(fdprotos), nil
	}

	result := &protoParseResult{}
	var allFDPs []*descriptorpb.FileDescriptorProto
	for _, fd := range fds {
		fdp := fd.AsFileDescriptorProto()
		allFDPs = append(allFDPs, fdp)
		result.Files = append(result.Files, fdp.GetName())
		pkg := fdp.GetPackage()
		msgLookup := map[string]*descriptorpb.DescriptorProto{}
		for _, msg := range fdp.GetMessageType() {
			grpcclient.CollectMessages(pkg, msg, msgLookup)
		}
		for _, svc := range fdp.GetService() {
			result.Services = append(result.Services, buildServiceDescriptor(pkg, svc, msgLookup))
		}
	}
	return &protoParseOutput{result: result, fdps: allFDPs}, nil
}

func parseFileDescriptorSet(fds *descriptorpb.FileDescriptorSet) *protoParseOutput {
	result := &protoParseResult{}
	msgLookup := map[string]*descriptorpb.DescriptorProto{}

	for _, fd := range fds.GetFile() {
		result.Files = append(result.Files, fd.GetName())
		for _, msg := range fd.GetMessageType() {
			grpcclient.CollectMessages(fd.GetPackage(), msg, msgLookup)
		}
	}
	for _, fd := range fds.GetFile() {
		for _, svc := range fd.GetService() {
			result.Services = append(result.Services, buildServiceDescriptor(fd.GetPackage(), svc, msgLookup))
		}
	}
	return &protoParseOutput{result: result, fdps: fds.GetFile()}
}

func buildResultFromProtos(fdprotos []*descriptorpb.FileDescriptorProto) *protoParseOutput {
	result := &protoParseResult{}
	msgLookup := map[string]*descriptorpb.DescriptorProto{}

	for _, fdp := range fdprotos {
		result.Files = append(result.Files, fdp.GetName())
		for _, msg := range fdp.GetMessageType() {
			grpcclient.CollectMessages(fdp.GetPackage(), msg, msgLookup)
		}
	}
	for _, fdp := range fdprotos {
		for _, svc := range fdp.GetService() {
			result.Services = append(result.Services, buildServiceDescriptor(fdp.GetPackage(), svc, msgLookup))
		}
	}
	return &protoParseOutput{result: result, fdps: fdprotos}
}

func buildServiceDescriptor(
	pkg string,
	svc *descriptorpb.ServiceDescriptorProto,
	msgLookup map[string]*descriptorpb.DescriptorProto,
) *grpcclient.ServiceDescriptor {
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
	return sd
}
