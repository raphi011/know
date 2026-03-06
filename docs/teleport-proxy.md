# Teleport AWS Proxy — Bedrock Integration

How `tsh proxy aws` works and the issues we hit getting the Bedrock LLM/embedding clients to work through it.

## Architecture

`tsh proxy aws --app <app> -p <port>` creates a **two-layer proxy**:

```
Client ──► Forward Proxy (:9876) ──► Local ALPN Proxy (MITM TLS) ──► Teleport Server ──► AWS
              │                            │
              │ CONNECT tunnel             │ Terminates TLS (local CA)
              │ Routes AWS requests        │ Runs AWSAccessMiddleware
              │ to ALPN proxy              │ Verifies SigV4 signatures
              │                            │ Forwards via reverse proxy
              └────────────────────────────┘
```

### Layer 1: Forward Proxy (port 9876)

- Plain HTTP server accepting CONNECT requests only
- Routes AWS-matching requests (by hostname) to the Local ALPN Proxy
- Non-AWS requests go to system proxy or direct
- Source: `lib/srv/alpnproxy/forward_proxy.go`

### Layer 2: Local ALPN Proxy

- TLS server using a self-signed local CA cert (the `AWS_CA_BUNDLE` file)
- HTTP reverse proxy with `AWSAccessMiddleware`
- Forwards to the Teleport server via ALPN, which re-signs with real AWS credentials
- Source: `lib/srv/alpnproxy/local_proxy.go`, `aws_local_proxy.go`

### Credential Flow

1. `tsh proxy aws` generates **fake UUID credentials** (random access key + secret)
2. These are exported as `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`
3. Client signs requests with these fake credentials
4. `AWSAccessMiddleware.HandleRequest()` **verifies the SigV4 signature** against the same fake credentials
5. If valid, request is forwarded to Teleport server
6. Teleport server **re-signs** with real AWS credentials before sending to AWS

Source: `tool/tsh/common/app_aws.go` — `GetAWSCredentialsProvider()` generates UUIDs, `GetEnvVars()` exports them.

### Signature Verification Details

`VerifyAWSSignature()` in `lib/utils/aws/aws.go`:

1. Parses the `Authorization` header for SigV4 components
2. Checks `AccessKeyID` matches the proxy-issued credentials
3. **Special case**: If `User-Agent` is in `SignedHeaders`, verification is **skipped** (only AccessKeyID checked) because "AWS Go SDK explicitly skips the User-Agent header so it will always produce a different signature"
4. Otherwise: re-computes the signature using the same credentials and compares
5. Mismatch → 403 Forbidden (no body)

### Error Sources

- **403 from proxy** (no body): `AWSAccessMiddleware.handleCommonRequest()` — signature verification failed
- **400 "Bad Request"** (text/plain, `X-Content-Type-Options: nosniff`): `httputil.ReverseProxy.ErrorHandler` in `makeHTTPReverseProxy()` — request passed verification but upstream connection to Teleport failed. Uses Go's `http.Error()` which explains the distinctive response format.
- **403 from Bedrock** (no body, no `X-Content-Type-Options`): Request reached AWS but had invalid credentials or permissions

## Issues Encountered

### Issue 1: AWS_CA_BUNDLE Panic (eino-ext bug #1)

**Symptom**: Server panics on startup when `AWS_CA_BUNDLE` is set.

**Root cause**: eino-ext passes `Config.HTTPClient` (*http.Client) to `awsConfig.WithHTTPClient()`. When `AWS_CA_BUNDLE` is set, the AWS SDK's `resolveCustomCABundle` does a type assertion to `*awshttp.BuildableClient`, which fails on a plain `*http.Client` → panic.

**Workaround**: Unset `AWS_CA_BUNDLE` before creating the eino-ext client, handle the CA cert ourselves.

**Status**: Fixed in `internal/llm/model.go:newBedrockChatModel()`.

### Issue 2: HTTPClient Not Used for API Calls (eino-ext bug #2)

**Symptom**: Even with a custom `HTTPClient` that trusts the proxy CA, requests fail with certificate errors.

**Root cause**: In eino-ext's Bedrock path (`claude.go:86-105`), the `HTTPClient` is only passed to `awsConfig.WithHTTPClient()` (for SigV4 signing), NOT to `option.WithHTTPClient()` (for actual API calls). The Anthropic SDK always uses `http.DefaultClient` for Bedrock requests.

**Workaround**: Patch `http.DefaultTransport` with the proxy CA cert so `http.DefaultClient` trusts it.

**Status**: Fixed in `internal/llm/model.go:addCAToDefaultTransport()`.

### Issue 3: CA Cert File Overwritten by Other tsh Sessions

**Symptom**: Intermittent TLS certificate errors. Requests fail with "certificate signed by unknown authority" even though the proxy is running.

**Root cause**: Multiple `tsh` sessions (e.g., `tsh proxy aws` for knowhow AND `tsh aws --exec claude` for Claude Code) all write their CA certs to the **same file** at `~/.tsh/keys/<host>/<user>-app/<cluster>/<app>-localca.pem`. Each new session overwrites the previous cert. The proxy uses the cert from when IT started, but the app reads the file later — by which point a different session may have overwritten it.

**Workaround**: Snapshot the CA cert to `~/.tsh/knowhow-proxy-ca.pem` immediately after starting the proxy.

**Status**: Fixed in `bedrock-setup.fish` (copies cert to stable location after proxy starts).

### Issue 4: 400 Bad Request from Proxy — RESOLVED

**Symptom**: All SigV4-signed requests (Go AWS SDK, Anthropic SDK, curl with SigV4 headers) get `400 Bad Request` with body `"Bad Request\n"`, content-type `text/plain; charset=utf-8`, header `X-Content-Type-Options: nosniff`.

**Root cause**: **HTTPS_PROXY environment variable loop.** The `tsh proxy aws` process inherited `HTTPS_PROXY=http://127.0.0.1:9876` from the shell (set by `bedrock-setup.fish` from a previous run or the current script order).

When the ALPN proxy's reverse proxy forwards signed requests to the Teleport server, it creates a new `http.Transport` connection. Go's `http.Transport` reads `HTTPS_PROXY` from the environment and tries to route the upstream connection through `127.0.0.1:9876` — the same local proxy — creating a connection loop. The proxy receives its own CONNECT request, which fails with `502 Bad Gateway`, and the `ErrorHandler` translates that to `400 Bad Request`.

**Why unsigned requests worked**: They take a different code path. The forward proxy routes the CONNECT tunnel directly to the ALPN proxy, which passes unsigned requests through to AWS without establishing a new HTTP connection. Only signed requests trigger the reverse proxy → Teleport server path.

**Debug log evidence**:
```
WARN [LOCALPROX] Failed to handle request error:[
  Original Error: *trace.BadParameterError unable to proxy connection: 502 Bad Gateway
  Stack Trace:
    client/proxy.go:158 client.dialProxyWithHTTPDialer  ← dials through HTTPS_PROXY
    ...
] method:POST url:https://<teleport-server>:443/model/.../invoke
```

**Fix**: Start `tsh proxy aws` with `HTTPS_PROXY`/`HTTP_PROXY` cleared from its environment:
```fish
env -u HTTPS_PROXY -u HTTP_PROXY tsh proxy aws --app $APP -p $PORT
```

**Status**: Fixed in `bedrock-setup.fish`.

## Files

- `internal/llm/model.go` — Bedrock LLM workaround (issues 1 & 2)
- `internal/llm/embedder.go` — Bedrock embedder (uses AWS SDK natively)
- `bedrock-setup.fish` — Proxy startup + cert snapshot (issues 3 & 4)
- `docker-compose.prod.yml` — Docker prod stack config
- `.env` — Generated credentials and proxy config (not committed)

## Teleport Source References (v18.6.1)

- `lib/srv/alpnproxy/forward_proxy.go` — Forward proxy CONNECT handler
- `lib/srv/alpnproxy/local_proxy.go` — Local ALPN proxy, `makeHTTPReverseProxy()` with ErrorHandler
- `lib/srv/alpnproxy/aws_local_proxy.go` — `AWSAccessMiddleware`, signature verification flow
- `lib/utils/aws/aws.go` — `VerifyAWSSignature()`, `ParseSigV4()`
- `lib/utils/aws/signing.go` — `SignRequest()`, `removeUnsignedHeaders()`
- `tool/tsh/common/app_aws.go` — `GetAWSCredentialsProvider()` (UUID generation), `startLocalForwardProxy()`
