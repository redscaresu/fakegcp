# Update flow for google_pubsub_subscription: change ack_deadline_seconds.
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve   # in-place patch
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "ack_deadline_seconds" {
  type = number
}

resource "google_pubsub_topic" "main" {
  name = "events"
}

resource "google_pubsub_subscription" "main" {
  name                 = "events-pull"
  topic                = google_pubsub_topic.main.name
  ack_deadline_seconds = var.ack_deadline_seconds
}
