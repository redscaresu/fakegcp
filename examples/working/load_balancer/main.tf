# Global external HTTPS load-balancer stack against fakegcp:
#   global_address → health_check → backend_service → url_map →
#   ssl_certificate + target_https_proxy → global_forwarding_rule
#
# Each resource type maps to a fakegcp handler in handlers/loadbalancer.go.
resource "google_compute_global_address" "lb" {
  name = "lb-ip"
}

resource "google_compute_health_check" "hc" {
  name = "lb-hc"

  http_health_check {
    port         = 80
    request_path = "/"
  }
}

resource "google_compute_backend_service" "be" {
  name          = "lb-be"
  protocol      = "HTTP"
  health_checks = [google_compute_health_check.hc.id]
}

resource "google_compute_url_map" "um" {
  name            = "lb-um"
  default_service = google_compute_backend_service.be.id
}

resource "google_compute_ssl_certificate" "cert" {
  name        = "lb-cert"
  private_key = "fake-private-key"
  certificate = "fake-certificate"
}

resource "google_compute_target_https_proxy" "thp" {
  name             = "lb-thp"
  url_map          = google_compute_url_map.um.id
  ssl_certificates = [google_compute_ssl_certificate.cert.id]
}

resource "google_compute_global_forwarding_rule" "fr" {
  name       = "lb-fr"
  ip_address = google_compute_global_address.lb.id
  port_range = "443"
  target     = google_compute_target_https_proxy.thp.id
}
