# CloudPulse

CloudPulse is a lightweight web application that provides a real-time dashboard to monitor AWS resource usage, Free Tier limits, and GitHub repository contributors. Built with a Go backend, HTML/CSS/JavaScript frontend, and deployed on AWS ECS Fargate using Terraform, it ensures automation, scalability, and adherence to AWS Free Tier constraints. The dashboard refreshes periodically, and a fallback webpage displays during downtime to indicate when it will be live again.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Project Structure](#project-structure)
- [Setup Instructions](#setup-instructions)
- [Local Development](#local-development)
- [Deployment to AWS](#deployment-to-aws)
- [Usage](#usage)
- [Contributing](#contributing)
- [License](#license)

## Prerequisites
To develop and deploy CloudPulse, ensure you have the following installed:
- **Go (1.21+)**: For the backend server.
- **Docker**: For containerizing the application.
- **Terraform (1.5.0+)**: For infrastructure automation.
- **AWS CLI**: For interacting with AWS services.
- **Node.js (18+)**: Optional, for frontend tooling.
- **Git**: For version control.
- **GitHub Personal Access Token**: With \`repo\` scope for accessing contributor data.
- **AWS Account**: Within the Free Tier, with IAM credentials configured.

## Project Structure
\`\`\`
CloudPulse/
├── backend/
│   ├── main.go          # Go backend server
│   ├── go.mod          # Go module dependencies
│   ├── go.sum          # Go dependency checksums
│   └── Dockerfile      # Docker configuration for the backend
├── frontend/
│   ├── index.html      # Main dashboard page
│   ├── styles.css      # CSS styles for the dashboard
│   ├── script.js       # JavaScript for dynamic content
│   └── offline.html    # Fallback page for downtime
├── terraform/
│   ├── main.tf         # Terraform configuration for AWS infrastructure
│   └── variables.tf    # Terraform variables
└── .github/
    └── workflows/
        └── deploy.yml      # GitHub Actions workflow for CI/CD
\`\`\`

## Setup Instructions
1. **Clone the Repository**:
   \`\`\`bash
   git clone https://github.com/your-username/CloudPulse.git
   cd CloudPulse
   \`\`\`

2. **Install Dependencies**:
   Run the provided script to install Go, Docker, Terraform, AWS CLI, and Node.js:
   \`\`\`bash
   chmod +x install_dependencies.sh
   ./install_dependencies.sh
   \`\`\`

3. **Configure AWS CLI**:
   \`\`\`bash
   aws configure
   \`\`\`
   Provide your AWS Access Key ID, Secret Access Key, region (\`us-east-1\`), and output format (\`json\`).

4. **Set GitHub Token**:
   \`\`\`bash
   export GITHUB_TOKEN=your-github-token
   \`\`\`
   Replace \`your-github-token\` with your GitHub Personal Access Token.

5. **Create Project Structure** (if not already done):
   \`\`\`bash
   chmod +x setup_cloudpulse_structure.sh
   ./setup_cloudpulse_structure.sh
   \`\`\`

6. **Add Implementation Code**:
   - Populate the files in \`backend/\`, \`frontend/\`, and \`terraform/\` with the implementation code (refer to project documentation or artifacts for details).
   - Update \`backend/main.go\` to use the correct frontend path (\`http.FileServer(http.Dir("frontend"))\`).

## Local Development
1. **Build and Run the Docker Container**:
   \`\`\`bash
   cd backend
   docker build -t cloudpulse .
   docker run -p 8080:8080 -e GITHUB_TOKEN=your-github-token cloudpulse
   \`\`\`
2. Open \`http://localhost:8080\` in a browser to view the dashboard.

## Deployment to AWS
1. **Set Up AWS Budgets**:
   - In the AWS Console, create a \$0 cost budget with an alert at \$0.01 to monitor Free Tier usage.
2. **Push Docker Image to ECR**:
   \`\`\`bash
   aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <your-account-id>.dkr.ecr.us-east-1.amazonaws.com
   docker tag cloudpulse:latest <your-account-id>.dkr.ecr.us-east-1.amazonaws.com/cloudpulse:latest
   docker push <your-account-id>.dkr.ecr.us-east-1.amazonaws.com/cloudpulse:latest
   \`\`\`
3. **Deploy with Terraform**:
   \`\`\`bash
   cd terraform
   terraform init
   terraform apply -var="github_token=your-github-token"
   \`\`\`
4. **Access the Dashboard**:
   - Retrieve the ECS service public IP from the AWS Console (ECS > Clusters > cloudpulse-cluster > Services > cloudpulse-service > Tasks).
   - Alternatively, configure a domain with Route 53.
5. **Offline Page**:
   - Upload \`frontend/offline.html\` to an S3 bucket and configure it as a static website:
     \`\`\`bash
     aws s3 cp frontend/offline.html s3://cloudpulse-bucket/
     aws s3 website s3://cloudpulse-bucket/ --index-document offline.html
     \`\`\`

## Usage
*To be added after implementation code is complete.*
- The dashboard displays AWS resource usage, Free Tier limits, and GitHub contributors.
- It refreshes every minute and is accessible at the ECS public IP or configured domain.

## Contributing
Contributions are welcome! Please:
1. Fork the repository.
2. Create a feature branch (\`git checkout -b feature/your-feature\`).
3. Commit changes (\`git commit -m "Add your feature"\`).
4. Push to the branch (\`git push origin feature/your-feature\`).
5. Open a pull request.

## License
This project is licensed under the MIT License.