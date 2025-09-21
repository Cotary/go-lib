package rabbitMQ

import "context"

type Consumer interface {
	Consume(ctx context.Context, msg *Delivery) error
}
