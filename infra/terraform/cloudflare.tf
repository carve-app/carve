# Cloudflare: DNS + R2 (media bucket). Web app deploys via Cloudflare Pages
# (managed in CI, not here, because Pages needs a GitHub OAuth handshake done
# in the dashboard once).

# ── R2 bucket for media (zero-egress S3-compatible storage) ──────────────────

resource "cloudflare_r2_bucket" "media" {
  account_id = var.cloudflare_account_id
  name       = "${var.project}-${var.env}-media"
  location   = "ENAM"
}

# Custom domain for R2 is set up via the dashboard or `wrangler r2 bucket
# domain add` (no Terraform resource as of provider v4.40). The runbook
# covers the manual step: connect media.carve.app to the bucket so public
# reads route through Cloudflare with zero egress charges.

# ── DNS ──────────────────────────────────────────────────────────────────────

resource "cloudflare_record" "api" {
  zone_id = var.cloudflare_zone_id
  name    = var.api_subdomain
  type    = "CNAME"
  content = aws_lb.main.dns_name
  ttl     = 1 # auto when proxied
  proxied = true
}

# media.carve.app — once R2 custom domain is attached, this becomes a CNAME
# managed by Cloudflare automatically. Until then, point at ALB so uploads
# work via the media service's S3 SDK path.
resource "cloudflare_record" "media" {
  zone_id = var.cloudflare_zone_id
  name    = var.media_subdomain
  type    = "CNAME"
  content = aws_lb.main.dns_name
  ttl     = 1
  proxied = true
}

# www -> apex (Cloudflare Pages serves apex)
resource "cloudflare_record" "www" {
  zone_id = var.cloudflare_zone_id
  name    = "www"
  type    = "CNAME"
  content = var.domain
  ttl     = 1
  proxied = true
}

# ── Edge cache rule: media is immutable, long-cache it ───────────────────────

resource "cloudflare_ruleset" "media_cache" {
  zone_id = var.cloudflare_zone_id
  name    = "media long cache"
  kind    = "zone"
  phase   = "http_request_cache_settings"

  rules {
    action = "set_cache_settings"
    action_parameters {
      cache = true
      edge_ttl {
        mode    = "override_origin"
        default = 31536000 # 1 year
      }
    }
    expression  = "(http.host eq \"${local.media_domain}\")"
    description = "Cache media for 1 year at edge"
    enabled     = true
  }
}
