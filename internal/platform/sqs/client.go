// Package sqs is the SQS equivalent of internal/platform/kafka — a producer,
// a long-poll consumer, app-level retry-queue + DLQ flow, and W3C trace
// context propagation. Behaviour and routing semantics mirror the Kafka
// package so callers can reason about both brokers the same way.
package sqs

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	pcfg "github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// SQSAPI is the subset of *awssqs.Client that the package needs. Exists so
// tests can mock the SDK with mockery instead of standing up LocalStack.
type SQSAPI interface {
	SendMessage(ctx context.Context, params *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, params *awssqs.ReceiveMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *awssqs.DeleteMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.DeleteMessageOutput, error)
	GetQueueUrl(ctx context.Context, params *awssqs.GetQueueUrlInput, optFns ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error)
	ChangeMessageVisibility(ctx context.Context, params *awssqs.ChangeMessageVisibilityInput, optFns ...func(*awssqs.Options)) (*awssqs.ChangeMessageVisibilityOutput, error)
}

// NewClient builds an AWS SDK v2 SQS client. cfg.Endpoint is honoured when
// non-empty (LocalStack dev); otherwise the SDK's default resolver is used.
// Credentials come from the SDK's default chain (env / shared / IAM role).
//
// On boot the caller should invoke VerifyQueue once on cfg.Consumer.QueueURL
// to fail fast if the queue is missing — boilerplate does NOT auto-provision.
func NewClient(ctx context.Context, cfg pcfg.SQSConfig) (SQSAPI, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("sqs: load aws config: %w", err)
	}

	clientOpts := []func(*awssqs.Options){}
	if cfg.Endpoint != "" {
		ep := cfg.Endpoint
		clientOpts = append(clientOpts, func(o *awssqs.Options) {
			o.BaseEndpoint = &ep
		})
	}
	return awssqs.NewFromConfig(awsCfg, clientOpts...), nil
}

// VerifyQueue calls GetQueueUrl on the queueURL's queue name to confirm it
// exists. Use during worker boot to fail fast on misconfiguration. queueURL
// must be the full URL — the queue name is parsed from its last path segment.
func VerifyQueue(ctx context.Context, client SQSAPI, queueURL string) error {
	name, err := QueueName(queueURL)
	if err != nil {
		return err
	}
	if _, err := client.GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{QueueName: &name}); err != nil {
		return fmt.Errorf("sqs: verify queue %q: %w", name, err)
	}
	return nil
}

// QueueName returns the queue name from a queue URL.
//
//	https://sqs.us-east-1.amazonaws.com/123456789012/events  →  events
func QueueName(queueURL string) (string, error) {
	if queueURL == "" {
		return "", fmt.Errorf("sqs: empty queue URL")
	}
	for i := len(queueURL) - 1; i >= 0; i-- {
		if queueURL[i] == '/' {
			return queueURL[i+1:], nil
		}
	}
	return queueURL, nil
}
