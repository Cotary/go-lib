package rabbitMQ

type Consumer interface {
	Handle(msg *Delivery) error
}
