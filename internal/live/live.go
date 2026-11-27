// Package live keeps the latest telemetry frame and heartbeat per station (DynamoDB).
package live

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Item is the live state of one station.
type Item struct {
	State       string
	HeartbeatAt time.Time
	Frame       []byte // raw telemetry JSON
}

// Store reads and writes live station state.
type Store interface {
	PutFrame(ctx context.Context, station string, frame []byte) error
	PutStatus(ctx context.Context, station, state string, at time.Time) error
	Get(ctx context.Context, station string) (Item, bool, error)
}

// DDB is the DynamoDB implementation (table station_live, PK station).
type DDB struct {
	Table string
	c     *dynamodb.Client
}

// NewDDB connects using the default AWS config (honours AWS_ENDPOINT_URL_DYNAMODB for local).
func NewDDB(ctx context.Context, table string) (*DDB, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &DDB{Table: table, c: dynamodb.NewFromConfig(cfg)}, nil
}

// EnsureTable creates the table if missing (local dynamodb only; Terraform owns it in AWS).
func (d *DDB) EnsureTable(ctx context.Context) error {
	_, err := d.c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            &d.Table,
		BillingMode:          types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("station"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("station"), KeyType: types.KeyTypeHash}},
	})
	var inUse *types.ResourceInUseException
	if errors.As(err, &inUse) {
		return nil
	}
	return err
}

func (d *DDB) update(ctx context.Context, station, expr string, names map[string]string, vals map[string]types.AttributeValue) error {
	_, err := d.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &d.Table, Key: map[string]types.AttributeValue{"station": &types.AttributeValueMemberS{Value: station}},
		UpdateExpression: &expr, ExpressionAttributeNames: names, ExpressionAttributeValues: vals,
	})
	return err
}

// PutFrame implements Store.
func (d *DDB) PutFrame(ctx context.Context, station string, frame []byte) error {
	return d.update(ctx, station, "SET frame = :f, updated_at = :u", nil, map[string]types.AttributeValue{
		":f": &types.AttributeValueMemberS{Value: string(frame)},
		":u": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	})
}

// PutStatus implements Store.
func (d *DDB) PutStatus(ctx context.Context, station, state string, at time.Time) error {
	return d.update(ctx, station, "SET #s = :s, heartbeat_at = :h", map[string]string{"#s": "state"}, map[string]types.AttributeValue{
		":s": &types.AttributeValueMemberS{Value: state},
		":h": &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339)},
	})
}

// Get implements Store.
func (d *DDB) Get(ctx context.Context, station string) (Item, bool, error) {
	out, err := d.c.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &d.Table, Key: map[string]types.AttributeValue{"station": &types.AttributeValueMemberS{Value: station}},
	})
	if err != nil || out.Item == nil {
		return Item{}, false, err
	}
	str := func(k string) string {
		if v, ok := out.Item[k].(*types.AttributeValueMemberS); ok {
			return v.Value
		}
		return ""
	}
	it := Item{State: str("state"), Frame: []byte(str("frame"))}
	it.HeartbeatAt, _ = time.Parse(time.RFC3339, str("heartbeat_at"))
	return it, true, nil
}

// Memory is an in-process Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]Item
}

// PutFrame implements Store.
func (m *Memory) PutFrame(_ context.Context, station string, frame []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]Item{}
	}
	it := m.m[station]
	it.Frame = frame
	m.m[station] = it
	return nil
}

// PutStatus implements Store.
func (m *Memory) PutStatus(_ context.Context, station, state string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]Item{}
	}
	it := m.m[station]
	it.State, it.HeartbeatAt = state, at
	m.m[station] = it
	return nil
}

// Get implements Store.
func (m *Memory) Get(_ context.Context, station string) (Item, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.m[station]
	return it, ok, nil
}
