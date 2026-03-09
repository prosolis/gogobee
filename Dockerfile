FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -tags goolm -o gogobee .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/gogobee .

VOLUME /app/data
ENV DATA_DIR=/app/data

CMD ["./gogobee"]
