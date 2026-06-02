# External services & API keys

Every key Carve depends on, where to register it, what it costs, and where it lands in our config.

After Terraform creates the **placeholder** SSM entries, set real values with:

```sh
aws ssm put-parameter \
  --name /carve/prod/<KEY_NAME> \
  --value '<real-value>' \
  --overwrite --type SecureString
```

The next ECS deploy picks up the new value (or restart tasks immediately with `aws ecs update-service --force-new-deployment`).

---

## Required — must have before first prod deploy

| # | Service | What for | Where to register | Free tier? | SSM key | Plug-in points |
|---|---|---|---|---|---|---|
| 1 | **AWS account** | Hosts everything except DNS, CDN, R2 | https://aws.amazon.com/ | 12 months free tier | n/a — uses IAM role | Root account → create `carve-admin` IAM user → use `AWS_PROFILE=carve-admin` for the `terraform apply` |
| 2 | **Cloudflare account** | DNS, CDN, R2, Pages | https://dash.cloudflare.com/sign-up | Free tier covers DNS + Pages; R2 is $0.015/GB | `CLOUDFLARE_API_TOKEN` env var (not SSM) | Add domain `carve.app` to Cloudflare → grab Zone ID and Account ID from the Overview pane → put into `terraform.tfvars` |
| 3 | **Cloudflare API token** | Lets Terraform manage DNS + R2 | Cloudflare dashboard → My Profile → API Tokens → Create Token | Free | Local env var `CLOUDFLARE_API_TOKEN`; GH secret `CLOUDFLARE_API_TOKEN` | Permissions: `Zone:DNS:Edit` on carve.app, `Account:Workers R2 Storage:Edit`, `Account:Cloudflare Pages:Edit` |
| 4 | **R2 access key** | Media service writes to R2 | Cloudflare → R2 → Manage R2 API Tokens → Create | First 10 GB/mo storage free | `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` | Created **after** Terraform creates the bucket. Scope: read+write on `carve-prod-media` bucket only |
| 5 | **Domain registration** | `carve.app` | Cloudflare Registrar or Namecheap | ~$15/yr `.app` | n/a | Point nameservers to Cloudflare |

---

## Strongly recommended — needed for normal operation

| # | Service | What for | Where to register | Cost | SSM key | Plug-in points |
|---|---|---|---|---|---|---|
| 6 | **Stripe** | Payments + Stripe Tax | https://dashboard.stripe.com/register | 2.9% + $0.30 / charge | `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Once activated, register webhook endpoint at `https://api.carve.app/v1/billing/webhook` and paste the signing secret into SSM |
| 7 | **SES production access** | Send mail to non-verified addresses | AWS SES console → "Request production access" | Free for 62k/mo from EC2; $0.10 per 1k after | Uses IAM role + auto-provisioned SMTP user | TF creates the verified domain identity, DKIM, and SMTP credentials. Manually click "Request production access" to leave the SES sandbox |
| 8 | **Sentry** | Error tracking | https://sentry.io/signup/ | Free for 5k errors/mo | `SENTRY_DSN` | Paste DSN into SSM. SDK already wired to read it |
| 9 | **PostHog** | Product analytics (self-hosted recommended past 1k MAU) | https://posthog.com/signup or self-host | Free for 1M events/mo cloud | `POSTHOG_API_KEY` | Front-end SDK reads `VITE_POSTHOG_KEY` |
| 10 | **Better Stack** | Uptime monitoring + status page | https://betterstack.com/ | Free for 10 monitors | n/a | Add HTTP monitors for `/health` on all three subdomains |
| 11 | **GitHub** | Source, CI, releases | github.com | Free public; $4/seat Team for private | n/a — uses OIDC | Add repo secrets: `AWS_DEPLOY_ROLE_ARN`, `AWS_TF_ROLE_ARN` (both from `terraform output`), `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_ZONE_ID` |

---

## Carve-specific content APIs

| # | Service | What for | Cost | SSM key | Trade-off |
|---|---|---|---|---|---|
| 12 | **DeepL** API Pro | Sentence translation in extension popup + mining | $25/M chars (Pro plan starts ~$30/mo) | `DEEPL_API_KEY` | Can defer — fall back to LLM translation (#15) or self-host Argos Translate on the NLP node |
| 13 | **Forvo** API Enterprise | Human-recorded pronunciation audio | €99/mo for 50k calls | `FORVO_API_KEY` | Can defer — fall back to Piper TTS bundled on the NLP node |
| 14 | **Google Cloud TTS** | TTS fallback when Forvo lacks the word | $4 per 1M chars | `GOOGLE_AI_API_KEY` (shared with Gemini) | Or self-host Piper. Recommended fallback |
| 15 | **LLM provider** (one of these) | In-context word translation + explanation | See cost doc — Gemini Flash cheapest | `OPENAI_API_KEY` and/or `ANTHROPIC_API_KEY` and/or `GOOGLE_AI_API_KEY` | At least one needs to be set if the LLM-lookup feature is enabled. Service picks first available, in env-var priority order |
| 16 | **JapanesePod101 / audio license** | Premium JA pronunciation (optional) | Negotiated deal | n/a | Migaku has this — Carve can ship without it. Defer indefinitely |

---

## Browser extension distribution

| # | Service | What for | Cost |
|---|---|---|---|
| 17 | **Chrome Web Store developer** | Publish Chrome extension | $5 one-time |
| 18 | **Mozilla Add-ons (AMO)** | Publish Firefox extension | Free |
| 19 | **Apple Developer Program** | Distribute Safari extension (+ code signing for native wrapper) | $99/yr |

Extension store credentials go in GH secrets, not SSM, since the CI publishes from there:
- `CHROME_EXTENSION_ID`, `CHROME_CLIENT_ID`, `CHROME_CLIENT_SECRET`, `CHROME_REFRESH_TOKEN` — see Chrome Web Store API docs
- `MOZILLA_JWT_ISSUER`, `MOZILLA_JWT_SECRET` — AMO API
- Apple App Store Connect API key for Safari — `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID`, `APPLE_API_PRIVATE_KEY` (P8 file, base64 encoded)

---

## Nice-to-have / later

| Service | What for | When to add |
|---|---|---|
| Loops / Resend | Marketing email (drip onboarding) | When you start running campaigns |
| Plain / Help Scout | Customer support inbox | When email support volume passes ~10/day |
| Vanta / Drata | SOC 2 readiness | Only if pursuing enterprise/B2B |
| Snyk | Dependency vulnerability scanning | Anytime; free tier exists |
| Crowdin / Lokalise | UI string translation | When you ship a second UI language |

---

## Where each key is wired

| Env var | Consumer service | File reference |
|---|---|---|
| `DATABASE_URL` | api | `services/api/internal/db/connect.go` |
| `REDIS_URL` | api | (currently optional; cache code gates on presence) |
| `JWT_SECRET` | api | `services/api/internal/auth/tokens.go` |
| `NLP_INTERNAL_SECRET` | api ↔ nlp | `services/nlp/src/app.py`, `services/api/internal/nlp/client.go` |
| `ALLOWED_ORIGINS` | api, media | `services/api/cmd/api/main.go`, `services/media/cmd/media/main.go` |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | api | `services/api/internal/billing/` |
| `SMTP_HOST/PORT/USER/PASSWORD/FROM` | api | `services/api/internal/reports/weekly.go` |
| `R2_*` | media | `services/media/internal/storage/s3.go` |
| `STORAGE_BACKEND` | media | `services/media/internal/storage/storage.go` |
| `DEEPL_API_KEY` | api | (wire when DeepL feature ships) |
| `FORVO_API_KEY` | api | (wire when Forvo feature ships) |
| `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GOOGLE_AI_API_KEY` | api | (wire when LLM lookup feature ships) |
| `SENTRY_DSN` | api, web | (wire when Sentry SDK installed) |
| `VITE_API_BASE`, `VITE_MEDIA_BASE`, `VITE_POSTHOG_KEY` | web (build time) | `.github/workflows/deploy-web.yml` |

Anything marked "wire when X ships" means the SSM placeholder exists so Terraform doesn't churn, but no service consumes it yet. Add the consumer code when you build the feature.
