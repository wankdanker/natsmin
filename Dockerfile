# syntax=docker/dockerfile:1
FROM node:22-alpine AS ui
WORKDIR /src/ui
RUN corepack enable
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ui/ ./
COPY docs/openapi.json /src/docs/openapi.json
RUN pnpm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/wankdanker/natsmin/internal/version.Version=${VERSION}" \
    -o /out/natsmin ./cmd/natsmin

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/natsmin /natsmin
ENV NATSMIN_DATA=/data NATSMIN_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/natsmin"]
