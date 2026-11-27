// Package iot publishes to AWS IoT Core through the IoT data plane (cloud-side Publisher).
package iot

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iotdataplane"
)

// Publisher publishes QoS 1 messages via IoT Core.
type Publisher struct{ c *iotdataplane.Client }

// New builds a publisher; endpoint is the account's IoT data endpoint host (iot:DescribeEndpoint, ATS).
func New(ctx context.Context, endpoint string) (*Publisher, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &Publisher{c: iotdataplane.NewFromConfig(cfg, func(o *iotdataplane.Options) {
		o.BaseEndpoint = aws.String("https://" + endpoint)
	})}, nil
}

// Publish implements mqtt.Publisher.
func (p *Publisher) Publish(ctx context.Context, topic string, payload []byte) error {
	_, err := p.c.Publish(ctx, &iotdataplane.PublishInput{Topic: &topic, Qos: 1, Payload: payload})
	return err
}
