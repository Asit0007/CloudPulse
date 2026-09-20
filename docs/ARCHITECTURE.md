# CloudPulse Architecture

## Overview

CloudPulse’s architecture is designed for simplicity and quick deployment using AWS EC2 Instance, Docker, and Terraform.  
Below is the architectural diagram (with official logos) and a description of each component.

![CloudPulse Architecture](assets/cloudpulse-architecture.svg)

---

## Component Breakdown

- **User Browser**: Users access the dashboard via HTTP.
- **Frontend (HTML/CSS/JS)**: The web dashboard, served by the Go backend.
- **Go Backend (API server)**: Provides dashboard data, fetches AWS resource usage, and GitHub contributor stats.
- **Docker**: Used to containerize the Go backend and frontend.
- **GitHub API**: Backend fetches contributor data using a Personal Access Token.
- **AWS EC2 Instance**: Runs the Docker container in the cloud.
- **AWS CloudWatch**: Source of all EC2 metrics — the `AWS/EC2` namespace for CPU and
  network, and the `CWAgent` namespace for memory (requires the CloudWatch Agent).
- **HashiCorp Vault**: Runtime secret store, running as a second container on the same
  instance. The backend reads the GitHub PAT from it at boot (KV v2, `kv/cloudpulse`).
- **Docker Hub**: Stores Docker images.
- **AWS IAM**: Provides the instance profile the backend uses to authenticate to CloudWatch.
- **Terraform**: Automates all AWS infrastructure provisioning.

---

## Data Flow

1. At startup, the backend reads the GitHub PAT from Vault and resolves its own instance
   ID from the EC2 metadata service. It exits if Vault is unreachable.
2. User requests the dashboard in a browser.
3. Served by Go backend (containerized) — the same process serves both the static frontend
   and the JSON API on one port.
4. Backend fetches AWS usage (via AWS SDK, authenticated by the IAM instance profile) and
   GitHub contributor data (via GitHub API). Each request is a live passthrough query;
   nothing is cached or stored.
5. Deployment and infrastructure managed by Terraform; app is hosted on an EC2 Instance,
   images pulled from Docker Hub.

---

## Security and Secrets

- **The GitHub PAT is not an environment variable.** It is fetched at runtime from Vault
  (KV v2, path `kv/cloudpulse`, key `github_token`) using a policy-scoped token. Only
  non-secret configuration — `GITHUB_OWNER`, `GITHUB_REPO`, `AWS_REGION`, `PORT` — is
  passed through the environment.
- **AWS credentials are never handled by the application.** They resolve through the SDK
  default credential chain, which on EC2 means the IAM instance profile: no long-lived
  access keys exist on the host.
- CI/CD credentials (Docker Hub, SSH key, Vault token) are stored as GitHub Secrets.
- **The IAM policy is not yet least-privilege.** The backend calls only
  `cloudwatch:GetMetricData` and `cloudwatch:GetMetricStatistics`, but the policy in
  `terraform/main.tf` grants eleven actions on `Resource = "*"` — including the write
  action `cloudwatch:PutMetricData`, five `logs:*` actions, and two `ec2:Describe*`
  actions that no code path uses. Narrowing this to the two required read actions is
  tracked in the README roadmap.
- `VAULT_TOKEN` is passed to the container as a plain environment variable, so anyone with
  shell access to the host can read it via `docker inspect`. Migrating to the Vault AWS
  auth method would remove this static token entirely.
- **`/api/*` is unauthenticated.** Anyone who can reach the listener reads this
  deployment's CloudWatch and Cost Explorer figures. Deliberate for a public build-log
  dashboard, but it is why handler errors are no longer echoed to the caller: an AWS
  authorization failure spells out the account ID, the IAM principal and the missing
  permission. `apiError` in `backend/main.go` logs the real error and returns only the
  stage name. CORS is likewise off unless `CORS_ALLOW_ORIGIN` is set — it used to be `*`
  unconditionally, which let any page on the internet read those figures out of a
  visitor's browser.

### Network posture of the decommissioned deployment

The AWS infrastructure was torn down on 2026-08-19. Recording what it looked like,
because the Terraform that built it is still in this repository's history and should not
be re-applied as written:

- The EC2 security group opened **22, 8080 and 8200 to `0.0.0.0/0`**.
- `vault/vault-config.hcl` ran the Vault listener on `0.0.0.0:8200` with
  **`tls_disable = true`** and `storage "inmem"`. Combined with the security group, that
  is an unauthenticated-at-the-network-edge secret store reachable over plaintext HTTP
  from anywhere, holding the GitHub PAT.
- `terraform.tfstate` for the production workspace was committed (later removed, still
  reachable in history), publishing the AWS account ID, VPC/subnet/SG/instance IDs and
  the public IP.

If this is ever redeployed: scope 22 to a known address or drop it for SSM, keep 8200
off the internet entirely, terminate TLS in front of 8080, and add
`*.tfstate`/`tfplan` to `.gitignore` **before** the first `terraform apply`.

---

> _For more details, see the main [README.md](../README.md) or review the Terraform and deployment scripts._
