# SuperApp H5 SSO Demo

A server-rendered Partner H5 demo for the SuperApp Embed SSO V1 flow.

The browser uses the pinned SuperApp Embed SDK to request an authorization
code from the trusted native WebView bridge. The Partner backend keeps the
PKCE verifier, client private key, access token, and refresh token on the
server side.

## Requirements

- Node.js 24+
- A registered and active SuperApp Embed Client
- An ES256 Partner private key stored in the ignored local `secrets/` directory
- An HTTPS origin for WebView testing

## Setup

```bash
npm install
cp .env.example .env
```

Fill in `.env` with the local SuperApp endpoints, Embed Client ID, key ID,
private-key path, approved scopes, and a randomly generated session secret.

Place the local Partner private key at:

```text
secrets/partner-es256-private.pem
```

Keep its filesystem mode at `600`. The entire `secrets/` directory is ignored
by Git and is not served by Express, which exposes static files only from
`public/`.

```bash
openssl rand -hex 32
```

Never commit `.env` or any file from `secrets/`.

## Run

```bash
npm run dev
```

The `predev` hook builds the browser bundle before starting the Express
server. By default the H5 is available at:

```text
http://localhost:3000/app
```

Use an HTTPS tunnel for a real SuperApp WebView integration. The registered
Launch URL must be the tunnel origin plus `/app`.

## Test

```bash
npm test
npm run build:client
```

## Security boundaries

- The browser receives only `transaction_id`, `state`, PKCE challenge,
  authorization code, and Partner-defined user display data.
- The PKCE verifier stays in the Partner transaction store.
- The ES256 private key is read only by the Partner backend.
- SuperApp access and refresh tokens stay in the server-side Partner session.
- A normal browser has no trusted native bridge and cannot start SSO.

The included transaction store and Express session store are in-memory and
are intended only for a single-process local demo. Production deployments
should use an atomic shared transaction store and a persistent encrypted
session/token store.
