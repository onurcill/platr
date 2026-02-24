package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"grpc-inspector/auth"
	"grpc-inspector/db"
	"grpc-inspector/handler"

	"grpc-inspector/middleware"
)

// loadDotEnv reads KEY=VALUE pairs from a .env file and sets them as env variables.
// Existing env variables are NOT overwritten (process env takes priority).
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // .env is optional
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Don't overwrite existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	log.Printf("📋 Loaded .env from %s", path)
}

func main() {
	// Load environment-specific file first, then .env as fallback
	// APP_ENV=development (default) | production | staging
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}
	loadDotEnv(".env." + appEnv) // .env.development or .env.production
	loadDotEnv(".env")           // shared defaults (not overwritten)
	log.Printf("🌍 Environment: %s", appEnv)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := flag.String("db", "", "Path to SQLite database file (default: ./grpc-inspector.db, env: DB_PATH)")
	flag.Parse()

	// Open database
	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}
	defer database.Close()


	// Proto file cache is restored per-connection on demand (workspace-scoped since v2)
	r := mux.NewRouter()
	r.Use(middleware.Logger)
	api := r.PathPrefix("/api").Subrouter()
	api.Use(auth.Middleware)

	// Auth (public — no JWT required)
	authHandler := handler.NewAuthHandler(database)
	api.HandleFunc("/auth/register", authHandler.Register).Methods("POST")
	api.HandleFunc("/auth/login", authHandler.Login).Methods("POST")
	api.HandleFunc("/auth/me", authHandler.Me).Methods("GET")

	// Workspaces
	wsHandler := handler.NewWorkspaceHandler(database)
	api.HandleFunc("/workspaces", wsHandler.List).Methods("GET")
	api.HandleFunc("/workspaces", wsHandler.Create).Methods("POST")
	api.HandleFunc("/workspaces/{id}", wsHandler.Get).Methods("GET")
	api.HandleFunc("/workspaces/{id}", wsHandler.Update).Methods("PUT")
	api.HandleFunc("/workspaces/{id}", wsHandler.Delete).Methods("DELETE")
	api.HandleFunc("/workspaces/{id}/members", wsHandler.ListMembers).Methods("GET")
	api.HandleFunc("/workspaces/{id}/members/{userId}", wsHandler.UpdateMember).Methods("PUT")
	api.HandleFunc("/workspaces/{id}/members/{userId}", wsHandler.RemoveMember).Methods("DELETE")
	api.HandleFunc("/workspaces/{id}/invites", wsHandler.CreateInvite).Methods("POST")
	api.HandleFunc("/workspaces/{id}/invites", wsHandler.ListInvites).Methods("GET")
	api.HandleFunc("/invites/{token}/accept", wsHandler.AcceptInvite).Methods("POST")

	// Roles & Permissions
	roleHandler := handler.NewRoleHandler(database)
	api.HandleFunc("/roles", roleHandler.List).Methods("GET")

	// Connections
	connHandler := handler.NewConnectionHandler()
	api.HandleFunc("/connections", connHandler.Create).Methods("POST")
	api.HandleFunc("/connections", connHandler.List).Methods("GET")
	api.HandleFunc("/connections/{id}", connHandler.Delete).Methods("DELETE")
	api.HandleFunc("/connections/{id}/test", connHandler.Test).Methods("POST")

	// Reflection
	reflectHandler := handler.NewReflectionHandler(connHandler)
	api.HandleFunc("/connections/{id}/reflect/services", reflectHandler.ListServices).Methods("GET")
	api.HandleFunc("/connections/{id}/reflect/service/{service}", reflectHandler.DescribeService).Methods("GET")
	api.HandleFunc("/connections/{id}/reflect/method/{service}/{method}", reflectHandler.DescribeMethod).Methods("GET")

	// Invoke (with DB for env variable resolution)
	invokeHandler := handler.NewInvokeHandler(connHandler, database)
	loadTestHandler := handler.NewLoadTestHandler(database, connHandler)
	api.HandleFunc("/connections/{id}/invoke", invokeHandler.Unary).Methods("POST")
	api.HandleFunc("/connections/{id}/stream", invokeHandler.Stream)
	api.HandleFunc("/connections/{id}/loadtest", loadTestHandler.Run).Methods("POST")

	// History
	historyHandler := handler.NewHistoryHandler(database)
	api.HandleFunc("/history", historyHandler.List).Methods("GET")
	api.HandleFunc("/history/{id}", historyHandler.Delete).Methods("DELETE")
	api.HandleFunc("/history", historyHandler.Clear).Methods("DELETE")


	// Billing & Subscriptions
	billingHandler := handler.NewBillingHandler(database)
	api.HandleFunc("/billing/plans", billingHandler.ListPlans).Methods("GET")
	api.HandleFunc("/billing/subscription", billingHandler.GetSubscription).Methods("GET")
	api.HandleFunc("/billing/checkout", billingHandler.CreateCheckout).Methods("POST")
	api.HandleFunc("/billing/portal", billingHandler.CreatePortal).Methods("POST")
	api.HandleFunc("/billing/trial", billingHandler.StartTrial).Methods("POST")
	// Webhook is public (no JWT) — Stripe calls it directly
	r.HandleFunc("/api/billing/webhook", billingHandler.StripeWebhook).Methods("POST")

	// Proto
	protoHandler := handler.NewProtoHandler(connHandler, database)
	api.HandleFunc("/proto/upload", protoHandler.Upload).Methods("POST")
	api.HandleFunc("/proto/parse", protoHandler.Parse).Methods("POST")
	api.HandleFunc("/proto/upload-multi", protoHandler.UploadMultiple).Methods("POST")

	// Environments
	envHandler := handler.NewEnvironmentHandler(database)
	api.HandleFunc("/environments", envHandler.List).Methods("GET")
	api.HandleFunc("/environments", envHandler.Create).Methods("POST")
	api.HandleFunc("/environments/{id}", envHandler.Get).Methods("GET")
	api.HandleFunc("/environments/{id}", envHandler.Update).Methods("PUT")
	api.HandleFunc("/environments/{id}", envHandler.Delete).Methods("DELETE")
	api.HandleFunc("/environments/{id}/duplicate", envHandler.Duplicate).Methods("POST")

	// Collections
	collectionHandler := handler.NewCollectionHandler(database)
	api.HandleFunc("/collections", collectionHandler.List).Methods("GET")
	api.HandleFunc("/collections", collectionHandler.Create).Methods("POST")
	api.HandleFunc("/collections/export", collectionHandler.Export).Methods("GET")
	api.HandleFunc("/collections/import", collectionHandler.Import).Methods("POST")
	api.HandleFunc("/collections/{id}", collectionHandler.Update).Methods("PUT")
	api.HandleFunc("/collections/{id}", collectionHandler.Delete).Methods("DELETE")
	api.HandleFunc("/collections/{id}/requests", collectionHandler.SaveRequest).Methods("POST")
	api.HandleFunc("/collections/requests/{reqId}", collectionHandler.UpdateRequest).Methods("PUT")
	api.HandleFunc("/collections/requests/{reqId}", collectionHandler.DeleteRequest).Methods("DELETE")
	api.HandleFunc("/collections/requests/{reqId}/move", collectionHandler.MoveRequest).Methods("POST")
	api.HandleFunc("/collections/migrate-orphans", collectionHandler.MigrateOrphans).Methods("POST")


	// K8s — kubectl subprocess port-forwarding
	k8sHandler := handler.NewK8sSimpleHandler(database)
	log.Printf("✅ K8s handler ready (kubectl subprocess mode)")
	api.HandleFunc("/k8s/kubeconfigs", k8sHandler.List).Methods("GET")
	api.HandleFunc("/k8s/kubeconfigs", k8sHandler.Upload).Methods("POST")
	api.HandleFunc("/k8s/kubeconfigs/{id}", k8sHandler.Delete).Methods("DELETE")
	api.HandleFunc("/k8s/kubeconfigs/{id}/context", k8sHandler.SwitchContext).Methods("POST")
	api.HandleFunc("/k8s/namespaces", k8sHandler.Namespaces).Methods("GET")
	api.HandleFunc("/k8s/services", k8sHandler.Services).Methods("GET")
	api.HandleFunc("/k8s/forwards", k8sHandler.ListForwards).Methods("GET")
	api.HandleFunc("/k8s/forwards/{id}", k8sHandler.StopForward).Methods("DELETE")
	api.HandleFunc("/k8s/connect", k8sHandler.ForwardAndConnect(connHandler)).Methods("POST")

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000", "*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	log.Printf("🚀 gRPC Inspector backend on :%s", port)
	if err := http.ListenAndServe(":"+port, c.Handler(r)); err != nil {
		log.Fatal(err)
	}
}
