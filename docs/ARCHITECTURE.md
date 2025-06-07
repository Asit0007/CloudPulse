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
- **Docker Hub**: Stores Docker images.
- **AWS IAM**: Manages credentials and permissions for resource access.
- **Terraform**: Automates all AWS infrastructure provisioning.

---

## Data Flow

1. User requests the dashboard in a browser.
2. Served by Go backend (containerized).
3. Backend fetches AWS usage (via AWS SDK with IAM credentials) and GitHub contributor data (via GitHub API).
4. Deployment and infrastructure managed by Terraform; app is hosted on an EC2 Instance, images pulled from Docker hub.

---

## Security and Secrets

- All secrets (GitHub PAT, AWS credentials) are handled via environment variables or GitHub Secrets.
- IAM policies are set for the least privilege.

---

> _For more details, see the main [README.md](../README.md) or review the Terraform and deployment scripts._
