#!/bin/bash
# install-to-rock-master.sh - Install rock-os-tools binaries to ROCK-MASTER
#
# This script installs the built Go tools from the rock-os-tools project
# to the ROCK-MASTER/bin/tools directory for immediate use.
#
# Usage:
#   ./install-to-rock-master.sh          # Copy binaries
#   ./install-to-rock-master.sh --symlink # Create symlinks (development)
#   ./install-to-rock-master.sh --force   # Overwrite existing files
#   ./install-to-rock-master.sh --check   # Check installation only

set -e

# Script configuration
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Load configuration if exists
if [ -f "${SCRIPT_DIR}/config.env" ]; then
    source "${SCRIPT_DIR}/config.env"
fi

# Default values
PROJECT_ROOT="${ROCK_TOOLS_ROOT:-${SCRIPT_DIR}}"
ROCK_MASTER_ROOT="${ROCK_MASTER_ROOT:-/Volumes/4TB/ROCK-MASTER}"
INSTALL_MODE="copy"
FORCE_INSTALL=false
CHECK_ONLY=false

# Tool list
TOOLS_LIST="${TOOLS_LIST:-rock-kernel rock-deps rock-build rock-image rock-config rock-security rock-cache rock-verify rock-compose rock-registry}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_color() {
    local color=$1
    shift
    echo -e "${color}$*${NC}"
}

# Function to print error and exit
error_exit() {
    print_color "$RED" "Error: $1" >&2
    exit 1
}

# Function to print warning
warning() {
    print_color "$YELLOW" "Warning: $1" >&2
}

# Function to print info
info() {
    print_color "$BLUE" "$1"
}

# Function to print success
success() {
    print_color "$GREEN" "✓ $1"
}

# Function to show usage
usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install rock-os-tools binaries to ROCK-MASTER/bin/tools directory

OPTIONS:
    --symlink       Create symlinks instead of copying (for development)
    --force         Overwrite existing files without prompting
    --check         Check installation status without installing
    --help          Show this help message

ENVIRONMENT:
    ROCK_TOOLS_ROOT     Source directory (default: script directory)
    ROCK_MASTER_ROOT    Target ROCK-MASTER directory (default: /Volumes/4TB/ROCK-MASTER)
    TOOLS_LIST          Space-separated list of tools to install

EXAMPLES:
    $SCRIPT_NAME                    # Standard installation
    $SCRIPT_NAME --symlink          # Development installation
    $SCRIPT_NAME --check            # Check what would be installed
    $SCRIPT_NAME --force            # Force overwrite existing tools

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --symlink)
            INSTALL_MODE="symlink"
            shift
            ;;
        --force)
            FORCE_INSTALL=true
            shift
            ;;
        --check)
            CHECK_ONLY=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            error_exit "Unknown option: $1\nUse --help for usage information"
            ;;
    esac
done

# Detect current platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Map architecture names
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        warning "Unknown architecture: $ARCH"
        ;;
esac

# Source and destination directories
SRC_DIR="${PROJECT_ROOT}/bin/${OS}"
DEST_DIR="${ROCK_MASTER_ROOT}/bin"
TOOLS_DEST="${DEST_DIR}/tools"

# Legacy compatibility directories
LEGACY_DEBUG="${DEST_DIR}/debug/macos"
LEGACY_RELEASE="${DEST_DIR}/release/macos"

# Validation checks
info "Checking installation environment..."

# Check if source directory exists
if [ ! -d "$PROJECT_ROOT" ]; then
    error_exit "Project root not found: $PROJECT_ROOT"
fi

# Check if any tools are built
if [ ! -d "$SRC_DIR" ]; then
    error_exit "No built tools found in: $SRC_DIR\nPlease run 'make build' first"
fi

# Check if ROCK_MASTER exists
if [ ! -d "$ROCK_MASTER_ROOT" ]; then
    if [ "$CHECK_ONLY" = true ]; then
        warning "ROCK_MASTER_ROOT not found: $ROCK_MASTER_ROOT"
    else
        error_exit "ROCK_MASTER_ROOT not found: $ROCK_MASTER_ROOT"
    fi
fi

# Print configuration
echo "Installation Configuration:"
echo "  Source:      $SRC_DIR"
echo "  Destination: $TOOLS_DEST"
echo "  Mode:        $INSTALL_MODE"
echo "  Platform:    $OS/$ARCH"
echo ""

# If check only, show what would be installed
if [ "$CHECK_ONLY" = true ]; then
    info "Checking available tools..."

    found_count=0
    missing_count=0
    installed_count=0

    for tool in $TOOLS_LIST; do
        if [ -f "$SRC_DIR/$tool" ]; then
            echo "  ✓ $tool (available)"
            ((found_count++))

            if [ -f "$TOOLS_DEST/$tool" ]; then
                echo "    → Already installed in tools/"
                ((installed_count++))
            fi
        else
            echo "  ✗ $tool (not built)"
            ((missing_count++))
        fi
    done

    echo ""
    echo "Summary:"
    echo "  Available:  $found_count"
    echo "  Missing:    $missing_count"
    echo "  Installed:  $installed_count"

    exit 0
fi

# Create destination directories
info "Creating destination directories..."
mkdir -p "$TOOLS_DEST"
mkdir -p "$LEGACY_DEBUG"
mkdir -p "$LEGACY_RELEASE"
success "Directories created"

# Function to install a tool
install_tool() {
    local tool=$1
    local src="$SRC_DIR/$tool"
    local dest="$TOOLS_DEST/$tool"

    if [ ! -f "$src" ]; then
        warning "$tool not found in $SRC_DIR"
        return 1
    fi

    # Check if destination exists
    if [ -f "$dest" ] && [ "$FORCE_INSTALL" = false ]; then
        if [ "$INSTALL_MODE" = "symlink" ]; then
            # For symlinks, check if it's already a symlink to the same source
            if [ -L "$dest" ]; then
                local current_target=$(readlink "$dest")
                if [ "$current_target" = "$src" ]; then
                    info "$tool already linked correctly"
                    return 0
                fi
            fi
        fi

        # Ask for confirmation
        read -p "  $tool already exists. Overwrite? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            warning "Skipping $tool"
            return 0
        fi
    fi

    # Remove existing file if present
    rm -f "$dest"

    # Install based on mode
    if [ "$INSTALL_MODE" = "symlink" ]; then
        ln -sf "$src" "$dest"
        success "Linked $tool"
    else
        cp "$src" "$dest"
        chmod 755 "$dest"
        success "Copied $tool"
    fi

    # Legacy compatibility for critical tools
    if [[ "$tool" == "rock-kernel" || "$tool" == "rock-build" || "$tool" == "rock-image" ]]; then
        if [ "$INSTALL_MODE" = "copy" ]; then
            cp "$src" "$LEGACY_DEBUG/$tool" 2>/dev/null || true
            cp "$src" "$LEGACY_RELEASE/$tool" 2>/dev/null || true
            info "  Also copied to legacy locations"
        fi
    fi

    return 0
}

# Install each tool
info "Installing tools..."
echo ""

success_count=0
fail_count=0

for tool in $TOOLS_LIST; do
    echo "Installing $tool..."
    if install_tool "$tool"; then
        ((success_count++))
    else
        ((fail_count++))
    fi
done

# Create PATH setup script
info "Creating PATH setup script..."
cat > "$TOOLS_DEST/setup-path.sh" << 'EOF'
#!/bin/bash
# setup-path.sh - Add rock-os-tools to PATH

# Get the directory where this script is located
TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Add to PATH if not already present
if [[ ":$PATH:" != *":$TOOLS_DIR:"* ]]; then
    export PATH="$PATH:$TOOLS_DIR"
    echo "rock-os-tools added to PATH at: $TOOLS_DIR"
else
    echo "rock-os-tools already in PATH"
fi

# Show available tools
echo "Available tools:"
for tool in $TOOLS_DIR/rock-*; do
    if [ -f "$tool" ] && [ -x "$tool" ]; then
        echo "  - $(basename $tool)"
    fi
done
EOF

chmod +x "$TOOLS_DEST/setup-path.sh"
success "Created setup-path.sh"

# Create version info file
info "Creating version info..."
cat > "$TOOLS_DEST/VERSION.txt" << EOF
rock-os-tools installation
Installed: $(date)
Source: $PROJECT_ROOT
Mode: $INSTALL_MODE
Platform: $OS/$ARCH
Tools: $success_count installed, $fail_count failed
EOF
success "Created VERSION.txt"

# Print summary
echo ""
print_color "$GREEN" "════════════════════════════════════════"
print_color "$GREEN" "Installation Complete!"
print_color "$GREEN" "════════════════════════════════════════"
echo ""
echo "Summary:"
echo "  Installed:  $success_count tools"
if [ $fail_count -gt 0 ]; then
    echo "  Failed:     $fail_count tools"
fi
echo "  Location:   $TOOLS_DEST"
echo ""
echo "To use the tools, either:"
echo ""
echo "  1. Add to PATH for this session:"
echo "     export PATH=\"\$PATH:$TOOLS_DEST\""
echo ""
echo "  2. Add to PATH permanently (add to ~/.bashrc or ~/.zshrc):"
echo "     export PATH=\"\$PATH:$TOOLS_DEST\""
echo ""
echo "  3. Source the setup script:"
echo "     source $TOOLS_DEST/setup-path.sh"
echo ""
echo "  4. Use tools directly:"
echo "     $TOOLS_DEST/rock-kernel --help"
echo ""

# Test one tool to verify installation
if [ -f "$TOOLS_DEST/rock-kernel" ]; then
    info "Testing installation..."
    if "$TOOLS_DEST/rock-kernel" version >/dev/null 2>&1; then
        success "Installation verified - tools are working!"
    else
        warning "Tools installed but may need additional setup"
    fi
fi

exit 0