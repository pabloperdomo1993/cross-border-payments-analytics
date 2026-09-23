// Package kafka wires payments-service to Kafka: a small producer used
// only by the outbox relay (internal/outbox) to publish
// payments.created events. This mirrors payment-processor's own
// producer almost exactly — accepted, minimal duplication rather than
// a shared library, matching this repo's established convention that
// each service owns its own infrastructure code.
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

// Producer publishes messages to Kafka via a synchronous producer, so
// callers know immediately whether a publish succeeded.
type Producer struct {
	sync sarama.SyncProducer
}

// NewProducer builds a Producer connected to brokers.
func NewProducer(brokers []string) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 3
	cfg.Producer.Return.Successes = true

	sp, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}
	return &Producer{sync: sp}, nil
}

// Publish sends value (with key) to topic. It respects ctx cancellation
// by racing the (otherwise blocking) SendMessage call against ctx.Done()
// in a goroutine; if ctx wins, Publish returns ctx.Err() and the
// SendMessage call is left to finish in the background (its result is
// discarded into a buffered channel, so that goroutine never leaks
// blocked on a send).
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.ByteEncoder(key),
		Value: sarama.ByteEncoder(value),
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := p.sync.SendMessage(msg)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("publish to %s: %w", topic, err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close releases the underlying producer's resources.
func (p *Producer) Close() error {
	return p.sync.Close()
}
