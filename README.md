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

Also, please do not compare this to Mozilla SOPS. SOPS is excellent software
made by serious people for serious workflows. `safeenv` is one person quietly
building the small stupid thing he needs so a cheap VPS can stop hoarding
plaintext `.env` files. If you compare it to SOPS, I will become emotionally
available to sadness.

## Install Locally

Requirements:

- Go
- `age`
- `ssh` / `scp`

Install:

```sh
git clone <this-repo-url>
cd safeenv
./install.sh
```

This builds and installs:

```text
/usr/local/bin/safeenv
```

It uses `sudo` if your user cannot write there directly, because of course it
cannot.

## Basic Usage

Create a local env file:

```sh
safeenv init .env.cloud
```

Or create the env file and keys in one shot:

```sh
safeenv init .env.cloud --keygen
```

Generate keys separately:

```sh
safeenv keygen .env.cloud
```

This creates:

```text
.env.cloud.key
.env.cloud.pub
```

Keep `.env.cloud.key` private. Put its contents in a GitHub Actions repository
secret. Name it whatever you want; the workflow maps it to `SAFEENV_PRIVATE_KEY`
when running `safeenv`.

Encrypt `.env.cloud`:

```sh
safeenv encrypt .env.cloud
```

With no extra flags, `safeenv` looks for the public key file next to it:

```text
.env.cloud.pub
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

Add extra recipients if more than one key should decrypt the file:

```sh
safeenv encrypt .env.cloud --public-key age1...
```

If `.env.cloud.pub` does not exist, you must pass at least one public key:

```sh
safeenv encrypt .env.cloud --public-key age1...
```

These are equivalent:

```sh
safeenv encrypt .env.cloud --public-key age1...
safeenv encrypt .env.cloud -r age1...
safeenv encrypt .env.cloud --recipient age1...
```

Use `--public-key` if you do not want to remember `age` vocabulary.

`.env.cloud.pub` can also contain multiple age public keys, one per line. Use
`--public-key` when you want to add more recipients without editing the file.

## Push To VPS

```sh
safeenv push .env.cloud.safeenv user@1.2.3.4 /app
```

This pushes that encrypted file to `/app`, detects the VPS OS/arch with
`uname`, and installs the matching Linux `safeenv` binary from the GitHub release
for the current `safeenv` version.
`push` only accepts `*.safeenv` files, so it will reject a plaintext `.env.cloud`
before anything leaves your machine.

It installs to:

```text
/usr/local/bin/safeenv
```

So you do not need to make GitHub Actions copy or build the binary, and the VPS
does not need Go installed. Relax. One fewer yak.

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

If you do not want to map your GitHub secret to `SAFEENV_PRIVATE_KEY`, pick the
env var name explicitly:

```sh
ENV_SECRET_KEY=... safeenv --env-key-name=ENV_SECRET_KEY .env.cloud.safeenv -- your-start-command
```

This decrypts `.env.cloud.safeenv` in memory, adds the values to the command
environment, and runs your command. No temporary plaintext `.env` file is
written to disk. A small miracle, by very low standards.

For Docker Compose, that might be:

```sh
safeenv .env.cloud.safeenv -- docker compose up -d
```

For a Node.js app, maybe:

```sh
safeenv .env.cloud.safeenv -- npm start
```

Private key sources:

```sh
SAFEENV_PRIVATE_KEY="$(cat .env.cloud.key)" safeenv .env.cloud.safeenv -- npm start
SAFEENV_PRIVATE_KEY=.env.cloud.key safeenv .env.cloud.safeenv -- npm start
ENV_SECRET_KEY=.env.cloud.key safeenv --env-key-name=ENV_SECRET_KEY .env.cloud.safeenv -- npm start
safeenv -i .env.cloud.key .env.cloud.safeenv -- npm start
cat .env.cloud.key | safeenv -i - .env.cloud.safeenv -- npm start
```

`safeenv` logs structured text to stderr: time, action, status, file, command,
and message. It does not log secret values. If it sees `docker compose` or
`docker-compose`, it logs a warning because Compose `up` does not have a useful
`-e` path; your compose file still needs to wire env vars into the container.

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

That is for debugging or recovery. It writes plaintext to disk. Do not use it as
your deploy path unless you enjoy leaving little problems around like decorative
furniture.

## Commands

```text
safeenv init <file.env> [--keygen]
safeenv keygen [name]
safeenv encrypt <file.env> [--public-key age1...]
safeenv push <file.safeenv> <user>@<ip> <folder>
safeenv pull <user>@<ip> <folder> [--decrypt=<secret.key>]
safeenv [--env-key-name=ENV] [-i <identity-file>|-i -] <file.safeenv> -- <command> [args...]
safeenv decrypt [-i <identity-file>|-i -] [file.safeenv]
safeenv list
safeenv inspect <file.safeenv>
safeenv clean
safeenv --help
safeenv --version
```

`list` shows `*.safeenv` and `.env.*` files with columns for format, size,
whether the file is encrypted, and whether matching public/private key files are
present. `inspect` shows metadata for one encrypted file.

`decrypt`, `pull --decrypt`, and `clean` are kept for debugging and manual
recovery. They can write plaintext to disk. For deploys, prefer the direct
command form so plaintext never has to become a file:

```sh
safeenv .env.cloud.safeenv -- your-start-command
```
Commands that overwrite files automatically rename the old file to
`<name>.<timestamp>.bak` first. Tiny seatbelt, still a seatbelt.

## What This Does Not Protect

This protects against plaintext `.env` being casually left on the VPS filesystem.

It does not protect against:

- someone who can read GitHub Actions secrets
- someone who controls the VPS during deploy
- root or Docker daemon access
- secrets already loaded into running containers

It is not Vault. It is not KMS. It is a small broom that sweeps `.env` off the
floor before someone trips over it.
