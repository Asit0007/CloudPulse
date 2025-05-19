#!/bin/bash

set -e  # Exit on error

if [ ! -d ".git" ]; then
  echo "❌ Please run this script from the root of the CloudPulse repo."
  exit 1
fi

OS=$(uname -s)
ARCH=$(uname -m)
if [ "$OS" != "Darwin" ] || [ "$ARCH" != "arm64" ]; then
  echo "❌ This script is for macOS ARM64 (M1/M2). Detected: $OS $ARCH"
  exit 1
fi
echo "✅ Detected macOS (M1 ARM64)"

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

check_version() {
  local cmd=$1
  local min_version=$2
  shift 2
  local version_flags=("$@")
  local version=""

  for flag in "${version_flags[@]}"; do
    if output=$($cmd $flag 2>/dev/null); then
      version=$(echo "$output" | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n 1)
      break
    fi
  done

  if [ -z "$version" ]; then
    echo "❌ Could not detect version of $cmd"
    return 1
  fi

  if [ "$(printf '%s\n' "$min_version" "$version" | sort -V | head -n1)" != "$min_version" ]; then
    echo "❌ $cmd version $version too old (required: $min_version)"
    return 1
  fi

  echo "✅ $cmd version $version OK"
}

# Install Homebrew
if ! command_exists brew; then
  echo "🔧 Installing Homebrew..."
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zshrc
  eval "$(/opt/homebrew/bin/brew shellenv)"
else
  echo "✅ Homebrew already installed"
  brew update
fi

# Define dependencies: name|min_version|version_flags(comma)|install_command
DEPENDENCIES=(
  "go|1.21|--version,version|brew install go@1.21"
  "docker|20.10|--version,version|brew install --cask docker"
  "terraform|1.5.0|version|brew tap hashicorp/tap && brew install hashicorp/tap/terraform"
  "aws|2.0|--version|curl -s https://awscli.amazonaws.com/AWSCLIV2.pkg -o AWSCLIV2.pkg && sudo installer -pkg AWSCLIV2.pkg -target / && rm AWSCLIV2.pkg"
  "node|18.0|--version,-v|brew install node@18 && echo 'export PATH=\"/opt/homebrew/opt/node@18/bin:\$PATH\"' >> ~/.zshrc && export PATH=\"/opt/homebrew/opt/node@18/bin:\$PATH\""
)

# Loop through and handle each dependency
for entry in "${DEPENDENCIES[@]}"; do
  IFS='|' read -r name min_version flags install_cmd <<< "$entry"
  IFS=',' read -ra version_flags <<< "$flags"

  if ! command_exists "$name"; then
    echo "🔧 Installing $name..."
    eval "$install_cmd"
  fi

  if ! check_version "$name" "$min_version" "${version_flags[@]}"; then
    echo "⚠️  $name version check failed after install. Please check manually."
    exit 1
  fi

  echo ""
done

# Setup Go modules
echo "📦 Setting up Go modules..."
mkdir -p backend
cd backend

if [ ! -f "go.mod" ]; then
  echo "🧰 Initializing Go module..."
  go mod init cloudpulse
fi

echo "📥 Installing Go packages..."
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/service/costexplorer
go get github.com/google/go-github/v53
go get golang.org/x/oauth2
go mod tidy
cd ..

# Final instructions
echo -e "\n🎉 All dependencies installed successfully!"
echo "👉 Next steps:"
echo "1. Run: aws configure"
echo "2. Export GITHUB_TOKEN: export GITHUB_TOKEN=<your-token>"
echo "3. Start Docker manually: open /Applications/Docker.app"
echo "4. Add code to backend/, frontend/, terraform/"
echo "5. Build: cd backend && docker build -t cloudpulse ."
echo "6. Run: docker run -p 8080:8080 -e GITHUB_TOKEN=<token> cloudpulse"
