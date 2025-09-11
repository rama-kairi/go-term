#!/bin/bash

# GoTerm MCP Server Installation Script
#
# This script installs GoTerm MCP Server with automatic dependency management
# Supports: macOS, Linux, Windows (WSL/Git Bash)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/rama-kairi/go-term/main/install.sh | bash
#   wget -qO- https://raw.githubusercontent.com/rama-kairi/go-term/main/install.sh | bash

set -e

# Script configuration
SCRIPT_VERSION="1.0.0"
REPO_OWNER="rama-kairi"
REPO_NAME="go-term"
BINARY_NAME="go-term"
MIN_GO_VERSION="1.19"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Emojis for better UX
SUCCESS="✅"
ERROR="❌"
INFO="ℹ️"
WARNING="⚠️"
ROCKET="🚀"
GEAR="⚙️"
DOWNLOAD="📥"

# Utility functions
log_info() {
    echo -e "${CYAN}${INFO} $1${NC}"
}

log_success() {
    echo -e "${GREEN}${SUCCESS} $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}${WARNING} $1${NC}"
}

log_error() {
    echo -e "${RED}${ERROR} $1${NC}"
}

log_step() {
    echo -e "\n${BLUE}${GEAR} $1${NC}"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect operating system
detect_os() {
    case "$(uname -s)" in
        Darwin*) echo "darwin" ;;
        Linux*) echo "linux" ;;
        CYGWIN*|MINGW*|MSYS*) echo "windows" ;;
        *) echo "unknown" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        armv7l) echo "arm" ;;
        *) echo "unknown" ;;
    esac
}

# Get Go binary info
get_go_binary_info() {
    local os="$1"
    local arch="$2"

    case "$os" in
        "darwin") echo "go.tar.gz" ;;
        "linux") echo "go.tar.gz" ;;
        "windows") echo "go.zip" ;;
        *) echo "unknown" ;;
    esac
}

# Get latest Go version
get_latest_go_version() {
    local version
    if command_exists curl; then
        version=$(curl -s https://go.dev/VERSION?m=text | head -n1)
    elif command_exists wget; then
        version=$(wget -qO- https://go.dev/VERSION?m=text | head -n1)
    else
        log_error "Neither curl nor wget found. Cannot determine latest Go version."
        exit 1
    fi
    echo "${version#go}"
}

# Compare versions
version_greater_equal() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

# Check Go installation
check_go() {
    if command_exists go; then
        local go_version=$(go version | cut -d' ' -f3 | sed 's/go//')
        if version_greater_equal "$go_version" "$MIN_GO_VERSION"; then
            log_success "Go $go_version is already installed and meets requirements"
            return 0
        else
            log_warning "Go $go_version is installed but needs upgrade (minimum: $MIN_GO_VERSION)"
            return 1
        fi
    else
        log_info "Go is not installed"
        return 1
    fi
}

# Install Go
install_go() {
    local os=$(detect_os)
    local arch=$(detect_arch)

    if [ "$os" = "unknown" ] || [ "$arch" = "unknown" ]; then
        log_error "Unsupported operating system or architecture: $os/$arch"
        exit 1
    fi

    log_step "Installing Go for $os/$arch"

    # Try package managers first
    case "$os" in
        "linux")
            # Try various package managers
            if command_exists apt-get; then
                log_info "Installing Go via apt..."
                sudo apt-get update && sudo apt-get install -y golang-go
                return 0
            elif command_exists yum; then
                log_info "Installing Go via yum..."
                sudo yum install -y golang
                return 0
            elif command_exists dnf; then
                log_info "Installing Go via dnf..."
                sudo dnf install -y golang
                return 0
            elif command_exists pacman; then
                log_info "Installing Go via pacman..."
                sudo pacman -S --noconfirm go
                return 0
            elif command_exists snap; then
                log_info "Installing Go via snap..."
                sudo snap install go --classic
                return 0
            fi
            ;;
        "darwin")
            if command_exists brew; then
                log_info "Installing Go via Homebrew..."
                brew install go
                return 0
            fi
            ;;
    esac

    # Fallback to manual installation
    log_info "Installing Go manually from official releases..."
    install_go_manual "$os" "$arch"
}

# Manual Go installation
install_go_manual() {
    local os="$1"
    local arch="$2"
    local go_version=$(get_latest_go_version)
    local filename="go${go_version}.${os}-${arch}"
    local extension=$(get_go_binary_info "$os" "$arch")
    local download_url="https://go.dev/dl/${filename}.${extension}"
    local install_dir="/usr/local"

    # Use home directory for non-root users
    if [ "$(id -u)" != "0" ]; then
        install_dir="$HOME/.local"
        mkdir -p "$install_dir"
    fi

    log_info "Downloading Go $go_version..."
    local temp_file="/tmp/${filename}.${extension}"

    if command_exists curl; then
        curl -fsSL "$download_url" -o "$temp_file"
    elif command_exists wget; then
        wget -q "$download_url" -O "$temp_file"
    else
        log_error "Neither curl nor wget found. Cannot download Go."
        exit 1
    fi

    log_info "Installing Go to $install_dir..."

    # Remove existing Go installation
    if [ -d "$install_dir/go" ]; then
        rm -rf "$install_dir/go"
    fi

    # Extract Go
    case "$extension" in
        "tar.gz")
            tar -C "$install_dir" -xzf "$temp_file"
            ;;
        "zip")
            unzip -q "$temp_file" -d "$install_dir"
            ;;
    esac

    # Clean up
    rm -f "$temp_file"

    # Set up environment
    setup_go_environment "$install_dir"

    log_success "Go $go_version installed successfully!"
}

# Set up Go environment
setup_go_environment() {
    local install_dir="$1"
    local go_bin="$install_dir/go/bin"
    local go_path="$HOME/go"

    # Create GOPATH directory
    mkdir -p "$go_path/bin"

    # Shell profile files to update
    local profiles=("$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile")

    for profile in "${profiles[@]}"; do
        if [ -f "$profile" ] || [ "$profile" = "$HOME/.profile" ]; then
            # Check if Go paths are already in the profile
            if ! grep -q "$go_bin" "$profile" 2>/dev/null; then
                log_info "Adding Go to PATH in $profile"
                {
                    echo ""
                    echo "# Go language environment"
                    echo "export GOROOT=$install_dir/go"
                    echo "export GOPATH=$go_path"
                    echo "export PATH=\$PATH:$go_bin:\$GOPATH/bin"
                } >> "$profile"
            fi
        fi
    done

    # Export for current session
    export GOROOT="$install_dir/go"
    export GOPATH="$go_path"
    export PATH="$PATH:$go_bin:$go_path/bin"

    log_success "Go environment configured"
}

# Install jq if needed (for JSON manipulation)
install_jq() {
    if command_exists jq; then
        return 0
    fi

    log_step "Installing jq (JSON processor)..."

    local os=$(detect_os)
    case "$os" in
        "linux")
            if command_exists apt-get; then
                sudo apt-get install -y jq
            elif command_exists yum; then
                sudo yum install -y jq
            elif command_exists dnf; then
                sudo dnf install -y jq
            elif command_exists pacman; then
                sudo pacman -S --noconfirm jq
            else
                log_warning "Could not install jq automatically. Please install it manually."
                return 1
            fi
            ;;
        "darwin")
            if command_exists brew; then
                brew install jq
            else
                log_warning "Homebrew not found. Please install jq manually."
                return 1
            fi
            ;;
        *)
            log_warning "Could not install jq automatically for $os. Please install it manually."
            return 1
            ;;
    esac

    log_success "jq installed successfully"
}

# Install GoTerm MCP Server
install_goterm() {
    log_step "Installing GoTerm MCP Server..."

    # Ensure Go is in PATH
    if ! command_exists go; then
        log_error "Go is not in PATH. Please restart your shell or source your profile."
        log_info "Run: source ~/.bashrc (or ~/.zshrc)"
        exit 1
    fi

    # Install via go install
    log_info "Installing go-term via 'go install'..."
    go install "github.com/${REPO_OWNER}/${REPO_NAME}@latest"

    # Verify installation
    local gopath=$(go env GOPATH)
    local binary_path="$gopath/bin/$BINARY_NAME"

    if [ -f "$binary_path" ]; then
        log_success "GoTerm MCP Server installed to $binary_path"
        return 0
    else
        log_error "Installation verification failed. Binary not found at $binary_path"
        return 1
    fi
}

# Get VS Code MCP config path
get_vscode_mcp_path() {
    local os=$(detect_os)
    case "$os" in
        "darwin")
            echo "$HOME/Library/Application Support/Code/User/mcp.json"
            ;;
        "linux")
            echo "$HOME/.config/Code/User/mcp.json"
            ;;
        "windows")
            echo "$APPDATA/Code/User/mcp.json"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Update VS Code MCP configuration
update_vscode_mcp_config() {
    local mcp_path=$(get_vscode_mcp_path)

    if [ -z "$mcp_path" ]; then
        log_warning "Could not determine VS Code MCP config path for your OS"
        return 1
    fi

    # Ensure directory exists
    mkdir -p "$(dirname "$mcp_path")"

    # Get binary path
    local gopath=$(go env GOPATH)
    local binary_path="$gopath/bin/$BINARY_NAME"

    log_step "Updating VS Code MCP configuration..."

    # Create or update mcp.json
    if [ -f "$mcp_path" ]; then
        # Check if jq is available for JSON manipulation
        if command_exists jq; then
            # Backup existing config
            cp "$mcp_path" "${mcp_path}.backup"

            # Add go-term server to existing config
            jq --arg binary_path "$binary_path" \
               '.servers["go-term"] = {"command": $binary_path}' \
               "$mcp_path" > "${mcp_path}.tmp" && mv "${mcp_path}.tmp" "$mcp_path"

            log_success "Updated existing MCP configuration"
        else
            log_warning "jq not available. Please manually add go-term to your MCP config."
            show_manual_config_instructions "$binary_path"
            return 1
        fi
    else
        # Create new config file
        cat > "$mcp_path" << EOF
{
  "servers": {
    "go-term": {
      "command": "$binary_path"
    }
  },
  "inputs": []
}
EOF
        log_success "Created new MCP configuration"
    fi

    log_info "MCP configuration updated at: $mcp_path"
    return 0
}

# Show manual configuration instructions
show_manual_config_instructions() {
    local binary_path="$1"
    local mcp_path=$(get_vscode_mcp_path)

    echo
    log_info "Manual MCP Configuration:"
    echo "Add the following to your MCP config file at: $mcp_path"
    echo
    echo "{"
    echo "  \"servers\": {"
    echo "    \"go-term\": {"
    echo "      \"command\": \"$binary_path\""
    echo "    }"
    echo "  }"
    echo "}"
}

# Show post-installation instructions
show_post_install_instructions() {
    echo
    echo -e "${GREEN}${ROCKET} GoTerm MCP Server Installation Complete! ${ROCKET}${NC}"
    echo
    log_success "Installation Summary:"
    echo "  • Go language installed and configured"
    echo "  • GoTerm MCP Server installed"
    echo "  • VS Code MCP configuration updated"
    echo
    log_info "Next Steps:"
    echo "  1. Restart VS Code to reload MCP configuration"
    echo "  2. Open VS Code MCP extension"
    echo "  3. Select 'go-term' server and disable default terminal tools"
    echo "  4. Enable 'Beast Mode' in GitHub Copilot settings for enhanced terminal features"
    echo
    log_info "Available GoTerm Tools:"
    echo "  • create_terminal_session - Create isolated project sessions"
    echo "  • list_terminal_sessions - View all active sessions"
    echo "  • run_command - Execute commands with smart background detection"
    echo "  • search_terminal_history - Find and analyze previous commands"
    echo "  • delete_session - Clean up sessions"
    echo "  • check_background_process - Monitor long-running processes"
    echo
    log_info "Documentation: https://github.com/${REPO_OWNER}/${REPO_NAME}#readme"
    echo
    log_warning "If you encounter issues, please restart your shell or run:"
    echo "  source ~/.bashrc  # or ~/.zshrc"
}

# Main installation function
main() {
    echo -e "${BLUE}"
    cat << 'EOF'
   ____      _____
  / ___| ___|_   _|__ _ __ _ __ ___
 | |  _ / _ \ | |/ _ \ '__| '_ ` _ \
 | |_| | (_) || |  __/ |  | | | | |
  \____|\___/ |_|\___|_|  |_| |_| |

     MCP Server Installation
EOF
    echo -e "${NC}"

    log_info "GoTerm MCP Server Installer v$SCRIPT_VERSION"
    log_info "Installing for $(detect_os)/$(detect_arch)"
    echo

    # Check and install Go
    if ! check_go; then
        install_go

        # Verify Go installation
        if ! command_exists go; then
            log_error "Go installation failed or not in PATH"
            log_info "Please restart your terminal and try again"
            exit 1
        fi
    fi

    # Install jq for JSON manipulation
    install_jq

    # Install GoTerm
    if ! install_goterm; then
        log_error "GoTerm installation failed"
        exit 1
    fi

    # Update VS Code MCP config
    if ! update_vscode_mcp_config; then
        local gopath=$(go env GOPATH)
        local binary_path="$gopath/bin/$BINARY_NAME"
        show_manual_config_instructions "$binary_path"
    fi

    # Show completion message
    show_post_install_instructions
}

# Handle script interruption
trap 'log_error "Installation interrupted by user"; exit 1' INT TERM

# Check if running as root (not recommended)
if [ "$(id -u)" = "0" ]; then
    log_warning "Running as root. This is not recommended."
    log_info "Consider running as a regular user instead."
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Run main installation
main "$@"
