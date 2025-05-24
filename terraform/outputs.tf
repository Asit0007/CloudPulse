# Output the URL of the ECR repository for CloudPulse
output "ecr_repository_url" {
  description = "The URL of the ECR repository used by CloudPulse"
  value       = aws_ecr_repository.cloudpulse.repository_url
}