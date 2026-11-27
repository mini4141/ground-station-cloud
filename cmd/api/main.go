// Command api serves the REST API: as an AWS Lambda behind API Gateway, or as a plain HTTP server.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/mini4141/ground-station-cloud/internal/auth"
	"github.com/mini4141/ground-station-cloud/internal/events"
	"github.com/mini4141/ground-station-cloud/internal/live"
	"github.com/mini4141/ground-station-cloud/internal/platform"
	"github.com/mini4141/ground-station-cloud/internal/server"
	"github.com/mini4141/ground-station-cloud/internal/storage"
	"github.com/mini4141/ground-station-cloud/internal/wiring"
)

func main() {
	ctx := context.Background()
	defer platform.Setup(ctx, "api")()
	if err := run(ctx); err != nil {
		slog.Error("api failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	d, err := wiring.New(ctx, "cloud-api")
	if err != nil {
		return err
	}
	srv := &server.Server{Pool: d.Pool, Q: d.Q, Events: d.Events, Dispatcher: d.Dispatcher, Sched: events.Ticker{},
		UploadSecret: d.Dispatcher.UploadSecret, PlanLeadMin: platform.EnvFloat("PLAN_LEAD_MIN", 5)}

	var local *storage.Local
	if bucket := os.Getenv("MISSION_BUCKET"); bucket != "" {
		if srv.Store, err = storage.NewS3(ctx, bucket); err != nil {
			return err
		}
	} else {
		local = &storage.Local{Dir: platform.Env("UPLOAD_DIR", ".local/uploads"), Secret: string(srv.UploadSecret), PublicBase: platform.Env("API_PUBLIC_URL", "http://localhost:8080")}
		srv.Store = local
	}
	ddb, err := live.NewDDB(ctx, platform.Env("LIVE_TABLE", "station_live"))
	if err != nil {
		return err
	}
	if os.Getenv("AWS_ENDPOINT_URL_DYNAMODB") != "" {
		if err := ddb.EnsureTable(ctx); err != nil {
			return err
		}
	}
	srv.Live = ddb
	if os.Getenv("SCHEDULER_LAMBDA_ARN") != "" {
		if srv.Sched, err = events.NewEventBridgeScheduler(ctx); err != nil {
			return err
		}
	}

	var verifier *auth.Verifier
	if platform.Env("TRUST_GATEWAY_JWT", "") == "true" {
		verifier = &auth.Verifier{TrustGateway: true}
	} else if verifier, err = auth.NewJWKS(platform.Env("JWKS_URL", "http://localhost:9000/.well-known/jwks.json")); err != nil {
		return err
	}
	h := srv.Handler(verifier, local, os.Getenv("CORS_ORIGIN"))

	if platform.InLambda() {
		lambda.Start(httpadapter.NewV2(h).ProxyWithContext)
		return nil
	}
	addr := platform.Env("HTTP_ADDR", ":8080")
	slog.Info("api listening", "addr", addr)
	return http.ListenAndServe(addr, h)
}
