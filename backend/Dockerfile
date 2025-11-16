FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o api ./cmd/api

FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /root/

COPY --from=builder /app/api .
COPY --from=builder /app/config.yaml .

RUN mkdir -p /data

EXPOSE 8080

CMD ["./api"]
