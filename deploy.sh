#!/bin/bash

# NFL App Deployment Script
# This script helps deploy the NFL Parlay Club application as a systemd service

set -e

# Configuration
APP_NAME="nfl-app"
APP_USER="nfl-app"
DEPLOY_DIR="/opt/nfl-app"
SERVICE_FILE="/etc/systemd/system/nfl-app.service"
BINARY_NAME="nfl-app"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Create application user
create_user() {
    if id "$APP_USER" &>/dev/null; then
        log_info "User $APP_USER already exists"
    else
        log_info "Creating user $APP_USER"
        useradd --system --shell /bin/false --home "$DEPLOY_DIR" --create-home "$APP_USER"
        log_success "User $APP_USER created"
    fi
}

# Build the application
build_app() {
    log_info "Building application..."

    if [ ! -f "main.go" ]; then
        log_error "main.go not found. Run this script from the project root directory."
        exit 1
    fi

    # Build for current architecture
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 /home/ryanp/.local/share/go/bin/go build -ldflags="-w -s" -o "$BINARY_NAME"
    

    if [ ! -f "$BINARY_NAME" ]; then
        log_error "Build failed - binary not created"
        exit 1
    fi

    log_success "Application built successfully"
}

# Build utility scripts
build_scripts() {
    log_info "Building utility scripts..."

    if [ -d "scripts" ]; then
        # Create scripts directory in build
        mkdir -p scripts_built

        # Build restore script
        if [ -f "scripts/restore_backup.go" ]; then
            CGO_ENABLED=0 GOARCH=arm64 /home/ryanp/.local/share/go/bin/go build -ldflags="-w -s" -o scripts_built/restore_backup scripts/restore_backup.go
            log_success "Restore backup script built"
        fi

        # Build manual backup script
        if [ -f "scripts/manual_backup.go" ]; then
            CGO_ENABLED=0 GOARCH=arm64 /home/ryanp/.local/share/go/bin/go build -ldflags="-w -s" -o scripts_built/manual_backup scripts/manual_backup.go
            log_success "Manual backup script built"
        fi
    fi
}

# Create deployment directory structure
create_directories() {
    log_info "Creating deployment directories..."

    # Create main directory
    mkdir -p "$DEPLOY_DIR"

    # Create subdirectories
    mkdir -p "$DEPLOY_DIR/backups"
    mkdir -p "$DEPLOY_DIR/logs"
    mkdir -p "$DEPLOY_DIR/scripts"

    # Set ownership
    chown -R "$APP_USER:$APP_USER" "$DEPLOY_DIR"

    log_success "Deployment directories created"
}

# Copy application files
copy_files() {
    log_info "Copying application files..."

    # Copy main binary
    cp "$BINARY_NAME" "$DEPLOY_DIR/"
    chmod +x "$DEPLOY_DIR/$BINARY_NAME"

    # Copy static files
    if [ -d "static" ]; then
        cp -r static "$DEPLOY_DIR/"
        log_success "Static files copied"
    fi

    # Copy templates
    if [ -d "templates" ]; then
        cp -r templates "$DEPLOY_DIR/"
        log_success "Templates copied"
    fi

    # Copy built scripts
    if [ -d "scripts_built" ]; then
        cp scripts_built/* "$DEPLOY_DIR/scripts/"
        chmod +x "$DEPLOY_DIR/scripts/"*
        log_success "Utility scripts copied"
    fi

    # Copy .env file if it exists and automatically set production environment
    if [ -f ".env" ]; then
        log_info "Copying .env file to deployment directory"
        cp .env "$DEPLOY_DIR/"

        # Automatically set ENVIRONMENT=production in the deployed copy only
        if grep -q "^ENVIRONMENT=" "$DEPLOY_DIR/.env"; then
            # Replace existing ENVIRONMENT setting in deployed copy
            sed -i 's/^ENVIRONMENT=.*/ENVIRONMENT=production/' "$DEPLOY_DIR/.env"
            log_success "Updated ENVIRONMENT=production in deployed .env (local .env unchanged)"
        else
            # Add ENVIRONMENT=production if not present in deployed copy
            echo "ENVIRONMENT=production" >> "$DEPLOY_DIR/.env"
            log_success "Added ENVIRONMENT=production to deployed .env (local .env unchanged)"
        fi

        chmod 600 "$DEPLOY_DIR/.env"
        log_success ".env file deployed with production environment"
    else
        log_warning ".env file not found - you'll need to create one in $DEPLOY_DIR"
    fi


    # Set ownership
    chown -R "$APP_USER:$APP_USER" "$DEPLOY_DIR"

    log_success "Application files copied"
}

# Create systemd service file
create_service() {
    log_info "Creating systemd service..."

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=NFL Parlay Club Web Application
After=network.target mongodb.service

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
WorkingDirectory=$DEPLOY_DIR
ExecStart=$DEPLOY_DIR/$BINARY_NAME
Restart=always
RestartSec=10
Environment=GIN_MODE=release

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DEPLOY_DIR/backups $DEPLOY_DIR/logs

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    systemctl daemon-reload

    log_success "Systemd service created"
}

# Configure service
configure_service() {
    log_info "Configuring service..."

    # Enable service
    systemctl enable "$APP_NAME"
    log_success "Service enabled for auto-start"

    # Don't start automatically - let user configure first
    log_info "Service configured but not started"
    log_info "Configure $DEPLOY_DIR/.env before starting"
}

# Setup log rotation
setup_log_rotation() {
    log_info "Setting up log rotation..."

    # Copy logrotate configuration
    if [ -f "logrotate.conf" ]; then
        cp logrotate.conf "/etc/logrotate.d/$APP_NAME"
        log_success "Log rotation configured"

        # Test logrotate configuration
        if logrotate -d "/etc/logrotate.d/$APP_NAME" >/dev/null 2>&1; then
            log_success "Log rotation configuration validated"
        else
            log_warning "Log rotation configuration may have issues - check manually"
        fi
    else
        log_warning "logrotate.conf not found - creating basic configuration"

        # Create basic logrotate config
        cat > "/etc/logrotate.d/$APP_NAME" << EOF
$DEPLOY_DIR/logs/*.log {
    size 30M
    rotate 10
    compress
    delaycompress
    missingok
    notifempty
    create 640 $APP_USER $APP_USER
    postrotate
        /bin/systemctl reload-or-restart $APP_NAME >/dev/null 2>&1 || true
    endscript
    copytruncate
    dateext
}
EOF
        log_success "Basic log rotation configuration created"
    fi
}

# Print post-deployment instructions
print_instructions() {
    echo ""
    log_success "Deployment completed successfully!"
    echo ""
    echo "📋 Next Steps:"
    echo "1. Configure the application:"
    echo "   sudo nano $DEPLOY_DIR/.env"
    echo ""
    echo "2. Start the service:"
    echo "   sudo systemctl start $APP_NAME"
    echo ""
    echo "3. Check service status:"
    echo "   sudo systemctl status $APP_NAME"
    echo ""
    echo "4. View logs:"
    echo "   sudo journalctl -u $APP_NAME -f"
    echo ""
    echo "📁 Deployment Structure:"
    echo "   Application: $DEPLOY_DIR/$BINARY_NAME"
    echo "   Configuration: $DEPLOY_DIR/.env"
    echo "   Backups: $DEPLOY_DIR/backups"
    echo "   Scripts: $DEPLOY_DIR/scripts"
    echo ""
    echo "🔧 Utility Commands:"
    echo "   Manual backup: sudo -u $APP_USER $DEPLOY_DIR/scripts/manual_backup"
    echo "   Restore backup: sudo systemctl stop $APP_NAME && sudo -u $APP_USER $DEPLOY_DIR/scripts/restore_backup && sudo systemctl start $APP_NAME"
    echo ""
    echo "⚠️  Important:"
    echo "   - Update JWT_SECRET in .env file"
    echo "   - Configure database credentials"
    echo "   - Set up MongoDB authentication"
    echo "   - Configure firewall rules"
    echo "   - Application configured for Cloudflare proxy (USE_TLS=false, BEHIND_PROXY=true)"
    echo ""
}

# Cleanup function
cleanup() {
    log_info "Cleaning up build artifacts..."
    rm -f "$BINARY_NAME"
    rm -rf scripts_built
}

# Main deployment process
main() {
    echo "🚀 NFL App Deployment Script"
    echo "============================"

    check_root
    create_user
    build_app
    build_scripts
    create_directories
    copy_files
    create_service
    configure_service
    setup_log_rotation
    cleanup
    print_instructions
}

# Handle script interruption
trap cleanup EXIT

# Run main function
main "$@"