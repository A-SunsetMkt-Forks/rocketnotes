package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type Document struct {
	ID           string    `json:"id"`
	ParentId     string    `json:"parentId"`
	UserId       string    `json:"userId"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	LastModified time.Time `json:"lastModified"`
	Deleted      bool      `json:"deleted"`
	IsPublic     bool      `json:"isPublic"`
}

type SqsMessage struct {
	UserId        string `json:"userId"`
	DocumentId    string `json:"documentId"`
	DeleteVectors bool   `json:"deleteVectors"`
}

func init() {
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	id := request.PathParameters["id"]

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	var svc *dynamodb.DynamoDB

	if os.Getenv("USE_LOCAL_DYNAMODB") == "1" {
		svc = dynamodb.New(sess, aws.NewConfig().WithEndpoint("http://dynamodb:8000"))
	} else {
		svc = dynamodb.New(sess)
	}

	tableName := "tnn-Documents"

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(id),
			},
		},
	})
	if err != nil {
		log.Fatalf("Got error calling GetItem: %s", err)
	}

	if result.Item == nil {
		return events.APIGatewayProxyResponse{
			StatusCode: 404,
		}, nil
	}

	item := Document{}

	err = dynamodbattribute.UnmarshalMap(result.Item, &item)
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal Record, %v", err))
	}

	item.Deleted = true

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Fatalf("Got error marshalling new movie item: %s", err)
	}

	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}

	_, err = svc.PutItem(input)
	if err != nil {
		log.Fatalf("Got error calling PutItem: %s", err)
	}

	user_config, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("tnn-UserConfig"),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(item.UserId),
			},
		},
	})
	if err != nil {
		log.Fatalf("Got error calling GetItem: %s", err)
	}

	if user_config.Item != nil && os.Getenv("USE_LOCAL_DYNAMODB") != "1" {
		qsvc := sqs.New(sess)

		m := SqsMessage{item.UserId, item.ID, true}
		b, err := json.Marshal(m)

		_, err = qsvc.SendMessage(&sqs.SendMessageInput{
			DelaySeconds: aws.Int64(0),
			MessageBody:  aws.String(string(b)),
			QueueUrl:     aws.String(os.Getenv("queueUrl")),
		})
		if err != nil {
			log.Fatalf("Error sending sqs message: %s", err)
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
