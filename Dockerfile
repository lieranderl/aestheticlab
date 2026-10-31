# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
# The Go site (cmd/web) for Cloud Run: one static binary on distroless, so a
# cold instance starts in well under a second. It replaced the Qwik image at
# the Phase 8 cutover (plans/go-datastar-scale-to-zero.md).
#
#   docker build -t aestheticlab-web:local .

# Static assets (frontend/build.mjs): CSS, fonts, Datastar, icons and WebP
# variants, content-hashed into static/dist/ for the Go binary to embed.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.4@sha256:1d22548f177f4ff38e0c15baa3c2c46c6c2060ba2d1bc573bb4720b407548f1d AS assets
WORKDIR /src
COPY package.json bun.lock ./
RUN --mount=type=cache,id=bun,target=/root/.bun/install/cache \
	bun install --frozen-lockfile
COPY frontend ./frontend
COPY internal ./internal
RUN bun run assets

FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY static ./static
COPY --from=assets /src/static/dist ./static/dist
# No `go mod download`: go.mod also lists the lint and security tools, and the
# build fetches only the modules cmd/web imports.
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags "-s -w" \
	-o /out/web ./cmd/web

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/web /web
LABEL org.opencontainers.image.title="Aesthetic Lab" \
	org.opencontainers.image.source="https://github.com/lieranderl/aestheticlab"
USER 65532:65532
EXPOSE 3000
ENTRYPOINT ["/web"]
