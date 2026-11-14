# Devcontainer for the Go + eBPF backend, Vue/Bun frontend, wrapper, and adapters.
FROM debian:trixie

ARG DEBIAN_FRONTEND=noninteractive
ARG GO_VERSION=1.27.3
ARG NODE_VERSION=24.11.1
ARG USERNAME=vscode
ARG USER_UID=1001
ARG USER_GID=${USER_UID}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV GOPATH=/go \
    GOROOT=/usr/local/go \
    BUN_INSTALL=/usr/local/bun \
    UV_LINK_MODE=copy \
    PATH=/usr/local/go/bin:/go/bin:/usr/local/bun/bin:/root/.cargo/bin:/home/${USERNAME}/.cargo/bin:/home/${USERNAME}/.local/bin:${PATH}

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash-completion \
    build-essential \
    ca-certificates \
    clang \
    clang-format \
    cmake \
    curl \
    git \
    gnupg \
    iproute2 \
    jq \
    less \
    libbpf-dev \
    libcap-dev \
    libclang-dev \
    libelf-dev \
    lld \
    llvm \
    make \
    nano \
    pkg-config \
    polkitd \
    pkexec \
    protobuf-compiler \
    python3 \
    python3-pip \
    python3-venv \
    sudo \
    unzip \
    xz-utils \
    zlib1g-dev \
    fish \
 && rm -rf /var/lib/apt/lists/*

RUN arch="$(dpkg --print-architecture)" \
 && case "$arch" in \
      amd64) go_arch=amd64 ;; \
      arm64) go_arch=arm64 ;; \
      *) echo "Unsupported architecture for Go: $arch" >&2; exit 1 ;; \
    esac \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" -o /tmp/go.tgz \
 && rm -rf /usr/local/go \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm /tmp/go.tgz \
 && mkdir -p /go/bin /go/pkg /go/src

RUN arch="$(dpkg --print-architecture)" \
 && case "$arch" in \
      amd64) node_arch=x64 ;; \
      arm64) node_arch=arm64 ;; \
      *) echo "Unsupported architecture for Node.js: $arch" >&2; exit 1 ;; \
    esac \
 && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" -o /tmp/node.tar.xz \
 && tar -C /tmp -xf /tmp/node.tar.xz \
 && cp "/tmp/node-v${NODE_VERSION}-linux-${node_arch}/bin/node" /usr/local/bin/node \
 && rm -rf /tmp/node.tar.xz "/tmp/node-v${NODE_VERSION}-linux-${node_arch}"

RUN curl -fsSL https://bun.sh/install | bash -s bun-v1.3.3 \
 && ln -sf /usr/local/bun/bin/bun /usr/local/bin/bun \
 && ln -sf /usr/local/bun/bin/bun /usr/local/bin/bunx

RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh

# Pin the official Zellij binary and verify the GitHub release asset digest.
# Avoid compiling the Rust toolchain under ARM64 QEMU in every image build.
ARG ZELLIJ_VERSION=0.46.0
RUN arch="$(dpkg --print-architecture)" \
 && case "$arch" in \
      amd64) target=x86_64; expected_sha=0940e467f99ca57aec5a63edcad633450d49cd6f22e8a7d9c686771a0d9f86ae ;; \
      arm64) target=aarch64; expected_sha=7ce8501136dab30767fea809191f39ca41d66d100082a96534539f49c7a53f0d ;; \
      *) echo "Unsupported Zellij architecture: $arch" >&2; exit 1 ;; \
    esac \
 && curl -fsSL --retry 3 "https://github.com/zellij-org/zellij/releases/download/v${ZELLIJ_VERSION}/zellij-${target}-unknown-linux-gnu.tar.gz" -o /tmp/zellij.tgz \
 && printf '%s  %s\\n' "$expected_sha" /tmp/zellij.tgz | sha256sum -c - \
 && mkdir -p /tmp/zellij-release \
 && tar -xzf /tmp/zellij.tgz -C /tmp/zellij-release \
 && install -m 0755 /tmp/zellij-release/zellij /usr/local/bin/zellij \
 && rm -rf /tmp/zellij.tgz /tmp/zellij-release

RUN groupadd --gid ${USER_GID} ${USERNAME} \
 && useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} -s /usr/bin/fish \
 && echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
 && chmod 0440 /etc/sudoers.d/${USERNAME} \
 && mkdir -p /home/${USERNAME}/.config/git /opt/agent-ebpf-predev \
 && touch /home/${USERNAME}/.gitconfig \
 && chown -R ${USERNAME}:${USERNAME} /go /usr/local/bun /home/${USERNAME}/.gitconfig /home/${USERNAME}/.config /opt/agent-ebpf-predev

USER ${USERNAME}
WORKDIR /workspaces/agent-ebpf-filiter

COPY --chown=${USERNAME}:${USERNAME} Makefile ./Makefile
COPY --chown=${USERNAME}:${USERNAME} frontend/package.json frontend/bun.lock ./frontend/
COPY --chown=${USERNAME}:${USERNAME} adapters/python/ ./adapters/python/
COPY --chown=${USERNAME}:${USERNAME} tools/dev-env-tui/ ./tools/dev-env-tui/
COPY --chown=${USERNAME}:${USERNAME} tools/agent-tui/ ./tools/agent-tui/
COPY --chown=${USERNAME}:${USERNAME} backend/go.mod backend/go.sum ./backend/

# During image construction the bind-mounted workspace and .git do not exist.
# Install only dependencies, never run the predev git-hook setup here.
RUN mkdir -p ~/.config/bun ~/.cache/go-build ~/.cache/uv \
 && make --jobs=4 predev-go predev-python predev-frontend predev-tui \
 && make predev-check \
 && mkdir -p /opt/agent-ebpf-predev/frontend /opt/agent-ebpf-predev/adapters/python \
 && cp -a frontend/node_modules /opt/agent-ebpf-predev/frontend/node_modules \
 && cp -a adapters/python/.venv /opt/agent-ebpf-predev/adapters/python/.venv

CMD ["fish"]
