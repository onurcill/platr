# gRPC Inspector — Backend

Go backend that acts as a proxy between the browser frontend and target gRPC servers.

## Quick start

```bash
# Install dependencies
go mod tidy

# Run (default port 8080)
go run ./...

# Or with live reload
make install-air && make dev
```

Set `PORT=<n>` env var to change the port.

---

## API Reference

### Connections

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/connections` | Create & dial a new connection |
| `GET` | `/api/connections` | List all active connections |
| `DELETE` | `/api/connections/{id}` | Close & remove a connection |
| `POST` | `/api/connections/{id}/test` | Test connectivity state |

**POST /api/connections**
```json
{
  "id": "my-service",          // optional, auto-generated if omitted
  "name": "User Service",      // display label
  "address": "localhost:50051",
  "tls": false,
  "insecure": false,           // skip TLS cert verification
  "metadata": {                // default headers sent on every call
    "authorization": "Bearer token"
  },
  "dialTimeout": 10            // seconds
}
```

---

### Reflection

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/connections/{id}/reflect/services` | List services via server reflection |
| `GET` | `/api/connections/{id}/reflect/service/{service}` | Describe a service (methods + schemas) |
| `GET` | `/api/connections/{id}/reflect/method/{service}/{method}` | Describe a single method |

**Response — DescribeService**
```json
{
  "name": "helloworld.Greeter",
  "package": "helloworld",
  "methods": [
    {
      "name": "SayHello",
      "fullName": "helloworld.Greeter/SayHello",
      "clientStreaming": false,
      "serverStreaming": false,
      "inputType": ".helloworld.HelloRequest",
      "outputType": ".helloworld.HelloReply",
      "inputSchema": {
        "name": "helloworld.HelloRequest",
        "fields": {
          "name": { "type": "string", "repeated": false, "optional": true }
        }
      },
      "outputSchema": { ... }
    }
  ]
}
```

---

### Invoke (Unary)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/connections/{id}/invoke` | Execute a unary gRPC call |

**Request**
```json
{
  "service": "helloworld.Greeter",
  "method": "SayHello",
  "payload": { "name": "world" },
  "metadata": { "x-request-id": "abc" }
}
```

**Response**
```json
{
  "historyId": "a1b2c3d4",
  "status": "OK",
  "headers": {},
  "trailers": {},
  "payload": { "message": "Hello world" },
  "durationMs": 12
}
```

---

### Invoke (Streaming — WebSocket)

Upgrade to WebSocket at `/api/connections/{id}/stream`.

**Protocol**

1. **Client → Server** — init frame (first message):
```json
{ "service": "pkg.SvcName", "method": "MethodName", "metadata": {} }
```

2. **Client → Server** — send a message (client/bidi streaming):
```json
{ "type": "message", "payload": { ... } }
```

3. **Client → Server** — close send (client/bidi streaming):
```json
{ "type": "end" }
```

4. **Server → Client** — response message:
```json
{ "type": "message", "payload": { ... } }
```

5. **Server → Client** — stream finished:
```json
{ "type": "trailer", "meta": { "grpc-status": "0" } }
{ "type": "end" }
```

6. **Server → Client** — error:
```json
{ "type": "error", "error": "rpc error: ..." }
```

---

### History

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/history` | List all history (newest first) |
| `GET` | `/api/history/{id}` | Get a single entry |
| `DELETE` | `/api/history/{id}` | Delete a single entry |
| `DELETE` | `/api/history` | Clear all history |

---

### Proto File Upload

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/proto/upload` | Upload a `FileDescriptorSet` binary |
| `POST` | `/api/proto/parse` | Parse a base64-encoded `FileDescriptorSet` |

Generate a `FileDescriptorSet` with:
```bash
protoc --descriptor_set_out=descriptor.bin \
       --include_imports \
       your_service.proto
```

---

## Architecture

```
Browser ──HTTP/WS──▶ Go Backend ──gRPC──▶ Target Server
```

The backend:
- Manages a pool of persistent `grpc.ClientConn` connections
- Uses **server reflection** (v1alpha) to enumerate services and resolve message descriptors
- Builds **dynamicpb** messages at runtime — no generated code needed
- Logs every call to an in-memory history store (last 500 entries)
