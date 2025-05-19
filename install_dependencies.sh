#!/bin/bash

# Ensure we're in the CloudPulse repository directory
if [ ! -d ".git" ]; then
  echo "Error: This script must be run from the root of the CloudPulse repository."
  exit 1
fi

# Detect operating system
OS=$(uname -s)
echo "Detected OS: $OS"

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Function to install dependencies on Linux (Ubuntu/Debian)
install_linux() {
  echo "Installing dependencies for Linux..."

  # Update package list
  sudo apt-get update

  # Install Go
  if ! command_exists go; then
    echo "Installing Go..."
    wget https://go.dev/dl/go1.21.13.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.21.13.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
    rm go1.21.13.linux-amd64.tar.gz
  else
    echo "Go is already installed: $(go version)"
  fi

  # Install Docker
  if ! command_exists docker; then
    echo "Installing Docker..."
    sudo apt-get install -y docker.io
    sudo systemctl start docker
    sudo systemctl enable docker
    sudo usermod -aG docker $USER
  else
    echo "Docker is already installed: $(docker --version)"
  fi

  # Install Terraform
  if ! command_exists terraform; then
    echo "Installing Terraform..."
    wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
    sudo apt-get update
    sudo apt-get install -y terraform=1.5.0
  else
    echo "Terraform is already installed: $(terraform version)"
  fi

  # Install AWS CLI
  if ! command_exists aws; then
    echo "Installing AWS CLI..."
    curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
    unzip awscliv2.zip
    sudo ./aws/install
    rm -rf aws awscliv2.zip
  else
    echo "AWS CLI is already installed: $(aws --version)"
  fi

  # Install Node.js (optional, for frontend tooling)
  if ! command_exists node; then
    echo "Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
    sudo apt-get install -y nodejs
  else
    echo "Node.js is already installed: $(node --version)"
  fi
}

# Function to install dependencies on macOS
install_macos() {
  echo "Installing dependencies for macOS..."

  # Install Homebrew if not present
  if ! command_exists brew; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zshrc
    eval "$(/opt/homebrew/bin/brew shellenv)"
  else
    echo "Homebrew is already installed."
  fi

  # Install Go
  if ! command_exists go; then
    echo "Installing Go..."
    brew install go@1.21
  else
    echo "Go is already installed: $(go version)"
  fi

  # Install Docker
  if ! command_exists docker; then
    echo "Installing Docker..."
    brew install --cask docker
    echo "Please start Docker Desktop manually after installation."
  else
    echo "Docker is already installed: $(docker --version)"
  fi

  # Install Terraform
  if ! command_exists terraform; then
    echo "Installing Terraform..."
    brew tap hashicorp/tap
    brew install hashicorp/tap/terraform@1.5.0
  else
    echo "Terraform is already installed: $(terraform version)"
  fi

  # Install AWS CLI
  if ! command_exists aws; then
    echo "Installing AWS CLI..."
    curl "https://awscli.amazonaws.com/AWSCLIV2.pkg" -o "AWSCLIV2.pkg"
    sudo installer -pkg AWSCLIV2.pkg -target /
    rm AWSCLIV2.pkg
  else
    echo "AWS CLI is already installed: $(aws --version)"
  fi

  # Install Node.js
  if ! command_exists node; then
    echo "Installing Node.js..."
    brew install node@18
  else
    echo "Node.js is already installed: $(node --version)"
  fi
}

# Install Go module dependencies
install_go_modules() {
  if [ -d "backend" ]; then
    echo "Installing Go module dependencies..."
    cd backend
    if [ -f "go.mod" ]; then
      go get github.com/aws/aws-sdk-go-v2/config
      go get github.com/aws/aws-sdk-go-v2/service/costexplorer
      go get github.com/google/go-github/v53
      go get golang.org/x/oauth2
      go mod tidy
      echo "Go module dependencies installed."
    else
      echo "Warning: go.mod not found in backend directory. Please initialize it with 'go mod init cloudpulse'."
    fi
    cd ..
  else
    echo "Warning: backend directory not found. Skipping Go module installation."
  fi
}

# Install dependencies based on OS
case "$OS" in
  Linux)
    install_linux
    ;;
  Darwin)
    install_macos
    ;;
  *)
    echo "Unsupported OS: $OS. This script supports Linux (Ubuntu/Debian) and macOS."
    exit 1
    ;;
esac

# Install Go modules
install_go_modules

# Final instructions
echo "All dependencies installed successfully!"
echo "Next steps:"
echo "1. Configure AWS CLI: Run 'aws configure' and provide your credentials."
echo "2. Set up GitHub Personal Access Token as an environment variable: export GITHUB_TOKEN=your-token"
echo "3. If on Linux, log out and log back in to apply Docker group changes."
echo "4. If on macOS, start Docker Desktop manually."
echo "5. Add implementation code to the files in the CloudPulse repository."