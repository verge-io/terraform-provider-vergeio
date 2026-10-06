# One message queued to a destination.
# Changing message or webhook_url_id sends a new message.

resource "vergeio_webhook_url" "example" {
  name = "Chat alerts"
  url  = "https://example.com/hooks/chat"
}

resource "vergeio_webhook" "example" {
  webhook_url_id = vergeio_webhook_url.example.id
  message        = "{\"text\":\"lab event\"}"
}
