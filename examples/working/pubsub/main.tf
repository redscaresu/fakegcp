# Pub/Sub topic + subscription. Subscriptions FK-validate their
# `topic` parent on Create (TestPubSubSubscriptionFKViolation),
# but topic delete intentionally does NOT block on live
# subscriptions — fakegcp mirrors real Pub/Sub orphan semantics
# (TestPubSubTopicDeleteWithSubscriptions): the topic is deleted,
# the subscriptions survive, and a real subscriber would see the
# topic field flip to "_deleted-topic_". The subscription's
# `topic` field is also immutable on PATCH; only mutable fields
# like ack_deadline_seconds can be updated in place.

resource "google_pubsub_topic" "main" {
  name = "test-topic"
}

resource "google_pubsub_subscription" "main" {
  name  = "test-subscription"
  topic = google_pubsub_topic.main.name

  ack_deadline_seconds = 20
}
