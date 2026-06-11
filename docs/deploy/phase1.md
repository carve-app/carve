# Phase 1 deployment

Practical alpha target:

- Cloudflare Pages serves `https://carve.app`.
- One AWS EC2 VM runs `api`, `nlp`, `media`, and Caddy via Docker Compose.
- AWS RDS Postgres stores application data.
- Cloudflare R2 stores screenshots/audio and backup dumps.
- No Redis, Kubernetes, ECS, ECR, ALB, or NAT gateway.

Terraform owns long-lived infrastructure. The scripts in
[`deploy/phase1/`](../../deploy/phase1/) own app releases, migrations, backups,
and rollbacks.

## Provision

Create or reuse the Terraform state backend:

```sh
cd infra/terraform/bootstrap
terraform init
terraform apply
```

Copy the `backend_hcl` output into `infra/terraform/backend.hcl`, then:

```sh
cd ../
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars

export AWS_PROFILE=carve-admin
export CLOUDFLARE_API_TOKEN=cf_...

terraform init -backend-config=backend.hcl
terraform plan
terraform apply
```

Terraform creates:

- EC2 VM with Docker installed by cloud-init
- private RDS Postgres
- Cloudflare Pages project and `carve.app` / `www.carve.app` domains
- Cloudflare R2 media and backup buckets
- Cloudflare DNS for `api.carve.app` and `media-upload.carve.app`

If `carve-web` or the R2 buckets already exist in Cloudflare, import them before
`terraform apply` instead of letting Terraform try to recreate them:

```sh
terraform import cloudflare_pages_project.web '<account_id>/carve-web'
terraform import cloudflare_pages_domain.apex '<account_id>/carve-web/carve.app'
terraform import cloudflare_pages_domain.www '<account_id>/carve-web/www.carve.app'
terraform import cloudflare_r2_bucket.media '<account_id>/carve-prod-media'
terraform import cloudflare_r2_bucket.backups '<account_id>/carve-prod-backups'
```

If matching DNS records already exist, either import those records by Cloudflare
record ID or delete them in the dashboard before apply.

Copy these outputs:

```sh
terraform output -raw phase1_host
terraform output -raw phase1_user
terraform output -raw phase1_app_dir
terraform output -raw database_url
terraform output -raw r2_media_bucket_name
terraform output -raw r2_backup_bucket_name
terraform output -raw media_public_url
terraform output -raw github_terraform_role_arn
```

## One Cloudflare Gap

With the pinned Cloudflare v4 Terraform provider, Terraform creates R2 buckets
but cannot attach an R2 custom domain. Do this once:

Cloudflare dashboard -> R2 -> `carve-prod-media` -> Settings -> Custom Domains
-> connect `media.carve.app`.

Then set `R2_PUBLIC_BASE` to `terraform output -raw media_public_url`.

## GitHub Secrets

Set these repository or `prod` environment secrets:

| Secret | Source |
|---|---|
| `PHASE1_HOST` | `terraform output -raw phase1_host` |
| `PHASE1_USER` | `terraform output -raw phase1_user` |
| `PHASE1_SSH_KEY` | Private key matching `ssh_public_key` in Terraform |
| `PHASE1_SSH_PUBLIC_KEY` | Public deploy key for Terraform CI plans |
| `PHASE1_APP_DIR` | Optional; default `/opt/carve/phase1` |
| `AWS_TF_ROLE_ARN` | `terraform output -raw github_terraform_role_arn` |
| `CLOUDFLARE_API_TOKEN` | Cloudflare token with DNS, Pages, and R2 permissions |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare dashboard |
| `CLOUDFLARE_ZONE_ID` | Cloudflare dashboard |
| `GHCR_READ_TOKEN` | Private repos only; PAT with package read access |

The first Terraform apply is local. After that, `.github/workflows/terraform.yml`
can run PR plans and manual applies.

## First Deploy

Run **Deploy Phase 1 VM** from GitHub Actions. The first run copies the deploy
bundle, creates `/opt/carve/phase1/.env`, and stops.

SSH to the VM and fill in `.env`:

```sh
cd /opt/carve/phase1
$EDITOR .env
```

Required values:

- `DATABASE_URL` = `terraform output -raw database_url`
- `R2_ACCOUNT_ID`
- `R2_BUCKET` = `terraform output -raw r2_media_bucket_name`
- `R2_PUBLIC_BASE` = `terraform output -raw media_public_url`
- `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY`
- optional Google, SMTP, AI, or STT keys

Keep `COMPOSE_PROFILES=` for managed Postgres. Set `COMPOSE_PROFILES=local-db`
only for a temporary local database fallback.

Rerun **Deploy Phase 1 VM**. It builds images, pushes to GHCR, writes
`release.env`, runs migrations, and starts services.

Check health:

```sh
curl -fsS https://api.carve.app/health
curl -fsS https://media-upload.carve.app/health
```

## Day-2 Commands

Deploy the current `release.env` from the VM:

```sh
cd /opt/carve/phase1
./scripts/deploy.sh
```

Tail logs:

```sh
docker compose --env-file .env --env-file release.env -f docker-compose.yml logs -f api
docker compose --env-file .env --env-file release.env -f docker-compose.yml logs -f nlp
docker compose --env-file .env --env-file release.env -f docker-compose.yml logs -f media
```

## Backups

RDS automated backups/PITR are enabled by Terraform. Keep logical dumps too:

```cron
15 2 * * * cd /opt/carve/phase1 && ./scripts/backup-postgres.sh >> /var/log/carve-postgres-backup.log 2>&1
```

Set `R2_BACKUP_BUCKET` to `terraform output -raw r2_backup_bucket_name` to upload
compressed dumps and `.sha256` files to R2.

Restore into an empty database:

```sh
cd /opt/carve/phase1
CONFIRM_RESTORE=1 ./scripts/restore-postgres.sh backups/postgres/carve-postgres-YYYYMMDDTHHMMSSZ.sql.gz
```

Do a restore drill before trusting the backup story.

## Rollback

Set `IMAGE_TAG` in `/opt/carve/phase1/release.env` to a previous image tag, then:

```sh
./scripts/deploy.sh
```

Schema rollbacks are not automatic. Avoid destructive migrations until there is
a tested rollback policy.

## When To Leave Phase 1

Stay here until one of these is clearly true:

- NLP CPU or memory contention is visible.
- RDS needs a larger class, read replica, or multi-AZ.
- Deploys need high availability.
- Traffic justifies two or more API instances.

The next step is probably ECS/Fargate or two VMs. It is still not Kubernetes.
