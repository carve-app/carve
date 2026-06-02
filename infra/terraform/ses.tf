# SES for transactional email (password reset, weekly digest, receipts).
# Sending domain is the apex; verification via Cloudflare DNS.
# New accounts start in sandbox mode (sends only to verified addresses) — file
# a production access ticket from the SES console once you want real users to
# receive mail.

resource "aws_ses_domain_identity" "main" {
  domain = var.domain
}

resource "cloudflare_record" "ses_verification" {
  zone_id = var.cloudflare_zone_id
  name    = "_amazonses"
  type    = "TXT"
  content = aws_ses_domain_identity.main.verification_token
  ttl     = 600
  proxied = false
}

resource "aws_ses_domain_identity_verification" "main" {
  domain     = aws_ses_domain_identity.main.id
  depends_on = [cloudflare_record.ses_verification]
}

# DKIM (3 CNAME records)
resource "aws_ses_domain_dkim" "main" {
  domain = aws_ses_domain_identity.main.domain
}

resource "cloudflare_record" "ses_dkim" {
  count   = 3
  zone_id = var.cloudflare_zone_id
  name    = "${aws_ses_domain_dkim.main.dkim_tokens[count.index]}._domainkey"
  type    = "CNAME"
  content = "${aws_ses_domain_dkim.main.dkim_tokens[count.index]}.dkim.amazonses.com"
  ttl     = 600
  proxied = false
}

# SMTP credentials for the API to use SES via SMTP (no AWS SDK in the API).
resource "aws_iam_user" "smtp" {
  name = "${local.name_prefix}-smtp"
}

resource "aws_iam_user_policy" "smtp" {
  user = aws_iam_user.smtp.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ses:SendRawEmail"]
      Resource = "*"
    }]
  })
}

resource "aws_iam_access_key" "smtp" {
  user = aws_iam_user.smtp.name
}

resource "aws_ssm_parameter" "smtp_user" {
  name  = "${local.ssm_prefix}/SMTP_USER"
  type  = "SecureString"
  value = aws_iam_access_key.smtp.id
}

# Overwrite the SMTP_PASSWORD placeholder with the SES SMTP password derived
# from the IAM secret access key.
resource "aws_ssm_parameter" "smtp_password" {
  name  = "${local.ssm_prefix}/SMTP_PASSWORD_REAL"
  type  = "SecureString"
  value = aws_iam_access_key.smtp.ses_smtp_password_v4
}
