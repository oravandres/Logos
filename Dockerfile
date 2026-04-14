# syntax=docker/dockerfile:1
# Builder runs on the host platform (fast on CI); binary targets $TARGETARCH for multi-arch images.
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /logos ./cmd/logos

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /logos /logos
COPY migrations/ /migrations/

EXPOSE 8000

ENTRYPOINT ["/logos"]
