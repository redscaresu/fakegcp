# Pub/Sub topic + subscription. Tests the topic -> subscription FK
# chain that fakegcp's TestPubSubSubscriptionFKViolation +
# TestPubSubTopicDeleteWithSubscriptions handler tests assert.

resource "google_pubsub_topic" "main" {
  name = "test-topic"
}

resource "google_pubsub_subscription" "main" {
  name  = "test-subscription"
  topic = google_pubsub_topic.main.name

  ack_deadline_seconds = 20
}
