# safeenv

`safeenv` is a tiny, intentionally stupid CLI for hiding your `.env` file.

I do not like dragging cloud secret managers into small projects that just run
on a cheap VPS. Sometimes there is no Key Vault, no Secret Manager, no fancy
platform layer, and no budget for pretending otherwise. I also do not like
leaving a plaintext `.env` on a VPS and pretending that is fine because the file
has a dot in front of it. I do not know what all the serious people out there
are doing, but my skill issue and my wallet have agreed that this is good enough
for now.

The idea is simple: keep plaintext `.env` on your machine, push only
`.env.safeenv` to the VPS, and let `safeenv` decrypt it in memory while running
your app start command.

If someone casually grabs your VPS files, they get encrypted soup instead of your
database password. If they already own your runtime, root, Docker daemon, GitHub
Actions, SSH key, laptop, and childhood memories, congratulations, this tool is
not a wizard.

## Install Locally

Requirements:

- Go
- `age`
- `ssh` / `scp`

Build:

```sh
go build -o safeenv .
```

Optional install:

```sh
mkdir -p ~/.local/bin
cp safeenv ~/.local/bin/safeenv
```

Make sure `~/.local/bin` is in your `PATH`.

## Basic Usage

Create a local env file:

```sh
safeenv init .env.cloud
```

Generate keys:

```sh
safeenv key-gen .env.cloud
```

This creates:

```text
.env.cloud.key
.env.cloud.pub
```

Keep `.env.cloud.key` private. Put its contents in GitHub Actions repository
secrets as `SAFEENV_PRIVATE_KEY`.

Encrypt `.env.cloud`:

```sh
safeenv encrypt .env.cloud
```

This creates:

```text
.env.cloud.safeenv
```

The file starts with:

```text
SAFEENV AGE V1
```

so at least it looks like it belongs to `safeenv`, not like some random cursed
blob you found under the deploy sofa.

## Push To VPS

```sh
safeenv push user@1.2.3.4 /app
```

This pushes your `*.safeenv` file to `/app`, and also installs `safeenv` on the
VPS.
`push` expects exactly one `*.safeenv` file in the current directory.

It tries:

```text
/usr/local/bin/safeenv
```

If that fails, it falls back to:

```text
~/.local/bin/safeenv
```

So you do not need to make GitHub Actions copy the binary. Relax. One fewer yak.

If your server falls back to `~/.local/bin/safeenv` and that directory is not in
`PATH`, call it with the full path. Computers enjoy tiny acts of betrayal.

## GitHub Actions Deploy

Store the private key in any GitHub Actions repository secret name you like, for
example:

```text
ENV_SECRET_KEY
```

Inside the deploy step, map that secret to `SAFEENV_PRIVATE_KEY`. That env var
name is what `safeenv` reads.

Example deploy step:

```yaml
- name: Deploy
  env:
    SAFEENV_PRIVATE_KEY: ${{ secrets.ENV_SECRET_KEY }}
  run: |
    printf '%s' "$SAFEENV_PRIVATE_KEY" | ssh user@1.2.3.4 '
      set -eu
      cd /app

      export SAFEENV_PRIVATE_KEY="$(cat)"
      safeenv .env.cloud.safeenv -- your-start-command
    '
```

This decrypts `.env.cloud.safeenv` in memory, adds the values to the command
environment, and runs your command. No temporary plaintext `.env` file is
written to disk. A small miracle, by very low standards.

If `safeenv push` had to fall back to the user-local install path, use:

```sh
~/.local/bin/safeenv .env.cloud.safeenv -- your-start-command
```

For Docker Compose, that might be:

```sh
safeenv .env.cloud.safeenv -- docker compose up -d
```

For a Node.js app, maybe:

```sh
safeenv .env.cloud.safeenv -- npm start
```

## Pull Back From VPS

Pull encrypted files only:

```sh
safeenv pull user@1.2.3.4 /app
```

Pull and decrypt locally:

```sh
safeenv pull user@1.2.3.4 /app --decrypt=.env.cloud.key
```

This writes:

```text
.env.decrypted
```

## Commands

```text
safeenv init <file.env>
safeenv key-gen [name]
safeenv encrypt <file.env>
safeenv push <user>@<ip> <folder>
safeenv pull <user>@<ip> <folder> [--decrypt=<secret.key>]
safeenv <file.safeenv> -- <command> [args...]
safeenv decrypt [file.safeenv]
safeenv clean
```

`decrypt` and `clean` are kept for debugging and manual recovery. For deploys,
prefer the direct command form so plaintext never has to become a file.

## What This Does Not Protect

This protects against plaintext `.env` being casually left on the VPS filesystem.

It does not protect against:

- someone who can read GitHub Actions secrets
- someone who controls the VPS during deploy
- root or Docker daemon access
- secrets already loaded into running containers

It is not Vault. It is not KMS. It is a small broom that sweeps `.env` off the
floor before someone trips over it.
