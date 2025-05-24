# Configure the AWS provider with a specific version for consistency
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = "us-west-2"
}


# Create an ECR repository for the CloudPulse application
resource "aws_ecr_repository" "cloudpulse" {
  name = "cloudpulse"
  # Consider adding tags for better resource tracking, e.g., tags = { Project = "CloudPulse" }
}

# Create a VPC only if the workspace is "production"
resource "aws_vpc" "main" {
  count                = terraform.workspace == "production" ? 1 : 0
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags = {
    Name = "cloudpulse-vpc"
  }
}

# Fetch available availability zones in the region
data "aws_availability_zones" "available" {}

# Create a public subnet in the first availability zone
resource "aws_subnet" "public" {
  count                   = terraform.workspace == "production" ? 1 : 0
  vpc_id                  = aws_vpc.main[0].id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = data.aws_availability_zones.available.names[0]
  map_public_ip_on_launch = true
  tags = {
    Name = "cloudpulse-public-0"
  }
}

# Create an internet gateway for the VPC
resource "aws_internet_gateway" "main" {
  count  = terraform.workspace == "production" ? 1 : 0
  vpc_id = aws_vpc.main[0].id
  tags = {
    Name = "cloudpulse-igw"
  }
}

# Create a route table for the public subnet
resource "aws_route_table" "public" {
  count  = terraform.workspace == "production" ? 1 : 0
  vpc_id = aws_vpc.main[0].id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main[0].id
  }
  tags = {
    Name = "cloudpulse-public-rt"
  }
}

# Associate the route table with the public subnet
resource "aws_route_table_association" "public" {
  count          = terraform.workspace == "production" ? 1 : 0
  subnet_id      = aws_subnet.public[0].id
  route_table_id = aws_route_table.public[0].id

# Create a security group for the EC2 instance
# NOTE: For production, consider restricting ingress to specific IP ranges instead of 0.0.0.0/0 for enhanced security
resource "aws_security_group" "ec2_sg" {
  count  = terraform.workspace == "production" ? 1 : 0
  vpc_id = aws_vpc.main[0].id
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]  # SSH access from anywhere
  }
  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]  # Application port
  }
  ingress {
    from_port   = 8200
    to_port     = 8200
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]  # Vault port
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"           # Allow all outbound traffic
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = {
    Name = "cloudpulse-ec2-sg"
  }
}

# Create an EC2 instance with Docker, Vault, and the application
resource "aws_instance" "cloudpulse_instance" {
  count                       = terraform.workspace == "production" ? 1 : 0
  ami                         = data.aws_ami.amazon_linux_2.id
  instance_type               = "t3.micro"
  subnet_id                   = aws_subnet.public[0].id
  vpc_security_group_ids      = [aws_security_group.ec2_sg[0].id]
  associate_public_ip_address = true

  user_data = <<-EOF
              #!/bin/bash
              # Install Docker
              yum update -y
              amazon-linux-extras install docker -y
              systemctl start docker
              systemctl enable docker
              usermod -a -G docker ec2-user

              # Install Docker Compose
              curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
              chmod +x /usr/local/bin/docker-compose

              # Create directory for Docker Compose
              mkdir -p /home/ec2-user/cloudpulse
              cd /home/ec2-user/cloudpulse

              # Create docker-compose.yml with Vault configuration mounted
              cat << 'DOCKER_COMPOSE' > docker-compose.yml
              version: '3.8'
              services:
                vault:
                  image: hashicorp/vault:1.15
                  ports:
                    - "8200:8200"
                  environment:
                    - VAULT_ADDR=http://0.0.0.0:8200
                  volumes:
                    - vault-data:/vault/file
                    - vault-logs:/var/log
                    - /home/ec2-user/cloudpulse/vault-config:/vault/config  # Mount config directory
                  cap_add:
                    - IPC_LOCK
                  command: server -config=/vault/config/vault.hcl  # Use the provided config file
                app:
                  image: ${aws_ecr_repository.cloudpulse.repository_url}:latest
                  ports:
                    - "8080:8080"
                  environment:
                    - STAGE=production
                    - AWS_REGION=us-east-1
                    - LOG_LEVEL=info
                    - PORT=8080
                    - VAULT_ADDR=http://vault:8200
                    - VAULT_TOKEN=${var.vault_token}  # NOTE: This might not match the generated root token
              volumes:
                vault-data:
                vault-logs:
              DOCKER_COMPOSE

              # Create Vault configuration file
              mkdir -p /home/ec2-user/cloudpulse/vault-config
              cat << 'VAULT_CONFIG' > vault-config/vault.hcl
              ui = true

              listener "tcp" {
                address     = "0.0.0.0:8200"
                tls_disable = true  # Consider enabling TLS in production
              }

              storage "file" {
                path = "/vault/file"
              }
              VAULT_CONFIG

              # Start containers
              export VAULT_ADDR=http://127.0.0.1:8200
              docker-compose up -d

              # Initialize Vault
              sleep 10  # Wait for Vault to start
              docker-compose exec -T vault vault operator init > /home/ec2-user/vault-init.txt
              VAULT_TOKEN=$(grep 'Initial Root Token' /home/ec2-user/vault-init.txt | awk '{print $4}')
              echo "VAULT_TOKEN=$VAULT_TOKEN" >> /home/ec2-user/.bashrc

              # Unseal Vault with the first three keys
              for i in {1..3}; do
                KEY=$(grep "Unseal Key $i" /home/ec2-user/vault-init.txt | awk '{print $4}')
                docker-compose exec -T vault vault operator unseal $KEY
              done

              # Set up Vault secrets using the root token
              docker-compose exec -T -e VAULT_TOKEN=$VAULT_TOKEN vault vault secrets enable -path=secret kv
              echo "After Vault is running, add the secrets using the below commands:"
              echo  "docker-compose exec -T -e VAULT_TOKEN=$VAULT_TOKEN vault vault kv put secret/cloudpulse/production/GITHUB_TOKEN value=${var.github_token}"
              echo "docker-compose exec -T -e VAULT_TOKEN=$VAULT_TOKEN vault vault kv put secret/cloudpulse/production/AWS_ACCESS_KEY_ID value=${var.aws_access_key_id}"
              echo "docker-compose exec -T -e VAULT_TOKEN=$VAULT_TOKEN vault vault kv put secret/cloudpulse/production/AWS_SECRET_ACCESS_KEY value=${var.aws_secret_access_key}"
              EOF

  tags = {
    Name = "cloudpulse-instance"
  }
}

# Data source for the latest Amazon Linux 2 AMI
data "aws_ami" "amazon_linux_2" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn2-ami-hvm-*-x86_64-gp2"]
  }
}

# Output the public IP of the instance if in production
output "instance_public_ip" {
  value = terraform.workspace == "production" ? aws_instance.cloudpulse_instance[0].public_ip : "N/A"
}

# NOTE: The 'app' service in docker-compose.yml starts with VAULT_TOKEN set to var.vault_token, which may not match
# the root token generated during Vault initialization. This could prevent the app from connecting to Vault initially.
# Consider starting the app service after Vault initialization or adjusting the app to dynamically retrieve the token.