FROM golang:1.26.2-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /logos ./cmd/logos

FROM gcr.io/distroless/static-debian12

COPY --from=builder /logos /logos

EXPOSE 8000

ENTRYPOINT ["/logos"]
