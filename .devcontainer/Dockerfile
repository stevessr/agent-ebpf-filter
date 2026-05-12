# Devcontainer for the Go + eBPF backend, Vue/Bun frontend, wrapper, and adapters.
FROM debian:trixie

ARG DEBIAN_FRONTEND=noninteractive
ARG GO_VERSION=1.26.2
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

RUN curl -fsSL https://bun.sh/install | bash -s bun-v1.3.3 \
 && ln -sf /usr/local/bun/bin/bun /usr/local/bin/bun \
 && ln -sf /usr/local/bun/bin/bun /usr/local/bin/bunx

RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh

RUN if ! command -v zellij >/dev/null 2>&1; then \
      curl -fsSL https://sh.rustup.rs -o /tmp/rustup.sh \
      && sh /tmp/rustup.sh -y --profile minimal \
      && . /root/.cargo/env \
      && cargo install --locked zellij \
      && cp /root/.cargo/bin/zellij /usr/local/bin/zellij \
      && rm -rf /root/.cargo /tmp/rustup.sh; \
    fi

RUN groupadd --gid ${USER_GID} ${USERNAME} \
 && useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} -s /usr/bin/fish \
 && echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
 && chmod 0440 /etc/sudoers.d/${USERNAME} \
 && chown -R ${USERNAME}:${USERNAME} /go /usr/local/bun

USER ${USERNAME}
WORKDIR /workspaces/agent-ebpf-filiter

RUN mkdir -p ~/.config/bun ~/.cache/go-build ~/.cache/uv

CMD ["fish"]
