# Install wizard promotes npx execution to a global install

`npx -y @yeepay/yop-cli@latest install` runs only inside a temporary npx cache, so the install wizard explicitly runs `npm install -g` for the package (resolving the registry's `latest` Declared Version, upgrading when the global prefix holds an older one; the postinstall downloads the matching binary) before installing global Skills. This mirrors ADR-0002's runtime-update promotion and the reference Feishu CLI installer, trading npx-only purity for a directly invocable `yop-cli` command.
