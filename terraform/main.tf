terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0" # Specify a compatible version
    }
  }
  required_version = ">= 1.0" # Specify a compatible Terraform version
}

provider "aws" {
  region = var.aws_region
}

# --- Networking ---
# Using default VPC and subnets for simplicity to stay within Free Tier.
# If you need a custom VPC, ensure it's configured correctly.

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# --- Security Group ---
resource "aws_security_group" "cloudpulse_sg" {
  name        = "${var.project_name}-sg"
  description = "Allow HTTP, HTTPS, and SSH for CloudPulse"
  vpc_id      = data.aws_vpc.default.id // Use default VPC

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_access_cidr] # Restrict SSH access to your IP
  }

  ingress {
    from_port   = 80 # For HTTP access to the application
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443 # For future HTTPS
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  
  # Allow Vault access from the EC2 instance itself (for the app container)
  # and potentially from your SSH IP for initial setup/management.
  ingress {
    description = "Allow Vault access from within EC2 and trusted SSH IP"
    from_port   = 8201
    to_port     = 8201
    protocol    = "tcp"
    cidr_blocks = [var.ssh_access_cidr] # Your IP for management
    # self = true # This would allow traffic from instances within this SG.
    # For container-to-container on same host, Docker networking is used.
    # This rule is more for accessing Vault UI from your trusted IP.
  }

    ingress {
    description = "Vault cluster communications (port 8202) within the SG"
    from_port       = 8202
    to_port         = 8202
    protocol        = "tcp"
    security_groups = [aws_security_group.cloudpulse_sg.id]
  }


  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1" # Allow all outbound traffic
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name        = "${var.project_name}-sg"
    Project     = var.project_name
    Environment = var.environment
  }
}

# --- IAM Role and Policy for EC2 ---
resource "aws_iam_role" "cloudpulse_ec2_role" {
  name = "${var.project_name}-ec2-role-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })

  tags = {
    Name    = "${var.project_name}-ec2-role-${var.environment}"
    Project = var.project_name
  }
}

resource "aws_iam_policy" "cloudpulse_ec2_policy" {
  name        = "${var.project_name}-ec2-policy-${var.environment}"
  description = "Policy for CloudPulse EC2 to access CloudWatch and other necessary services."

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "cloudwatch:GetMetricData",
          "cloudwatch:ListMetrics" // Useful for diagnostics
          // Add other permissions if needed, e.g., for EC2 metadata service access (usually allowed by default)
        ]
        Effect   = "Allow"
        Resource = "*" // CloudWatch actions often require '*' or specific metric ARNs
      },
      // Potentially add permissions for Systems Manager Session Manager if you prefer it over SSH
      // {
      //   "Effect": "Allow",
      //   "Action": [
      //       "ssm:UpdateInstanceInformation",
      //       "ssmmessages:CreateControlChannel",
      //       "ssmmessages:CreateDataChannel",
      //       "ssmmessages:OpenControlChannel",
      //       "ssmmessages:OpenDataChannel"
      //   ],
      //   "Resource": "*"
      // }
    ]
  })

  tags = {
    Name    = "${var.project_name}-ec2-policy-${var.environment}"
    Project = var.project_name
  }
}

resource "aws_iam_role_policy_attachment" "cloudpulse_ec2_policy_attach" {
  role       = aws_iam_role.cloudpulse_ec2_role.name
  policy_arn = aws_iam_policy.cloudpulse_ec2_policy.arn
}

resource "aws_iam_instance_profile" "cloudpulse_ec2_instance_profile" {
  name = "${var.project_name}-ec2-profile-${var.environment}"
  role = aws_iam_role.cloudpulse_ec2_role.name

  tags = {
    Name    = "${var.project_name}-ec2-profile-${var.environment}"
    Project = var.project_name
  }
}

# --- EC2 Instance ---
# Find the latest Amazon Linux 2 AMI
data "aws_ami" "amazon_linux_2" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn2-ami-hvm-*-x86_64-gp2"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_instance" "cloudpulse_server" {
  ami           = data.aws_ami.amazon_linux_2.id
  instance_type = var.ec2_instance_type # Should be "t3.micro" for Free Tier

  # Use one of the default subnets. For more control, specify a subnet_id.
  # Ensure the chosen subnet is in an AZ that supports t3.micro.
  subnet_id = data.aws_subnets.default.ids[0]

  vpc_security_group_ids = [aws_security_group.cloudpulse_sg.id]
  iam_instance_profile   = aws_iam_instance_profile.cloudpulse_ec2_instance_profile.name
  key_name               = var.ec2_key_name # Name of your EC2 Key Pair for SSH

  # User data to install Docker, Docker Compose and other utilities
  user_data = <<-EOF
              #!/bin/bash
              sudo yum update -y
              sudo amazon-linux-extras install docker -y
              sudo systemctl start docker
              sudo systemctl enable docker
              sudo usermod -a -G docker ec2-user

              # Install Docker Compose (latest version)
              sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
              sudo chmod +x /usr/local/bin/docker-compose
              
              # Install Git (useful for pulling helper scripts or manual setup)
              sudo yum install -y git

              # You might want to install the Vault CLI for easier management from the EC2 instance itself
              # sudo yum install -y yum-utils
              # sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/AmazonLinux/hashicorp.repo
              # sudo yum -y install vault

              echo "User data script completed." >> /tmp/user_data.log
              EOF

  tags = {
    Name        = "${var.project_name}-server-${var.environment}"
    Project     = var.project_name
    Environment = var.environment
  }

  # Ensure root block device is within Free Tier (30GB of EBS general purpose (SSD) or magnetic storage)
  root_block_device {
    volume_size = var.ebs_volume_size # e.g., 20 GB, well within 30GB free tier
    volume_type = "gp3"             # gp3 is often cheaper and better performing than gp2
    delete_on_termination = true
  }

  # Enable detailed monitoring if desired, but basic monitoring is free.
  # monitoring = false # Basic monitoring is free. Detailed costs extra.
}

# --- Outputs ---
output "instance_public_ip" {
  description = "Public IP address of the CloudPulse EC2 instance."
  value       = aws_instance.cloudpulse_server.public_ip
}

output "instance_id" {
  description = "ID of the CloudPulse EC2 instance."
  value       = aws_instance.cloudpulse_server.id
}

output "security_group_id" {
  description = "ID of the CloudPulse security group."
  value       = aws_security_group.cloudpulse_sg.id
}
