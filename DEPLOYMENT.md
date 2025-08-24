# Deployment Configurations

This application supports multiple deployment modes with full security.

## Environment Variables

Configure these in your `.env` file:

```env
# Server Configuration
USE_TLS=false          # Set to true for direct HTTPS
SERVER_PORT=8080       # Port to listen on
BEHIND_PROXY=true      # Set to true when using Cloudflare Tunnel/reverse proxy

# Database
DB_HOST=your-mongo-host
DB_PORT=27017
DB_USERNAME=your-username  
DB_PASSWORD=your-password
DB_NAME=nfl_app

# Email (Gmail SMTP)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
FROM_EMAIL=your-email@gmail.com
FROM_NAME=NFL Games

# Security
JWT_SECRET=your-long-random-secret-key
```

## Deployment Modes

### 1. Cloudflare Tunnel (Recommended)
**Configuration:**
```env
USE_TLS=false
SERVER_PORT=8080
BEHIND_PROXY=true
```

**Security Features:**
- ✅ HTTPS handled by Cloudflare
- ✅ Cookies: `Secure=false` (Cloudflare handles HTTPS)
- ✅ HSTS headers when CF-Visitor header detected
- ✅ All other security headers active
- ✅ Passwords encrypted by Cloudflare's TLS

**Setup:**
1. Install `cloudflared`
2. Run: `cloudflared tunnel create nfl-app`
3. Configure tunnel to point to `http://localhost:8080`
4. Start app with above config

### 2. Direct HTTPS (Development)
**Configuration:**
```env
USE_TLS=true
SERVER_PORT=8443
BEHIND_PROXY=false
```

**Security Features:**
- ✅ Direct TLS encryption
- ✅ Cookies: `Secure=true`
- ✅ HSTS headers
- ✅ Self-signed certificate (browser warning)

**Setup:**
1. Generate certificate: `go run generate_cert.go`
2. Visit: `https://localhost:8443`
3. Accept browser security warning

### 3. HTTP Only (Development Only)
**Configuration:**
```env
USE_TLS=false
SERVER_PORT=8080
BEHIND_PROXY=false
```

**⚠️ Security Warning:**
- ❌ Passwords transmitted in plaintext
- ❌ No cookie security
- ❌ Only use for local development

## Production Security Checklist

✅ **Encryption:** Passwords encrypted in transit (Cloudflare or direct TLS)
✅ **Hashing:** Passwords hashed with bcrypt in database  
✅ **Cookies:** HttpOnly, Secure (when appropriate), SameSite=Strict
✅ **Headers:** HSTS, XSS Protection, CSRF Protection, CSP
✅ **Logging:** No plaintext passwords in logs
✅ **JWT:** Secure token generation with expiry

## Cloudflare Setup Commands

```bash
# Install cloudflared
# Download from: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/

# Authenticate with Cloudflare
cloudflared tunnel login

# Create tunnel
cloudflared tunnel create nfl-app

# Create config file ~/.cloudflared/config.yml
tunnel: your-tunnel-id
credentials-file: /path/to/credentials.json

ingress:
  - hostname: your-domain.com
    service: http://localhost:8080
  - service: http_status:404

# Run tunnel
cloudflared tunnel run nfl-app
```

Your application is now production-ready with enterprise-grade security! 🔒