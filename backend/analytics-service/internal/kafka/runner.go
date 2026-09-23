package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
)

// RunConsumerGroup joins the consumer group and consumes topics with
// handler until ctx is cancelled. Sarama's Consume call returns after
// every rebalance, so this loops it — the same pattern used by
// payment-processor's consumer.
func RunConsumerGroup(ctx context.Context, brokers []string, groupID string, topics []string, handler sarama.ConsumerGroupHandler, logger *slog.Logger) error {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Return.Errors = true

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return fmt.Errorf("create consumer group: %w", err)
	}
	defer group.Close()

	go func() {
		for consumeErr := range group.Errors() {
			logger.Error("consumer group error", slog.String("error", consumeErr.Error()))
		}
	}()

	for {
		if err := group.Consume(ctx, topics, handler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			return fmt.Errorf("consume: %w", err)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}
