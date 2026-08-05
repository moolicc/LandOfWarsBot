FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o bot .

# --- Runtime Stage ---
FROM alpine:latest

WORKDIR /app

# Required for HTTPS requests to the Discord API
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bot .

ENV DISCORD_BOT_TOKEN="your_discord_bot_token_here"
ENV DB_PATH="/app/data/landsofwar.db"

CMD ["./bot"]