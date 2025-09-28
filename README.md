# NFL Parlay Club - Go Edition

A real-time NFL pick'em and parlay scoring web application built with Go, MongoDB, and HTMX. Features live game updates, user pick management, parlay scoring, and comprehensive analytics.

## 🏈 Features

- **Real-time Game Updates**: Live scores and game status updates via ESPN API
- **Pick Management**: User-friendly interface for making weekly picks
- **Parlay Scoring**: Automatic calculation of parlay scores and club rankings
- **Live Updates**: Server-Sent Events (SSE) with HTMX for real-time UI updates
- **Analytics Dashboard**: Comprehensive statistics and user performance tracking
- **Automated Backups**: Nightly database backups with configurable retention
- **Responsive Design**: Mobile-friendly interface with dark/light theme support

## 🛠️ Technology Stack

- **Backend**: Go 1.21+
- **Database**: MongoDB with authentication
- **Frontend**: Server-side rendered HTML with HTMX
- **Real-time**: Server-Sent Events (SSE)
- **Styling**: CSS with Material Design elements
- **Authentication**: JWT-based sessions
- **Email**: SMTP support for password resets

## 📋 Prerequisites

- Go 1.21 or later
- MongoDB 4.4+ (with authentication configured)
- Access to ESPN API (automatic)
- Optional: SMTP server for email functionality

## 🚀 Quick Start

### 1. Clone and Build

```bash
git clone <repository-url>
cd nfl-app-go

# Build the application
go build -o nfl-app
```

### 2. Configure Environment

Create a `.env` file in the project root:

```env
# Database Configuration
DB_HOST=your-mongo-host
DB_PORT=27017
DB_USERNAME=your-username
DB_PASSWORD=your-password
DB_NAME=nfl_app

# Server Configuration
SERVER_PORT=8080
USE_TLS=false
BEHIND_PROXY=true
ENVIRONMENT=production

# JWT Secret (CHANGE THIS!)
JWT_SECRET=your-unique-jwt-secret-here

# Email Configuration (Optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
FROM_EMAIL=your-email@gmail.com
FROM_NAME=NFL Games

# Application Settings
CURRENT_SEASON=2025
BACKGROUND_UPDATER_ENABLED=true
MOCK_UPDATER_ENABLED=false

# Backup Configuration
BACKUP_ENABLED=true
BACKUP_DIR=./backups
BACKUP_TIME=02:00
BACKUP_RETENTION_DAYS=30

# Logging Configuration (Production)
LOG_LEVEL=info
LOG_FILE=true
LOG_DIR=./logs
```

### 3. Run the Application

```bash
# Development
./nfl-app

# Or with environment variables
DB_PASSWORD=yourpassword ./nfl-app
```

Visit `http://localhost:8080` to access the application.

## 🏗️ Production Deployment

### Building for Production

```bash
# Build optimized binary
CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o nfl-app

# Or for ARM64 (Raspberry Pi)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o nfl-app
```

### systemd Service Setup

Create `/etc/systemd/system/nfl-app.service`:

```ini
[Unit]
Description=NFL Parlay Club Web Application
After=network.target mongodb.service

[Service]
Type=simple
User=nfl-app
Group=nfl-app
WorkingDirectory=/opt/nfl-app
ExecStart=/opt/nfl-app/nfl-app
Restart=always
RestartSec=10
Environment=GIN_MODE=release

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/nfl-app/backups /opt/nfl-app/logs

[Install]
WantedBy=multi-user.target
```

### Deployment Structure

```
/opt/nfl-app/
├── nfl-app                 # Main executable
├── .env                    # Environment configuration
├── static/                 # Static assets
│   ├── style.css
│   └── favicon.ico
├── templates/              # HTML templates
├── backups/               # Backup directory (auto-created)
├── logs/                  # Log directory with rotation
└── scripts/               # Pre-compiled utility scripts
    ├── restore_backup     # Compiled restore tool
    └── manual_backup      # Compiled backup tool
```

### Service Management

```bash
# Install and start service
sudo systemctl enable nfl-app
sudo systemctl start nfl-app

# Check status
sudo systemctl status nfl-app

# View logs
sudo journalctl -u nfl-app -f

# Restart service
sudo systemctl restart nfl-app
```

## 💾 Backup Management

### Automated Backups

Automated backups run nightly at the configured time (default: 2:00 AM) and include:
- `weekly_picks` collection (all user picks)
- `games` collection (game data and scores)
- Automatic cleanup of backups older than retention period

### Manual Backup Operations

**Create Manual Backup:**
```bash
# From deployment directory
go run scripts/manual_backup.go

# Or if deployed as service
sudo -u nfl-app /opt/nfl-app/scripts/manual_backup
```

**Restore from Backup:**
```bash
# Interactive restore tool
go run scripts/restore_backup.go

# Or if deployed as service (STOP SERVICE FIRST!)
sudo systemctl stop nfl-app
sudo -u nfl-app /opt/nfl-app/scripts/restore_backup
sudo systemctl start nfl-app
```

### Service Deployment Backup Considerations

When deployed as a systemd service:

1. **Backup Location**: Ensure backup directory has proper permissions
   ```bash
   sudo mkdir -p /opt/nfl-app/backups
   sudo chown nfl-app:nfl-app /opt/nfl-app/backups
   ```

2. **Restore Process**: Always stop the service before restoring
   ```bash
   sudo systemctl stop nfl-app
   # Perform restore
   sudo systemctl start nfl-app
   ```

3. **Script Compilation**: Pre-compile scripts for production
   ```bash
   # Build utility scripts
   go build -o scripts/restore_backup scripts/restore_backup.go
   go build -o scripts/manual_backup scripts/manual_backup.go
   ```

## 🔧 Configuration Reference

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `p5server` | MongoDB hostname |
| `DB_PORT` | `27017` | MongoDB port |
| `DB_USERNAME` | `nflapp` | MongoDB username |
| `DB_PASSWORD` | *(required)* | MongoDB password |
| `DB_NAME` | `nfl_app` | MongoDB database name |
| `SERVER_PORT` | `8080` | HTTP server port |
| `USE_TLS` | `false` | Enable HTTPS |
| `BEHIND_PROXY` | `false` | Running behind reverse proxy |
| `ENVIRONMENT` | `development` | Environment mode |
| `JWT_SECRET` | *(required)* | JWT signing secret |
| `CURRENT_SEASON` | `2025` | NFL season year |
| `BACKUP_ENABLED` | `true` | Enable automated backups |
| `BACKUP_DIR` | `./backups` | Backup storage directory |
| `BACKUP_TIME` | `02:00` | Daily backup time (24h format) |
| `BACKUP_RETENTION_DAYS` | `30` | Days to retain backups |
| `LOG_LEVEL` | `info` | Logging level (debug/info/warn/error) |
| `LOG_FILE` | `false` | Enable file logging (production: true) |
| `LOG_DIR` | `./logs` | Log file directory |

### Database Setup

The application expects a MongoDB instance with:
- Authentication enabled
- Database named according to `DB_NAME`
- User with read/write permissions
- Collections: `users`, `games`, `picks`

Default users are automatically seeded on first run.

## 🔍 Monitoring and Log Management

### Log Configuration

The application supports both console and file logging:

- **Development**: Logs to console only (`LOG_FILE=false`)
- **Production**: Logs to both file and console (`LOG_FILE=true`)

### Log Rotation System

**Automatic Rotation** (Production):
- **File Size**: Logs rotate when they reach 30MB
- **Retention**: Keeps 10 rotated files (~300MB total)
- **Compression**: Old logs are automatically compressed
- **Service Integration**: Rotation triggers graceful log file reopening

**Configuration File**: `/etc/logrotate.d/nfl-app`

### Log Levels
- `debug`: Detailed development information
- `info`: General application information
- `warn`: Warning messages
- `error`: Error messages
- `fatal`: Critical errors that stop the application

### Log File Locations

**Production Deployment**:
```
/opt/nfl-app/logs/
├── nfl-app.log                 # Current log file
├── nfl-app.log-20250928-123456 # Rotated log (compressed)
└── nfl-app.log-20250927-123456.gz
```

### Log Monitoring Commands

```bash
# Monitor live application logs
sudo journalctl -u nfl-app -f

# View log files directly (production)
sudo tail -f /opt/nfl-app/logs/nfl-app.log

# Monitor specific patterns
sudo journalctl -u nfl-app | grep -i backup    # Backup operations
sudo journalctl -u nfl-app | grep -i espn      # ESPN API updates
sudo journalctl -u nfl-app | grep -i sse       # SSE connections
sudo journalctl -u nfl-app | grep -i mongo     # Database operations
sudo journalctl -u nfl-app | grep -i error     # Error messages

# Check log rotation status
sudo logrotate -d /etc/logrotate.d/nfl-app

# Force log rotation (testing)
sudo logrotate -f /etc/logrotate.d/nfl-app
```

### Log Volume Management

**Expected Log Volume**:
- **Normal Operation**: ~5-10MB per day
- **High Activity**: ~20-30MB per day (heavy SSE traffic)
- **Debug Mode**: ~50-100MB per day

**Storage Planning**:
- 10 rotated files × 30MB = ~300MB storage
- Plus current log file = ~330MB total maximum

## 🚨 Security Considerations

1. **Change Default Passwords**: Update all default passwords and JWT secrets
2. **TLS Configuration**: Use HTTPS in production with valid certificates
3. **Database Security**: Ensure MongoDB authentication is properly configured
4. **File Permissions**: Restrict access to .env file and backup directory
5. **Network Security**: Use firewall rules to limit access to necessary ports
6. **Regular Updates**: Keep Go runtime and dependencies updated

## 🐛 Troubleshooting

### Common Issues

**Database Connection Failed**
```bash
# Check MongoDB status
sudo systemctl status mongodb

# Verify credentials
mongosh "mongodb://username:password@host:port/database"
```

**Service Won't Start**
```bash
# Check service logs
sudo journalctl -u nfl-app --since="1 hour ago"

# Verify .env file
sudo -u nfl-app cat /opt/nfl-app/.env
```

**Backup Failures**
```bash
# Check disk space
df -h /opt/nfl-app/backups

# Check permissions
sudo -u nfl-app ls -la /opt/nfl-app/backups
```

## 📝 Development

### Local Development Setup

```bash
# Install dependencies
go mod download

# Run in development mode
ENVIRONMENT=development go run main.go

# Run with hot reload (requires air)
air
```

### Building Scripts

```bash
# Build all utility scripts
go build -o scripts/restore_backup scripts/restore_backup.go
go build -o scripts/manual_backup scripts/manual_backup.go
```

## 📜 License

This project is proprietary software. All rights reserved.

## 🤝 Support

For support and questions, check the application logs and configuration settings. The application includes comprehensive logging to help diagnose issues.