// Package events emits domain events (EventBridge bus) and arms pass-reminder schedules (EventBridge Scheduler).
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
)

// Sink receives domain events.
type Sink interface {
	Emit(ctx context.Context, detailType string, detail any)
}

// Log writes events to the structured log (local).
type Log struct{}

// Emit implements Sink.
func (Log) Emit(_ context.Context, detailType string, detail any) {
	slog.Info("domain event", "type", detailType, "detail", detail)
}

// Bus puts events on the default EventBridge bus.
type Bus struct{ c *eventbridge.Client }

// NewBus builds the EventBridge sink.
func NewBus(ctx context.Context) (*Bus, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &Bus{c: eventbridge.NewFromConfig(cfg)}, nil
}

// Emit implements Sink; failures are logged, never fatal.
func (b *Bus) Emit(ctx context.Context, detailType string, detail any) {
	body, _ := json.Marshal(detail)
	_, err := b.c.PutEvents(ctx, &eventbridge.PutEventsInput{Entries: []ebtypes.PutEventsRequestEntry{{
		Source: aws.String("groundstation.platform"), DetailType: aws.String(detailType), Detail: aws.String(string(body)),
	}}})
	if err != nil {
		slog.Warn("eventbridge put events", "err", err)
	}
}

// Scheduler arms the dispatch of a booking's pass plan at a wall-clock instant.
type Scheduler interface {
	Schedule(ctx context.Context, bookingID string, at time.Time) error
}

// Ticker is the local no-op: the scheduler service polls due bookings against sim time.
type Ticker struct{}

// Schedule implements Scheduler.
func (Ticker) Schedule(context.Context, string, time.Time) error { return nil }

// EventBridgeScheduler creates one-time schedules targeting the scheduler Lambda.
type EventBridgeScheduler struct {
	c                         *scheduler.Client
	Group, LambdaARN, RoleARN string
}

// NewEventBridgeScheduler reads SCHEDULE_GROUP, SCHEDULER_LAMBDA_ARN and SCHEDULER_ROLE_ARN.
func NewEventBridgeScheduler(ctx context.Context) (*EventBridgeScheduler, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &EventBridgeScheduler{c: scheduler.NewFromConfig(cfg), Group: os.Getenv("SCHEDULE_GROUP"),
		LambdaARN: os.Getenv("SCHEDULER_LAMBDA_ARN"), RoleARN: os.Getenv("SCHEDULER_ROLE_ARN")}, nil
}

// Schedule implements Scheduler.
func (s *EventBridgeScheduler) Schedule(ctx context.Context, bookingID string, at time.Time) error {
	if at.Before(time.Now().Add(5 * time.Second)) {
		at = time.Now().Add(5 * time.Second)
	}
	input, _ := json.Marshal(map[string]string{"booking_id": bookingID})
	_, err := s.c.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
		Name: aws.String("pass-" + bookingID), GroupName: aws.String(s.Group),
		ScheduleExpression:    aws.String(fmt.Sprintf("at(%s)", at.UTC().Format("2006-01-02T15:04:05"))),
		FlexibleTimeWindow:    &schtypes.FlexibleTimeWindow{Mode: schtypes.FlexibleTimeWindowModeOff},
		ActionAfterCompletion: schtypes.ActionAfterCompletionNone,
		Target:                &schtypes.Target{Arn: &s.LambdaARN, RoleArn: &s.RoleARN, Input: aws.String(string(input))},
	})
	return err
}
