# variables.tf: Terraform variables for CloudPulse


# Define the AWS region to be used for resources
variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "The AWS region where resources will be created. Defaults to us-east-1."
}

# Define the AWS account ID
variable "aws_account_id" {
  type        = string
  description = "The AWS account ID to be used for authentication and resource creation."
}

# Define the Vault token for accessing secrets
variable "vault_token" {
  type        = string
  description = "The token used to authenticate with HashiCorp Vault."
  sensitive   = true # Mark as sensitive to prevent accidental exposure
}

# Define the GitHub token for API access
variable "github_token" {
  type        = string
  description = "The token used to authenticate with the GitHub API."
  sensitive   = true # Mark as sensitive to prevent accidental exposure
}

# Define the AWS access key ID
variable "aws_access_key_id" {
  type        = string
  description = "The AWS access key ID for authentication."
  sensitive   = true # Mark as sensitive to prevent accidental exposure
}

# Define the AWS secret access key
variable "aws_secret_access_key" {
  type        = string
  description = "The AWS secret access key for authentication."
  sensitive   = true # Mark as sensitive to prevent accidental exposure
}