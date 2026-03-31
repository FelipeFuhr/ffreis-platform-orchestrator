package configctl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

type errDynamoClient struct {
	getErr    error
	putErr    error
	deleteErr error
	queryErr  error
}

func (e errDynamoClient) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (e errDynamoClient) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if e.putErr != nil {
		return nil, e.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (e errDynamoClient) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if e.deleteErr != nil {
		return nil, e.deleteErr
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (e errDynamoClient) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if e.queryErr != nil {
		return nil, e.queryErr
	}
	return &dynamodb.QueryOutput{}, nil
}

func TestDynamoStore_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	store := configctl.NewDynamoStore(errDynamoClient{getErr: errors.New("get")}, testTable)
	if _, err := store.Get(ctx, testProject, testEnv, "key"); err == nil {
		t.Fatal("expected get error")
	}

	store = configctl.NewDynamoStore(errDynamoClient{putErr: errors.New("put")}, testTable)
	if err := store.Set(ctx, testProject, testEnv, "key", "value"); err == nil {
		t.Fatal("expected put error")
	}

	store = configctl.NewDynamoStore(errDynamoClient{deleteErr: errors.New("delete")}, testTable)
	if err := store.Delete(ctx, testProject, testEnv, "key"); err == nil {
		t.Fatal("expected delete error")
	}

	store = configctl.NewDynamoStore(errDynamoClient{queryErr: errors.New("query")}, testTable)
	if _, err := store.List(ctx, testProject, testEnv); err == nil {
		t.Fatal("expected query error")
	}
}
