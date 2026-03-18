# FTP Server

Know includes an optional FTP server for accessing documents with any standard FTP client. Files map to documents, directories map to vaults and folders.

## Quick Start

```bash
# Enable FTP server
know serve --ftp

# Custom port
know serve --ftp --ftp-port 2121

# Or via environment variables
KNOW_FTP_ENABLED=true KNOW_FTP_PORT=2121 know serve
```

## Authentication

FTP uses the same API tokens as the REST API:

- **Username**: anything (ignored, e.g. `know` or your email)
- **Password**: your Know API token (e.g. `kh_...`)

In `--no-auth` mode, any password is accepted.

## Connecting

### Command-line FTP

```bash
ftp localhost 2121
# Username: know
# Password: kh_your_token_here
ftp> ls
ftp> cd default
ftp> ls
ftp> get notes/hello.md
ftp> put newfile.md notes/newfile.md
```

### lftp (recommended)

```bash
lftp -u know,kh_your_token ftp://localhost:2121
lftp> ls
lftp> mirror default/notes ./notes   # download folder
lftp> put hello.md -o default/hello.md
```

### FileZilla / Cyberduck

1. Host: `localhost`, Port: `2121`, Protocol: FTP
2. Username: `know`, Password: your API token
3. Connect — vaults appear as top-level directories

### curl

```bash
# List files
curl ftp://localhost:2121/ -u know:kh_your_token

# Download
curl ftp://localhost:2121/default/notes/hello.md -u know:kh_your_token -o hello.md

# Upload
curl -T hello.md ftp://localhost:2121/default/hello.md -u know:kh_your_token
```

## File Structure

```
/                       # Root — lists all accessible vaults
├── default/            # Vault "default"
│   ├── notes/          # Folder
│   │   ├── hello.md
│   │   └── todo.md
│   └── journal/
│       └── 2026-03-18.md
└── work/               # Vault "work"
    └── projects/
        └── design.md
```

## Constraints

- **Markdown only**: Only `.md` files can be created or uploaded. Other file types are rejected.
- **Max file size**: 10 MB per document.
- **Passive mode only**: Active mode (`PORT`/`EPRT`) is not supported. Most modern clients default to passive mode.
- **No TLS**: FTP traffic is unencrypted. For secure access, use SSH/SFTP instead (`know serve --ssh`).

## Passive Port Range

By default, the OS picks random ports for passive data connections. To use a specific range (useful for firewalls/Docker):

```bash
KNOW_FTP_PASV_MIN=30000 KNOW_FTP_PASV_MAX=30100 know serve --ftp
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_FTP_ENABLED` | `false` | Enable FTP server |
| `KNOW_FTP_PORT` | `2121` | FTP control port |
| `KNOW_FTP_PASV_MIN` | `0` | Passive port range start (0 = OS picks) |
| `KNOW_FTP_PASV_MAX` | `0` | Passive port range end (0 = OS picks) |

## Supported FTP Commands

| Command | Description |
|---------|-------------|
| USER/PASS | Authentication |
| LIST, NLST, MLSD | Directory listing |
| RETR | Download file |
| STOR | Upload file |
| DELE | Delete file |
| MKD/RMD | Create/remove directory |
| RNFR/RNTO | Rename file or directory |
| CWD/CDUP/PWD | Navigate directories |
| SIZE | Get file size |
| MDTM | Get modification time |
| PASV/EPSV | Passive mode |
| TYPE | Transfer type (ASCII/Binary) |
| SYST/FEAT | Server info |

## Comparison with Other Access Methods

| Feature | FTP | SFTP | WebDAV | NFS |
|---------|-----|------|--------|-----|
| Encryption | No | Yes (SSH) | Optional (HTTPS) | No |
| Auth | Token via password | Token via password | Token via password | None (localhost) |
| Client support | Universal | Universal | Editors, OS mount | OS mount |
| Binary assets | No (markdown only) | No (markdown only) | Yes | No (markdown only) |
| Best for | Simple file transfer | Secure file transfer | Editor integration | Fast local mount |
