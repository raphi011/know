# Remotes (Multi-Server Federation)

The remote system enables federation between knowhow servers. Connect to other knowhow instances and access their vaults as if they were local.

## Overview

Remotes let you register external knowhow servers by name, URL, and API token. Once registered, a remote's vaults are discovered automatically and become accessible alongside local vaults using a namespaced identifier (`remoteName/vaultName`). Tool calls targeting a remote vault are transparently proxied via the REST API.

## How It Works

1. A remote server is registered with a unique name, base URL, and authentication token.
2. The system queries the remote's `/api/vaults` endpoint to discover available vaults.
3. Discovered vaults are namespaced as `remoteName/vaultName` (e.g. `home/default`) so they coexist with local vaults without collisions.
4. Remote vault lists are cached for 60 seconds. Unreachable remotes are skipped with a warning rather than failing the entire operation.
5. When a tool call targets a vault like `home/default`, the system splits the identifier to find remote name `home` and vault name `default`, then proxies the request to the corresponding server.

## Usage

### CLI Commands

```bash
# List configured remotes
knowhow remote list

# Add a remote server
knowhow remote add home http://home-server:8484 --token kh_token_here

# The token can also be provided via environment variable
KNOWHOW_REMOTE_TOKEN=kh_token_here knowhow remote add home http://home-server:8484

# Remove a remote
knowhow remote remove home
```

### Agent Usage

Once a remote is configured, agents can target remote vaults in any supported tool call by using the namespaced vault identifier:

```
search query="deployment guide" vault="home/default"
read_document path="/ops/runbook.md" vault="home/default"
```

No special syntax is needed beyond the `remoteName/vaultName` format.

## Supported Operations

The following operations are proxied to remote servers via the REST API:

| Operation | Description |
|---|---|
| `search` | Hybrid search across a remote vault |
| `read_document` | Read a document by path |
| `list_labels` | List all labels in the vault |
| `list_folders` | Browse the folder structure |
| `list_folder_contents` | List children of a specific folder |
| `create_document` | Create a new document |
| `edit_document` | Edit an existing document (full replacement) |
| `create_memory` | Create a memory entry on the remote |
| `get_document_versions` | List version history for a document |

### Not Supported on Remotes

- **`edit_document_section`** -- Section-level editing requires local execution. As a workaround, use `read_document` to fetch the full content, then `edit_document` to replace it.

## Reference

- **Namespace format**: `remoteName/vaultName` (e.g. `home/default`, `work/team-docs`)
- **Cache TTL**: Remote vault lists are cached for 60 seconds
- **Failure handling**: Unreachable remotes are skipped with a warning; other remotes and local vaults remain available
- **Auth**: Each remote uses its own API token, independent of the local server's tokens
