# Keep the build toolchain aligned with go.mod; do not inherit the old 0.3.4 image.
ARG GO_VERSION=1.27.3
FROM golang:${GO_VERSION}-bookworm

LABEL org.opencontainers.image.title="yatori-go-desktop"
LABEL org.opencontainers.image.version="0.4.2"

WORKDIR /src/yatori-go-desktop

# go.mod replaces yatori-go-console with the sibling source directory.
ARG YATORI_CONSOLE_REF=b003030ccb53209bf56977596dbc6c0d6cfb71e4
RUN git init /src/yatori-go-console \
    && git -C /src/yatori-go-console remote add origin https://github.com/yatori-dev/yatori-go-console.git \
    && git -C /src/yatori-go-console fetch --depth 1 origin "${YATORI_CONSOLE_REF}" \
    && git -C /src/yatori-go-console checkout --detach FETCH_HEAD

COPY . .

ENV CGO_ENABLED=1 GOTOOLCHAIN=local

CMD ["go", "test", "./service/...", "-v"]
