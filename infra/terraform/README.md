# Carve Phase 1 infrastructure

This stack provisions the practical alpha deployment:

- one Ubuntu VM for Docker Compose (`api`, `nlp`, `media`, Caddy)
- private AWS RDS Postgres
- Cloudflare Pages project and custom domains for the web app
- Cloudflare R2 buckets for media and database backups
- Cloudflare DNS for `api` and `media-upload`

It intentionally does not create ECS, ECR, ALB, ElastiCache/Redis, NAT gateways,
or Kubernetes.

## First apply

```sh
cd infra/terraform/bootstrap
terraform init
terraform apply
```

Copy the `backend_hcl` output into `../backend.hcl`, then:

```sh
cd ..
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars

export AWS_PROFILE=carve-admin
export CLOUDFLARE_API_TOKEN=cf_...

terraform init -backend-config=backend.hcl
terraform plan
terraform apply
```

The first apply takes roughly 15 minutes because RDS is the slow part.

## Outputs to copy

```sh
terraform output -raw phase1_host
terraform output -raw phase1_user
terraform output -raw phase1_app_dir
terraform output -raw github_terraform_role_arn
terraform output -raw r2_media_bucket_name
terraform output -raw r2_backup_bucket_name
terraform output -raw media_public_url
terraform output -raw database_url
```

Use them in `deploy/phase1/.env` and GitHub Actions secrets as described in
[`docs/deploy/phase1.md`](../../docs/deploy/phase1.md).

## Cloudflare R2 custom domain

With the pinned Cloudflare v4 provider, Terraform can create R2 buckets but not
attach a bucket custom domain. After apply, connect `media.carve.app` to the
media bucket in the Cloudflare dashboard, then set `R2_PUBLIC_BASE` to the
`media_public_url` output.

## GitHub Terraform workflow

The first apply must be local because GitHub needs an AWS role before it can run
Terraform. After the first apply, set:

- `AWS_TF_ROLE_ARN` = `terraform output -raw github_terraform_role_arn`
- `CLOUDFLARE_API_TOKEN`
- `CLOUDFLARE_ACCOUNT_ID`
- `CLOUDFLARE_ZONE_ID`
- `PHASE1_SSH_PUBLIC_KEY`

Then `.github/workflows/terraform.yml` can run plans on PRs and manual applies.
