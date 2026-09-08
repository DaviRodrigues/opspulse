FROM golang:1.24.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o /app/opspulse ./cmd/opspulse

FROM alpine:latest AS runner

WORKDIR /app

RUN apk update && apk add --no-cache ca-certificates tzdata

RUN update-ca-certificates

COPY --from=builder /app/opspulse /app/opspulse

ENTRYPOINT [ "/app/opspulse" ]