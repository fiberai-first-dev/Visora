FROM golang:alpine AS builder

WORKDIR /app

# Install dependencies for go-sqlite3 if needed (though we use postgres now, good to have)
RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build both binaries
RUN go build -o /app/bin/server ./cmd/server
RUN go build -o /app/bin/worker ./cmd/worker

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/bin/server /usr/local/bin/
COPY --from=builder /app/bin/worker /usr/local/bin/

CMD ["server"]
