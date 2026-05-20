package broker

// Project-wide broker topology. These constants are the single source of
// truth for exchange/queue/routing names; both the producer (cmd/server) and
// the consumer (cmd/worker) reference the same values, so any rename happens
// in exactly one place.
const (
	// ExchangeName is the topic exchange every avatar event is published to.
	ExchangeName = "avatars.exchange"
	// ExchangeType is the exchange kind. Topic gives us cheap routing-key
	// dispatch without sacrificing the option to add new event types later.
	ExchangeType = "topic"

	// QueueUploaded receives AvatarUploadEvent messages.
	QueueUploaded = "avatars.uploaded"
	// QueueDeleted receives AvatarDeleteEvent messages.
	QueueDeleted = "avatars.deleted"

	// RoutingUploaded is the routing key bound to QueueUploaded.
	RoutingUploaded = "avatar.uploaded"
	// RoutingDeleted is the routing key bound to QueueDeleted.
	RoutingDeleted = "avatar.deleted"

	// DLXName is the dead-letter exchange. Messages nacked without requeue
	// from the work queues land here for later inspection.
	DLXName = "avatars.dlx"
	// QueueDead is the single fanout-bound queue that retains dead-lettered
	// messages from both work queues.
	QueueDead = "avatars.dead"
)
