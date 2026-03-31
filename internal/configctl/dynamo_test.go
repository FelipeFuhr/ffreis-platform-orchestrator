package configctl_test

import (
	"context"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

const (
	testTable   = "test-table"
	testProject = "proj"
	testEnv     = "env"
)

// mockDynamoClient implements DynamoClient with an in-memory map.
type mockDynamoClient struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue
}

func newMockDynamoClient() *mockDynamoClient {
	return &mockDynamoClient{items: make(map[string]map[string]types.AttributeValue)}
}

func (m *mockDynamoClient) itemKey(pk, sk string) string { return pk + "##" + sk }

func (m *mockDynamoClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pkVal := params.Key["PK"].(*types.AttributeValueMemberS).Value
	skVal := params.Key["SK"].(*types.AttributeValueMemberS).Value
	item, ok := m.items[m.itemKey(pkVal, skVal)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: item}, nil
}

func (m *mockDynamoClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pkVal := params.Item["PK"].(*types.AttributeValueMemberS).Value
	skVal := params.Item["SK"].(*types.AttributeValueMemberS).Value
	m.items[m.itemKey(pkVal, skVal)] = params.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pkVal := params.Key["PK"].(*types.AttributeValueMemberS).Value
	skVal := params.Key["SK"].(*types.AttributeValueMemberS).Value
	delete(m.items, m.itemKey(pkVal, skVal))
	return &dynamodb.DeleteItemOutput{}, nil
}

func (m *mockDynamoClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Extract PK value from expression attribute values.
	pkAttr, ok := params.ExpressionAttributeValues[":pk"]
	if !ok {
		return &dynamodb.QueryOutput{}, nil
	}
	pkVal := pkAttr.(*types.AttributeValueMemberS).Value
	prefixAttr, ok := params.ExpressionAttributeValues[":prefix"]
	if !ok {
		return &dynamodb.QueryOutput{}, nil
	}
	prefix := prefixAttr.(*types.AttributeValueMemberS).Value

	var items []map[string]types.AttributeValue
	for _, item := range m.items {
		itemPK := item["PK"].(*types.AttributeValueMemberS).Value
		itemSK := item["SK"].(*types.AttributeValueMemberS).Value
		if itemPK == pkVal && len(itemSK) >= len(prefix) && itemSK[:len(prefix)] == prefix {
			items = append(items, item)
		}
	}
	return &dynamodb.QueryOutput{Items: items, Count: int32(len(items))}, nil
}

func newTestStore() (*configctl.DynamoStore, *mockDynamoClient) {
	mc := newMockDynamoClient()
	return configctl.NewDynamoStore(mc, testTable), mc
}

// TestSetGet: set a value, get it back; get a different key returns ErrNotFound.
func TestSetGet(t *testing.T) {
	store, _ := newTestStore()
	ctx := context.Background()

	if err := store.Set(ctx, testProject, testEnv, "my/key", "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get(ctx, testProject, testEnv, "my/key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}

	_, err = store.Get(ctx, testProject, testEnv, "other/key")
	if !configctl.IsNotFound(err) {
		t.Errorf("expected ErrNotFound for missing key, got %v", err)
	}
}

// TestIdempotentSet: setting same key+value twice results in one stored record.
func TestIdempotentSet(t *testing.T) {
	store, mc := newTestStore()
	ctx := context.Background()

	if err := store.Set(ctx, testProject, testEnv, "key", "value"); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	// Count items before second set.
	mc.mu.Lock()
	countBefore := len(mc.items)
	mc.mu.Unlock()

	// Set same value again.
	if err := store.Set(ctx, testProject, testEnv, "key", "value"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	mc.mu.Lock()
	countAfter := len(mc.items)
	mc.mu.Unlock()

	if countAfter != countBefore {
		t.Errorf("expected idempotent set (no new record), got %d before, %d after", countBefore, countAfter)
	}

	// Value should still be retrievable.
	got, err := store.Get(ctx, testProject, testEnv, "key")
	if err != nil {
		t.Fatalf("Get after idempotent set: %v", err)
	}
	if got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

// TestDelete: set then delete; subsequent get returns ErrNotFound.
func TestDelete(t *testing.T) {
	store, _ := newTestStore()
	ctx := context.Background()

	if err := store.Set(ctx, testProject, testEnv, "key", "val"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, testProject, testEnv, "key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get(ctx, testProject, testEnv, "key")
	if !configctl.IsNotFound(err) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// TestList: set 3 keys, list returns all 3.
func TestList(t *testing.T) {
	store, _ := newTestStore()
	ctx := context.Background()

	keys := map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	}
	for k, v := range keys {
		if err := store.Set(ctx, testProject, testEnv, k, v); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	all, err := store.List(ctx, testProject, testEnv)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 items, got %d: %v", len(all), all)
	}
	for k, want := range keys {
		got, ok := all[k]
		if !ok {
			t.Errorf("key %q missing from list", k)
			continue
		}
		if got != want {
			t.Errorf("key %q: expected %q, got %q", k, want, got)
		}
	}
}

// Ensure the mock implements the interface (compile-time check).
var _ interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
} = (*mockDynamoClient)(nil)

// Suppress unused variable warning.
var _ = aws.String
