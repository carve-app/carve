# External Services And Keys

Phase 1 stores runtime application values in `/opt/carve/phase1/.env` on the VM.
GitHub-only values live in GitHub repository or `prod` environment secrets.

## Required

| Service | What For | Where It Lands |
|---|---|---|
| AWS account | EC2 VM, RDS Postgres, Terraform state | local AWS profile; `AWS_TF_ROLE_ARN` after first apply |
| Cloudflare account | DNS, Pages, R2 | Terraform vars/secrets |
| Cloudflare API token | Terraform manages DNS, Pages, R2 | `CLOUDFLARE_API_TOKEN` |
| R2 access key | Media service writes screenshots/audio | `.env`: `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` |
| SSH deploy key | GitHub deploys to the VM | Terraform `ssh_public_key`, GitHub `PHASE1_SSH_KEY` |

Cloudflare API token permissions:

- `Zone:DNS:Edit` on `carve.app`
- `Account:Workers R2 Storage:Edit`
- `Account:Cloudflare Pages:Edit`

R2 access keys are created after Terraform creates the bucket:
Cloudflare -> R2 -> Manage R2 API Tokens -> Create API token. Scope it to
read/write on `carve-prod-media`.

## Strongly Recommended

| Service | What For | Env Var / Secret |
|---|---|---|
| Better Stack or similar | Uptime monitoring | monitor `https://api.carve.app/health` |
| SMTP provider | Weekly digest and account mail | `.env`: `SMTP_HOST`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` |
| Sentry | Error tracking | `.env`: `SENTRY_DSN` once SDKs are wired |
| Stripe | Payments | `.env`: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` once billing is enabled |

## Content And AI

| Service | What For | Env Var |
|---|---|---|
| Google Cloud TTS / Gemini | TTS or AI fallback | `GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_AI_API_KEY` |
| Anthropic | LLM lookup/explanations | `ANTHROPIC_API_KEY` |
| STT backend | Speaking/shadowing checks | `STT_BACKEND_URL`, `STT_BACKEND_KEY` |

Only add paid providers when the feature that consumes them is actually live.

## Browser Extension Distribution

Extension store credentials belong in GitHub secrets, not `.env`:

- Chrome: `CHROME_EXTENSION_ID`, `CHROME_CLIENT_ID`,
  `CHROME_CLIENT_SECRET`, `CHROME_REFRESH_TOKEN`
- Firefox AMO: `MOZILLA_JWT_ISSUER`, `MOZILLA_JWT_SECRET`
- Safari/App Store Connect: `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID`,
  `APPLE_API_PRIVATE_KEY`

## Runtime Env Map

| Env Var | Consumer |
|---|---|
| `DATABASE_URL` | API |
| `JWT_SECRET` | API |
| `NLP_INTERNAL_SECRET` | API and NLP |
| `MEDIA_INTERNAL_TOKEN` | API and media |
| `ALLOWED_ORIGINS` | API |
| `R2_*` | media |
| `STORAGE_BACKEND` | media |
| `SMTP_*` | API weekly reports |
| `VITE_API_BASE`, `VITE_MEDIA_BASE` | web build workflow |
