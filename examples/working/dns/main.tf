# DNS managed zone + record set. Tests the zone -> record set FK
# chain that fakegcp's TestDNSRecordSetFKViolation +
# TestDNSZoneDeleteWithRecords handler tests assert.

resource "google_dns_managed_zone" "main" {
  name        = "test-zone"
  dns_name    = "test.example.invalid."
  description = "fakegcp test zone"
}

resource "google_dns_record_set" "main" {
  name         = "host.${google_dns_managed_zone.main.dns_name}"
  managed_zone = google_dns_managed_zone.main.name
  type         = "A"
  ttl          = 300
  rrdatas      = ["192.0.2.10"]
}
