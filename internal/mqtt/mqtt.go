// Package mqtt wraps the Paho client with mutual-TLS and the Publisher abstraction
// (Mosquitto locally, AWS IoT Core in the cloud use the same wire protocol).
package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// Publisher sends a message to a topic at QoS 1.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Options configure a client.
type Options struct {
	URL           string // ssl://host:8883
	ClientID      string
	CA, Cert, Key string
	WillTopic     string // optional retained last-will
	WillPayload   []byte
	OnConnect     func(c *Client)
}

// Client is a connected mTLS MQTT client.
type Client struct{ c paho.Client }

// TLSConfig loads the CA and client certificate for mutual TLS.
func TLSConfig(ca, cert, key string) (*tls.Config, error) {
	pem, err := os.ReadFile(ca)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no CA certs in %s", ca)
	}
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, nil
}

// Connect dials the broker and keeps reconnecting in the background.
func Connect(o Options) (*Client, error) {
	tc, err := TLSConfig(o.CA, o.Cert, o.Key)
	if err != nil {
		return nil, err
	}
	po := paho.NewClientOptions().AddBroker(o.URL).SetClientID(o.ClientID).SetTLSConfig(tc).
		SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(2 * time.Second).
		SetKeepAlive(20 * time.Second).SetCleanSession(false).
		SetConnectionLostHandler(func(_ paho.Client, err error) { slog.Warn("mqtt connection lost", "err", err) })
	if o.WillTopic != "" {
		po.SetBinaryWill(o.WillTopic, o.WillPayload, 1, true)
	}
	cl := &Client{}
	po.SetOnConnectHandler(func(paho.Client) {
		slog.Info("mqtt connected", "url", o.URL, "client", o.ClientID)
		if o.OnConnect != nil {
			o.OnConnect(cl)
		}
	})
	cl.c = paho.NewClient(po)
	t := cl.c.Connect()
	if !t.WaitTimeout(20 * time.Second) {
		return nil, fmt.Errorf("mqtt connect timeout to %s", o.URL)
	}
	return cl, t.Error()
}

// Publish implements Publisher.
func (c *Client) Publish(_ context.Context, topic string, payload []byte) error {
	return c.PublishRetained(topic, payload, false)
}

// PublishRetained publishes at QoS 1, optionally retained.
func (c *Client) PublishRetained(topic string, payload []byte, retained bool) error {
	t := c.c.Publish(topic, 1, retained, payload)
	if !t.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("publish %s: timeout", topic)
	}
	return t.Error()
}

// Subscribe registers a QoS 1 handler.
func (c *Client) Subscribe(topic string, h func(topic string, payload []byte)) error {
	t := c.c.Subscribe(topic, 1, func(_ paho.Client, m paho.Message) { h(m.Topic(), m.Payload()) })
	if !t.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("subscribe %s: timeout", topic)
	}
	return t.Error()
}

// Close disconnects.
func (c *Client) Close() { c.c.Disconnect(250) }
