// Command ingest consumes the SQS event bus. In AWS it is the Lambda behind the IoT rule -> SQS;
// locally it also bridges MQTT into SQS (the IoT topic rule stand-in) and polls the queue.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	awsevents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/mini4141/ground-station-cloud/internal/db/sqlc"
	"github.com/mini4141/ground-station-cloud/internal/ingest"
	"github.com/mini4141/ground-station-cloud/internal/live"
	"github.com/mini4141/ground-station-cloud/internal/mqtt"
	"github.com/mini4141/ground-station-cloud/internal/platform"
)

func main() {
	ctx := context.Background()
	defer platform.Setup(ctx, "ingest")()
	if err := run(ctx); err != nil {
		slog.Error("ingest failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	pool, err := platform.OpenDB(ctx, platform.Env("DATABASE_URL", "postgres://gs:gs@localhost:5432/gs?sslmode=disable"))
	if err != nil {
		return err
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
	p := &ingest.Processor{Q: sqlc.New(pool), Live: ddb}

	if platform.InLambda() {
		lambda.Start(func(ctx context.Context, ev awsevents.SQSEvent) (awsevents.SQSEventResponse, error) {
			var resp awsevents.SQSEventResponse
			for _, r := range ev.Records {
				if err := p.Process(ctx, []byte(r.Body)); err != nil {
					slog.Error("process", "err", err, "id", r.MessageId)
					resp.BatchItemFailures = append(resp.BatchItemFailures, awsevents.SQSBatchItemFailure{ItemIdentifier: r.MessageId})
				}
			}
			return resp, nil
		})
		return nil
	}

	platform.ServeAdmin(platform.Env("ADMIN_ADDR", ":8091"))
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	q := sqs.NewFromConfig(cfg)
	created, err := q.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(platform.Env("SQS_QUEUE", "gs-events"))})
	if err != nil {
		return err
	}
	queueURL := created.QueueUrl

	// bridge: MQTT -> SQS, adding station/kind exactly like the IoT topic rule does
	_, err = mqtt.Connect(mqtt.Options{URL: platform.Env("MQTT_URL", "ssl://localhost:8883"), ClientID: "cloud-bridge" + platform.Env("MQTT_CLIENT_SUFFIX", ""),
		CA: platform.Env("MQTT_CA", ".local/certs/ca.crt"), Cert: platform.Env("MQTT_CERT", ".local/certs/cloud.crt"), Key: platform.Env("MQTT_KEY", ".local/certs/cloud.key"),
		OnConnect: func(c *mqtt.Client) {
			for _, kind := range []string{"telemetry", "status", "events"} {
				if err := c.Subscribe("gs/+/"+kind, func(topic string, payload []byte) {
					parts := strings.Split(topic, "/")
					var m map[string]any
					if json.Unmarshal(payload, &m) != nil {
						return
					}
					m["station"], m["kind"] = parts[1], parts[2]
					b, _ := json.Marshal(m)
					if _, err := q.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: queueURL, MessageBody: aws.String(string(b))}); err != nil {
						slog.Error("bridge send", "err", err)
					}
				}); err != nil {
					slog.Error("bridge subscribe", "err", err)
				}
			}
		}})
	if err != nil {
		return err
	}

	for {
		out, err := q.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: queueURL, MaxNumberOfMessages: 10, WaitTimeSeconds: 1})
		if err != nil {
			slog.Error("receive", "err", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, m := range out.Messages {
			if err := p.Process(ctx, []byte(*m.Body)); err != nil {
				slog.Error("process", "err", err)
			}
			_, _ = q.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: queueURL, ReceiptHandle: m.ReceiptHandle})
		}
	}
}
