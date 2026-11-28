// Package iot publishes to AWS IoT Core through the IoT data plane (cloud-side Publisher).
package iot

import (
	"context"
	"net"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iotdataplane"
)

// Publisher publishes QoS 1 messages via IoT Core.
type Publisher struct{ c *iotdataplane.Client }

// New builds a publisher for the account's IoT data endpoint (iot:DescribeEndpoint, ATS).
//
// The IoT data service has no private DNS name, so inside the VPC (no NAT, no internet) vpceHost, the
// interface endpoint's DNS name, is dialled instead while TLS and SigV4 still use the real endpoint host.
func New(ctx context.Context, endpoint, vpceHost string) (*Publisher, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if vpceHost != "" {
		cfg.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
			d := &net.Dialer{}
			tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return d.DialContext(ctx, network, net.JoinHostPort(vpceHost, "443"))
			}
		})
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
