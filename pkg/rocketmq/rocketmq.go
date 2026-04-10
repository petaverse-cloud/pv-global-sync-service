// Package rocketmq provides RocketMQ producer and consumer wrappers.
package rocketmq

import (
	"context"
	"fmt"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
)

// Producer wraps RocketMQ producer with service-specific methods
type Producer struct {
	inner rocketmq.Producer
	topic string
}

// ProducerConfig holds RocketMQ producer configuration
type ProducerConfig struct {
	NameServer string
	Topic      string
	GroupID    string
}

// NewProducer creates a new RocketMQ producer
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	p, err := rocketmq.NewProducer(
		producer.WithNameServer([]string{cfg.NameServer}),
		producer.WithGroupName(cfg.GroupID),
		producer.WithRetry(3),
		producer.WithQueueSelector(producer.NewHashQueueSelector()),
	)
	if err != nil {
		return nil, fmt.Errorf("create producer: %w", err)
	}

	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start producer: %w", err)
	}

	return &Producer{
		inner: p,
		topic: cfg.Topic,
	}, nil
}

// SendSync sends a message synchronously and waits for acknowledgment
func (p *Producer) SendSync(ctx context.Context, key string, body []byte) error {
	msg := &primitive.Message{
		Topic: p.topic,
		Body:  body,
	}
	msg.WithKeys([]string{key})

	result, err := p.inner.SendSync(ctx, msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	if result.Status != primitive.SendOK {
		return fmt.Errorf("send failed: status=%v", result.Status)
	}

	return nil
}

// Shutdown gracefully shuts down the producer
func (p *Producer) Shutdown() error {
	return p.inner.Shutdown()
}

// Consumer wraps RocketMQ consumer with service-specific methods
type Consumer struct {
	inner   rocketmq.PushConsumer
	topic   string
	groupID string
}

// ConsumerConfig holds RocketMQ consumer configuration
type ConsumerConfig struct {
	NameServer string
	Topic      string
	GroupID    string
}

// MessageHandler is the callback type for processing consumed messages
type MessageHandler func(ctx context.Context, msg *primitive.MessageExt) (consumer.ConsumeResult, error)

// NewConsumer creates a new RocketMQ consumer with the given handler
func NewConsumer(cfg ConsumerConfig, handler MessageHandler) (*Consumer, error) {
	c, err := rocketmq.NewPushConsumer(
		consumer.WithGroupName(cfg.GroupID),
		consumer.WithNameServer([]string{cfg.NameServer}),
		consumer.WithConsumerOrder(true), // Enable ordered consumption per message key
		consumer.WithMaxReconsumeTimes(5),
	)
	if err != nil {
		return nil, fmt.Errorf("create consumer: %w", err)
	}

	// Subscribe to the topic
	if err := c.Subscribe(cfg.Topic, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			result, err := handler(ctx, msg)
			if err != nil {
				return consumer.ConsumeRetryLater, err
			}
			if result == consumer.ConsumeRetryLater {
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}); err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	return &Consumer{
		inner:   c,
		topic:   cfg.Topic,
		groupID: cfg.GroupID,
	}, nil
}

// Start begins consuming messages
func (c *Consumer) Start() error {
	return c.inner.Start()
}

// Shutdown gracefully shuts down the consumer
func (c *Consumer) Shutdown() error {
	return c.inner.Shutdown()
}
