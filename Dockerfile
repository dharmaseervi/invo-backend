
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -s -w strips the symbol table and DWARF data, and -trimpath removes local build
# paths. Smaller binary means a smaller image, which means less to pull when the
# platform starts a fresh container.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o main ./cmd/server/main.go

# Pinned rather than :latest so a rebuild produces the same image, and an upstream
# change cannot alter production without a commit.
FROM alpine:3.21
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Runs unprivileged: a process that never needs to write outside its own directory
# has no reason to be root.
RUN adduser -D -u 10001 appuser

COPY --from=builder /app/main .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/static ./static
COPY --from=builder /app/public ./public

USER appuser

EXPOSE 8080
CMD ["./main"]
