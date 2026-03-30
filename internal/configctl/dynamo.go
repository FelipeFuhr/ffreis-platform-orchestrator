package configctl

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoClient is the minimal DynamoDB API surface used by DynamoStore.
type DynamoClient interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// DynamoStore implements Client backed by DynamoDB.
// Key convention:
//
//	PK = "PROJECT#{project}#ENV#{env}"
//	SK = "CONFIG#{key}"
type DynamoStore struct {
	client    DynamoClient
	tableName string
}

// NewDynamoStore constructs a DynamoStore.
func NewDynamoStore(client DynamoClient, tableName string) *DynamoStore {
	return &DynamoStore{client: client, tableName: tableName}
}

type record struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	Value     string `dynamodbav:"value"`
	ItemType  string `dynamodbav:"item_type"`
	Version   int64  `dynamodbav:"version"`
	Checksum  string `dynamodbav:"checksum"`
	UpdatedAt string `dynamodbav:"updated_at"`
}

func pk(project, env string) string { return "PROJECT#" + project + "#ENV#" + env }
func sk(key string) string          { return "CONFIG#" + key }

func csumOf(v string) string {
	h := sha256.Sum256([]byte(v))
	return fmt.Sprintf("sha256:%x", h)
}

func (s *DynamoStore) Get(ctx context.Context, project, env, key string) (string, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk(project, env)},
			"SK": &types.AttributeValueMemberS{Value: sk(key)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("dynamodb GetItem: %w", err)
	}
	if len(out.Item) == 0 {
		return "", &ErrNotFoundError{Key: key}
	}
	var r record
	if err := attributevalue.UnmarshalMap(out.Item, &r); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	return r.Value, nil
}

func (s *DynamoStore) Set(ctx context.Context, project, env, key, value string) error {
	// Get current record to derive version for optimistic-style update.
	var (
		currentVersion int64
		existing       record
	)

	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk(project, env)},
			"SK": &types.AttributeValueMemberS{Value: sk(key)},
		},
	})
	if err != nil {
		return fmt.Errorf("dynamodb GetItem: %w", err)
	}

	if len(out.Item) != 0 {
		if err := attributevalue.UnmarshalMap(out.Item, &existing); err != nil {
			return fmt.Errorf("unmarshal existing record: %w", err)
		}
		// Value unchanged — skip write (idempotent).
		if existing.Value == value {
			return nil
		}
		currentVersion = existing.Version
	}

	r := record{
		PK:        pk(project, env),
		SK:        sk(key),
		Value:     value,
		ItemType:  "config",
		Version:   currentVersion + 1,
		Checksum:  csumOf(value),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	av, merr := attributevalue.MarshalMap(r)
	if merr != nil {
		return fmt.Errorf("marshal: %w", merr)
	}
	_, werr := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if werr != nil {
		return fmt.Errorf("dynamodb PutItem: %w", werr)
	}
	return nil
}

func (s *DynamoStore) Delete(ctx context.Context, project, env, key string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk(project, env)},
			"SK": &types.AttributeValueMemberS{Value: sk(key)},
		},
	})
	if err != nil {
		return fmt.Errorf("dynamodb DeleteItem: %w", err)
	}
	return nil
}

func (s *DynamoStore) List(ctx context.Context, project, env string) (map[string]string, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: pk(project, env)},
			":prefix": &types.AttributeValueMemberS{Value: "CONFIG#"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb Query: %w", err)
	}

	result := make(map[string]string, len(out.Items))
	for _, item := range out.Items {
		var r record
		if err := attributevalue.UnmarshalMap(item, &r); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		// Strip "CONFIG#" prefix to return the bare key.
		key := r.SK
		if len(key) > 7 {
			key = key[7:]
		}
		result[key] = r.Value
	}
	return result, nil
}
