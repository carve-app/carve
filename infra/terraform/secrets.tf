# Secret values live in SSM Parameter Store (SecureString). Task definitions
# reference these by ARN under the `secrets` key so the value is injected as
# an env var at task start without ever appearing in plaintext.
#
# Third-party API keys (Forvo, DeepL, OpenAI/Anthropic, Stripe, Cloudflare R2)
# are seeded with placeholder values here; rotate them out of band via:
#   aws ssm put-parameter --name /carve/prod/<key> --value '<real>' --overwrite --type SecureString

locals {
  ssm_prefix = "/${var.project}/${var.env}"
}

resource "aws_ssm_parameter" "database_url" {
  name  = "${local.ssm_prefix}/DATABASE_URL"
  type  = "SecureString"
  value = local.database_url
}

resource "aws_ssm_parameter" "redis_url" {
  name  = "${local.ssm_prefix}/REDIS_URL"
  type  = "SecureString"
  value = local.redis_url
}

resource "random_password" "jwt" {
  length  = 64
  special = false
}

resource "aws_ssm_parameter" "jwt_secret" {
  name  = "${local.ssm_prefix}/JWT_SECRET"
  type  = "SecureString"
  value = random_password.jwt.result
}

resource "random_password" "nlp_internal" {
  length  = 48
  special = false
}

resource "aws_ssm_parameter" "nlp_internal_secret" {
  name  = "${local.ssm_prefix}/NLP_INTERNAL_SECRET"
  type  = "SecureString"
  value = random_password.nlp_internal.result
}

resource "random_password" "media_internal" {
  length  = 48
  special = false
}

# Shared bearer the API presents to the media service on write endpoints.
resource "aws_ssm_parameter" "media_internal_token" {
  name  = "${local.ssm_prefix}/MEDIA_INTERNAL_TOKEN"
  type  = "SecureString"
  value = random_password.media_internal.result
}

# ── Third-party API keys (placeholders — set real values out of band) ────────

variable "placeholder_secrets" {
  description = "Names of API keys to provision as SecureString placeholders."
  type        = list(string)
  default = [
    "FORVO_API_KEY",
    "DEEPL_API_KEY",
    "OPENAI_API_KEY",
    "ANTHROPIC_API_KEY",
    "GOOGLE_AI_API_KEY",
    "STRIPE_SECRET_KEY",
    "STRIPE_WEBHOOK_SECRET",
    "R2_ACCESS_KEY_ID",
    "R2_SECRET_ACCESS_KEY",
    "SENTRY_DSN",
    "POSTHOG_API_KEY",
    # SMTP credentials are provisioned for real in ses.tf (SMTP_USER /
    # SMTP_PASSWORD_REAL), so no placeholder is needed here.
  ]
}

resource "aws_ssm_parameter" "placeholder" {
  for_each = toset(var.placeholder_secrets)
  name     = "${local.ssm_prefix}/${each.value}"
  type     = "SecureString"
  value    = "PLACEHOLDER_SET_VIA_CLI"

  lifecycle {
    # Once rotated out of band, don't let TF overwrite with the placeholder.
    ignore_changes = [value]
  }
}
