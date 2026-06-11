# Deployment docs

Phase 1 is the default production path:

- Terraform creates the VM, managed Postgres, Cloudflare Pages, Cloudflare R2,
  and DNS.
- Docker Compose on the VM runs `api`, `nlp`, `media`, and Caddy.
- GitHub Actions builds service images, pushes them to GHCR, and SSH deploys to
  the VM.

| Doc | Read when |
|---|---|
| [phase1.md](./phase1.md) | Provision, deploy, operate, or roll back Phase 1 |
| [api-keys.md](./api-keys.md) | Register or rotate external service keys |
| [runbook.md](./runbook.md) | Looking for the old runbook path |

GitHub Actions workflows:

- `.github/workflows/terraform.yml` — Terraform plan on PR, manual apply only
- `.github/workflows/deploy-phase1.yml` — backend images to GHCR, deploy to VM
- `.github/workflows/deploy-web.yml` — SvelteKit web app to Cloudflare Pages

The old ECS/Fargate/ALB/Redis stack has been removed. Scale up only when usage
asks for it.
