# Deployment docs

| Doc | Read when |
|---|---|
| [api-keys.md](./api-keys.md) | Need to register an external service or rotate a key |
| [runbook.md](./runbook.md) | Bootstrapping a new environment, doing day-2 ops, or breaking glass |

Infrastructure-as-code lives in [`/infra/terraform/`](../../infra/terraform/) — see its README for layout and scaling levers.

GitHub Actions workflows for deploy:
- `.github/workflows/deploy.yml` — backend services (api, nlp, media) → ECR + ECS
- `.github/workflows/deploy-web.yml` — SvelteKit web app → Cloudflare Pages
- `.github/workflows/terraform.yml` — Terraform plan on PR, apply on merge to main
