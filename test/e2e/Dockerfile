FROM golang:1.27-bookworm

ARG KIND_VERSION=v0.31.0
ARG KUBECTL_VERSION=v1.35.0

RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        ca-certificates \
        curl \
        docker.io \
        openssh-client \
    && rm -rf /var/lib/apt/lists/*

RUN architecture="$(dpkg --print-architecture)" \
    && curl --fail --location --silent --show-error \
        "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${architecture}" \
        --output /usr/local/bin/kind \
    && chmod +x /usr/local/bin/kind \
    && curl --fail --location --silent --show-error \
        "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${architecture}/kubectl" \
        --output /usr/local/bin/kubectl \
    && chmod +x /usr/local/bin/kubectl

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/local/bin/kage .

ENTRYPOINT ["/src/test/e2e/test.sh"]
