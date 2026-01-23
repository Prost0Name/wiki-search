# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o wikiracer-api api.go

# Final stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/wikiracer-api .
COPY --from=builder /app/docs ./docs

CMD ["./wikiracer-api"]
