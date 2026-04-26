# BROKEN: subscription points at a topic that does not exist.
#
# A platform engineer wires a pull subscription to a topic by hard-coded
# project path instead of a resource reference, and the project doesn't
# yet host that topic. Both fields are typed `string`, so the config is
# syntactically valid and passes all static checks.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `topic` is typed string; the value is
#                                  a valid string
#   terraform plan      ✓ passes — Terraform cannot verify the topic
#                                  actually exists in the project
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: Referenced resource not found
#
#   The subscription create handler resolves the referenced topic
#   before insert. If no topic row exists in this project, the create
#   fails — same outcome as the real Pub/Sub API, just earlier in the
#   loop than a full plan-and-apply against production.
#
# ── Why this matters ────────────────────────────────────────────────────
#
#   In production the FK violation has the same shape — the
#   subscription create 404s. In CI without fakegcp, this typically
#   gets discovered on the first real-environment apply, after the
#   broken Terraform has already merged.
resource "google_pubsub_subscription" "broken" {
  name  = "events-pull"
  topic = "projects/fake-project/topics/missing-topic"
}
