variable "aws_region" {
  description = "AWS region to deploy resources."
  type        = string
  default     = "us-east-1" # Choose a region that supports t3.micro and is convenient for you
}

variable "project_name" {
  description = "A name for the project, used for tagging and naming resources."
  type        = string
  default     = "CloudPulse"
}

variable "environment" {
  description = "Deployment environment (e.g., dev, prod)."
  type        = string
  default     = "dev"
}

variable "ec2_instance_type" {
  description = "EC2 instance type for the application server."
  type        = string
  default     = "t3.micro" # CRITICAL: Must be t2.micro or t3.micro for AWS Free Tier eligibility
  validation {
    condition     = contains(["t2.micro", "t3.micro"], var.ec2_instance_type)
    error_message = "Instance type must be t2.micro or t3.micro to stay within AWS Free Tier."
  }
}

variable "ec2_key_name" {
  description = "Name of the EC2 Key Pair to use for SSH access to the instance."
  type        = string
  # No default, should be provided by the user.
  # Example: "my-ec2-keypair"
  # Ensure this key pair exists in your AWS account in the selected region.
}

variable "ssh_access_cidr" {
  description = "CIDR block allowed for SSH access to the EC2 instance. Should be your IP address."
  type        = string
  default     = "0.0.0.0/0" # WARNING: This is insecure. Replace with your specific IP: "YOUR_IP/32"
  # To find your IP, search "what is my IP" in Google.
}

variable "ebs_volume_size" {
  description = "Size of the EBS root volume in GB for the EC2 instance."
  type        = number
  default     = 20 # AWS Free Tier includes up to 30GB of EBS storage.
  validation {
    condition     = var.ebs_volume_size <= 30 && var.ebs_volume_size >= 8
    error_message = "EBS volume size must be between 8GB and 30GB to stay within AWS Free Tier limits."
  }
}
