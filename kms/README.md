# ProgressDB KMS (prgkms)

Key Management Service for ProgressDB encryption.

# Install

### Direct Download
```bash
# macOS/Linux
curl -L https://github.com/ProgressDB/progressdb/releases/latest/download/prgkms_{{version}}_{{os}}_{{arch}}.tar.gz | tar xz
sudo mv prgkms /usr/local/bin/

# Windows
# Download prgkms_{{version}}_windows_amd64.zip and extract
```

### Package Managers
```bash
# Homebrew (macOS/Linux)
brew install progressdb/prgkms

# Scoop (Windows)
scoop install progressdb/prgkms
```

### Docker
```bash
docker run -d --name prgkms -p 6820:6820 -v $PWD/kms-data:/data docker.io/progressdb/prgkms:latest
```

# Configuration

```yaml
kms:
  db_path: "/path/to/kms/data"
  master_key_file: "/path/to/master/key.txt"
  # or master_key_hex: "your-32-byte-hex-key"
```

Create master key:
```bash
openssl rand -hex 32 > /path/to/master/key.txt
chmod 600 /path/to/master/key.txt
```

# Usage

```bash
# Start with defaults (127.0.0.1:6820)
prgkms

# Custom address
prgkms --addr 0.0.0.0:8080

# With config
prgkms --config config.yaml
```

# API

```http
GET /health
POST /api/v1/keys
GET /api/v1/keys/{key-id}
GET /api/v1/keys
DELETE /api/v1/keys/{key-id}
```